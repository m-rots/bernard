package bernard

import (
	"encoding/json"
	"fmt"
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

type pageTokenResponse struct {
	StartPageToken string
}

type driveResponse struct {
	Name string
}

type allContentResponse struct {
	Files         []driveItem
	NextPageToken string
}

type changedContentResponse struct {
	NextPageToken     string
	NewStartPageToken string
	Changes           []driveChange
}

var client = &http.Client{
	Timeout: 30 * time.Second,
}

type fetcher struct {
	auth    Authenticator
	baseURL string
	driveID string
}

func bearer(accessToken string) string {
	return "Bearer " + accessToken
}

func (fetch *fetcher) mapError(status int) error {
	switch status {
	case 401:
		// Scope / Credential error
		return ErrInvalidCredentials
	case 404:
		// Shared Drive not found
		return fmt.Errorf("%v: %w", fetch.driveID, ErrNotFound)
	case 500:
		// should handle exponential timeout.
		// for now, just keep retrying
		return nil
	default:
		return fmt.Errorf("%v: %w", status, ErrNetwork)
	}
}

func (fetch *fetcher) pageToken() (string, error) {
	var startPageToken string

	for {
		token, _, err := fetch.auth.AccessToken()
		if err != nil {
			return "", err
		}

		req, _ := http.NewRequest("GET", fetch.baseURL+"/changes/startPageToken", nil)
		req.Header.Add("Authorization", bearer(token))

		q := url.Values{}
		q.Add("driveId", fetch.driveID)
		q.Add("supportsAllDrives", "true")
		req.URL.RawQuery = q.Encode()

		res, err := client.Do(req)
		if err != nil {
			// create a bernard network issue?
			return "", fmt.Errorf("could not make request: %w", ErrNetwork)
		}

		if res.StatusCode != 200 {
			res.Body.Close()

			if err := fetch.mapError(res.StatusCode); err != nil {
				return "", err
			}

			continue
		}

		response := new(pageTokenResponse)
		json.NewDecoder(res.Body).Decode(response)

		res.Body.Close()

		startPageToken = response.StartPageToken
		if startPageToken != "" {
			break
		}
	}

	return startPageToken, nil
}

func (fetch *fetcher) drive() (string, error) {
	var name string

	// for loop to handle 500s
	for {
		token, _, err := fetch.auth.AccessToken()
		if err != nil {
			return "", err
		}

		req, _ := http.NewRequest("GET", fetch.baseURL+"/drives/"+fetch.driveID, nil)
		req.Header.Add("Authorization", bearer(token))

		q := url.Values{}
		q.Add("fields", "name")
		req.URL.RawQuery = q.Encode()

		res, err := client.Do(req)
		if err != nil {
			// create a bernard network issue?
			return "", fmt.Errorf("could not make request: %w", ErrNetwork)
		}

		if res.StatusCode != 200 {
			res.Body.Close()

			if err := fetch.mapError(res.StatusCode); err != nil {
				return "", err
			}

			continue
		}

		response := new(driveResponse)
		json.NewDecoder(res.Body).Decode(response)

		res.Body.Close()

		name = response.Name
		if name != "" {
			break
		}
	}

	return name, nil
}

func (fetch *fetcher) allContent() ([]ds.Folder, []ds.File, error) {
	var files []ds.File
	var folders []ds.Folder
	var pageToken string

	for {
		token, _, err := fetch.auth.AccessToken()
		if err != nil {
			return nil, nil, err
		}

		req, _ := http.NewRequest("GET", fetch.baseURL+"/files", nil)
		req.Header.Add("Authorization", bearer(token))

		q := url.Values{}
		q.Add("corpora", "drive")
		q.Add("driveId", fetch.driveID)
		q.Add("pageSize", "1000")
		q.Add("includeItemsFromAllDrives", "true")
		q.Add("supportsAllDrives", "true")
		q.Add("fields", "nextPageToken,files(id,name,mimeType,parents,md5Checksum,size,trashed)")
		if pageToken != "" {
			q.Add("pageToken", pageToken)
		}

		req.URL.RawQuery = q.Encode()

		res, err := client.Do(req)
		if err != nil {
			return nil, nil, err
		}

		if res.StatusCode != 200 {
			res.Body.Close()

			if err := fetch.mapError(res.StatusCode); err != nil {
				return nil, nil, err
			}

			continue
		}

		decoder := json.NewDecoder(res.Body)
		response := new(allContentResponse)
		decoder.Decode(response)

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

type changedContent struct {
	Drive          ds.Drive
	ChangedFiles   []ds.File
	ChangedFolders []ds.Folder
	RemovedIDs     []string
}

func (fetch *fetcher) changedContent(pageToken string) (*changedContent, error) {
	var files []ds.File
	var folders []ds.Folder
	var removedIDs []string

	drive := ds.Drive{ID: fetch.driveID}

	for {
		token, _, err := fetch.auth.AccessToken()
		if err != nil {
			return nil, err
		}

		req, _ := http.NewRequest("GET", fetch.baseURL+"/changes", nil)
		req.Header.Add("Authorization", bearer(token))

		q := url.Values{}
		q.Add("driveId", fetch.driveID)
		q.Add("pageSize", "1000")
		q.Add("pageToken", pageToken)
		q.Add("includeItemsFromAllDrives", "true")
		q.Add("supportsAllDrives", "true")
		q.Add("fields", "nextPageToken,newStartPageToken,changes(driveId,fileId,removed,drive(id,name),file(id,driveId,name,mimeType,parents,md5Checksum,size,trashed))")
		req.URL.RawQuery = q.Encode()

		res, err := client.Do(req)
		if err != nil {
			return nil, err
		}

		if res.StatusCode != 200 {
			res.Body.Close()

			if err := fetch.mapError(res.StatusCode); err != nil {
				return nil, err
			}

			continue
		}

		response := new(changedContentResponse)
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

			if change.Removed || change.File.DriveID != fetch.driveID {
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
