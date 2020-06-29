package bernard

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"time"

	ds "github.com/m-rots/bernard/datastore"
)

type driveItem struct {
	ID          string
	Name        string
	MimeType    string
	Parents     []string
	Size        int `json:"size,string"`
	MD5Checksum string
	Trashed     bool
	DriveID     string
}

type sharedDrive struct {
	ID   string
	Name string
}

type driveChange struct {
	Drive   sharedDrive
	DriveID string
	File    driveItem
	FileID  string
	Removed bool
}

type driveError struct {
	Domain  string
	Message string
	Reason  string
}

type errorResponse struct {
	Error struct {
		Errors  []driveError
		Code    int
		Message string
	}
}

type changedContent struct {
	Drive          ds.Drive
	ChangedFiles   []ds.File
	ChangedFolders []ds.Folder
	RemovedIDs     []string
}

type fetcher struct {
	auth    Authenticator
	baseURL string
	client  *http.Client
	sleep   func(time.Duration)
}

func (fetch *fetcher) withAuth(req *http.Request) (res *http.Response, err error) {
	var retriedAttempts int

	// handle exponential backoff
	handleBackoff := func() {
		var waitDuration time.Duration

		exponentialBackoff := math.Exp2(float64(retriedAttempts))
		if exponentialBackoff <= 32 {
			waitDuration = time.Duration(exponentialBackoff) * time.Second
		} else {
			waitDuration = time.Duration(32) * time.Second
		}

		fetch.sleep(waitDuration)
		retriedAttempts++
	}

	// for loop to retry if necessary
	for {
		token, _, err := fetch.auth.AccessToken()
		if err != nil {
			return nil, err
		}

		req.Header.Add("Authorization", "Bearer "+token)
		res, err = fetch.client.Do(req)
		if err != nil {
			return nil, ErrNetwork
		}

		if res.StatusCode == 200 {
			return res, nil
		}

		response := new(errorResponse)
		json.NewDecoder(res.Body).Decode(response)
		res.Body.Close()

		switch res.StatusCode {
		case 429, 500, 502, 503, 504:
			handleBackoff()
			continue
		case 401:
			return nil, ErrInvalidCredentials
		case 403:
			driveErrors := response.Error.Errors
			if len(driveErrors) == 0 {
				return nil, fmt.Errorf("%v: %w", response.Error.Message, ErrNetwork)
			}
			switch response.Error.Errors[0].Reason {
			case "userRateLimitExceeded", "rateLimitExceeded":
				handleBackoff()
				continue
			default:
				return nil, fmt.Errorf("%v: %w", response.Error.Message, ErrNetwork)
			}
		case 404:
			return nil, fmt.Errorf("%v: %w", response.Error.Message, ErrNotFound)
		default:
			return nil, fmt.Errorf("%v: %w", response.Error.Message, ErrNetwork)
		}
	}
}

func (fetch *fetcher) pageToken(driveID string) (string, error) {
	req, _ := http.NewRequest("GET", fetch.baseURL+"/changes/startPageToken", nil)

	q := url.Values{}
	q.Add("driveId", driveID)
	q.Add("supportsAllDrives", "true")
	req.URL.RawQuery = q.Encode()

	res, err := fetch.withAuth(req)
	if err != nil {
		return "", err
	}

	type Response struct {
		StartPageToken string
	}

	response := new(Response)
	json.NewDecoder(res.Body).Decode(response)
	res.Body.Close()

	return response.StartPageToken, nil
}

func (fetch *fetcher) drive(driveID string) (string, error) {
	req, _ := http.NewRequest("GET", fetch.baseURL+"/drives/"+driveID, nil)

	q := url.Values{}
	q.Add("fields", "name")
	req.URL.RawQuery = q.Encode()

	res, err := fetch.withAuth(req)
	if err != nil {
		return "", err
	}

	type Response struct {
		Name string
	}

	response := new(Response)
	json.NewDecoder(res.Body).Decode(response)
	res.Body.Close()

	return response.Name, nil
}

func (fetch *fetcher) allContent(driveID string) ([]ds.Folder, []ds.File, error) {
	var files []ds.File
	var folders []ds.Folder
	var pageToken string

	for {
		req, _ := http.NewRequest("GET", fetch.baseURL+"/files", nil)

		q := url.Values{}
		q.Add("corpora", "drive")
		q.Add("driveId", driveID)
		q.Add("pageSize", "1000")
		q.Add("includeItemsFromAllDrives", "true")
		q.Add("supportsAllDrives", "true")
		q.Add("fields", "nextPageToken,files(id,name,mimeType,parents,md5Checksum,size,trashed)")
		if pageToken != "" {
			q.Add("pageToken", pageToken)
		}

		req.URL.RawQuery = q.Encode()

		res, err := fetch.withAuth(req)
		if err != nil {
			return nil, nil, err
		}

		type Response struct {
			Files         []driveItem
			NextPageToken string
		}

		response := new(Response)
		json.NewDecoder(res.Body).Decode(response)
		res.Body.Close()

		newFolders, newFiles := convert(response.Files)
		folders = append(folders, newFolders...)
		files = append(files, newFiles...)

		pageToken = response.NextPageToken

		if pageToken == "" {
			break
		}
	}

	orderedFolders := orderFoldersOnHierarchy(folders)
	return orderedFolders, files, nil
}

func (fetch *fetcher) changedContent(driveID string, pageToken string) (*changedContent, error) {
	var files []ds.File
	var folders []ds.Folder
	var removedIDs []string

	drive := ds.Drive{ID: driveID}

	for {
		req, _ := http.NewRequest("GET", fetch.baseURL+"/changes", nil)

		q := url.Values{}
		q.Add("driveId", driveID)
		q.Add("pageSize", "1000")
		q.Add("pageToken", pageToken)
		q.Add("includeItemsFromAllDrives", "true")
		q.Add("supportsAllDrives", "true")
		q.Add("fields", "nextPageToken,newStartPageToken,changes(driveId,fileId,removed,drive(id,name),file(id,driveId,name,mimeType,parents,md5Checksum,size,trashed))")
		req.URL.RawQuery = q.Encode()

		res, err := fetch.withAuth(req)
		if err != nil {
			return nil, err
		}

		type Response struct {
			NextPageToken     string
			NewStartPageToken string
			Changes           []driveChange
		}

		response := new(Response)
		json.NewDecoder(res.Body).Decode(response)
		res.Body.Close()

		var changedItems []driveItem

		for _, change := range response.Changes {
			if change.DriveID != "" {
				drive.Name = change.Drive.Name
				continue
			}

			if change.FileID == "" {
				continue
			}

			if change.Removed || change.File.DriveID != driveID {
				removedIDs = append(removedIDs, change.FileID)
			} else {
				changedItems = append(changedItems, change.File)
			}
		}

		changedFolders, changedFiles := convert(changedItems)
		folders = append(folders, changedFolders...)
		files = append(files, changedFiles...)

		pageToken = response.NextPageToken
		drive.PageToken = response.NewStartPageToken

		if pageToken == "" {
			break
		}
	}

	orderedFolders := orderFoldersOnHierarchy(folders)

	output := &changedContent{
		Drive:          drive,
		ChangedFiles:   files,
		ChangedFolders: orderedFolders,
		RemovedIDs:     removedIDs,
	}

	return output, nil
}

func convert(content []driveItem) (folders []ds.Folder, files []ds.File) {
	for _, item := range content {
		if item.MimeType == "application/vnd.google-apps.folder" {
			folder := ds.Folder{
				ID:      item.ID,
				Name:    item.Name,
				Parent:  item.Parents[0],
				Trashed: item.Trashed,
			}

			folders = append(folders, folder)
		} else {
			file := ds.File{
				ID:      item.ID,
				Name:    item.Name,
				Parent:  item.Parents[0],
				Trashed: item.Trashed,
				MD5:     item.MD5Checksum,
				Size:    item.Size,
			}

			files = append(files, file)
		}
	}

	return folders, files
}

func rootFolders(folders []ds.Folder) (roots []ds.Folder, nonRoots []ds.Folder) {
	IDtoParent := make(map[string]string)
	IDtoFolder := make(map[string]ds.Folder)

	for _, folder := range folders {
		IDtoParent[folder.ID] = folder.Parent
		IDtoFolder[folder.ID] = folder
	}

	for _, f := range folders {
		if _, ok := IDtoParent[f.Parent]; ok {
			nonRoots = append(nonRoots, IDtoFolder[f.ID])
		} else {
			roots = append(roots, IDtoFolder[f.ID])
		}
	}

	return roots, nonRoots
}

func orderFoldersOnHierarchy(nonRoots []ds.Folder) (ordered []ds.Folder) {
	for {
		if len(nonRoots) == 0 {
			break
		}

		roots, newNonRoots := rootFolders(nonRoots)
		nonRoots = newNonRoots

		ordered = append(ordered, roots...)
	}

	return ordered
}
