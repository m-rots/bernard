package bernard

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	ds "github.com/m-rots/bernard/datastore"
)

const (
	accessToken string = "testAccessToken"
	driveID     string = "testDrive"
)

type mockAuth struct{}

func (auth *mockAuth) AccessToken() (string, int64, error) {
	return accessToken, 0, nil
}

func setupTest(handler http.HandlerFunc) (*fetcher, *httptest.Server) {
	server := httptest.NewServer(http.HandlerFunc(handler))
	fetch := &fetcher{
		auth:    &mockAuth{},
		driveID: driveID,
		baseURL: server.URL,
	}

	return fetch, server
}

func TestDrive(t *testing.T) {
	var called int

	handler := func(w http.ResponseWriter, r *http.Request) {
		called++

		// check if request is retried
		if called == 1 {
			w.WriteHeader(500)
			return
		}

		body := driveResponse{
			Name: "Coolest Drive on earth",
		}

		json.NewEncoder(w).Encode(body)
	}

	fetch, server := setupTest(handler)
	defer server.Close()

	name, err := fetch.drive()
	if err != nil {
		t.Errorf("unexpected error: %s", err.Error())
		return
	}

	if name != "Coolest Drive on earth" {
		t.Errorf("Wrong name returned")
	}
}

func TestPageToken(t *testing.T) {
	var called int

	handler := func(w http.ResponseWriter, r *http.Request) {
		called++

		// check if request is retried
		if called == 1 {
			w.WriteHeader(500)
			return
		}

		body := pageTokenResponse{
			StartPageToken: "100",
		}

		json.NewEncoder(w).Encode(body)
	}

	fetch, server := setupTest(handler)
	defer server.Close()

	pageToken, err := fetch.pageToken()
	if err != nil {
		t.Errorf("unexpected error: %s", err.Error())
		return
	}

	if pageToken != "100" {
		t.Errorf("Wrong pageToken returned")
	}
}

func TestAllContent(t *testing.T) {
	type test struct {
		name    string
		fixture string
		folders []ds.Folder
		files   []ds.File
	}

	var testCases = []test{
		{
			name:    "All Fields",
			fixture: "testdata/all-content/fields.json",
			folders: []ds.Folder{
				{
					ID:      "A",
					Name:    "FOLDER A",
					Parent:  "testDrive",
					Trashed: false,
				},
				{
					ID:      "B",
					Name:    "FOLDER B",
					Parent:  "A",
					Trashed: true,
				},
			},
			files: []ds.File{
				{
					ID:      "Z",
					MD5:     "ZZZ",
					Name:    "FILE Z",
					Parent:  "A",
					Size:    10,
					Trashed: false,
				},
				{
					ID:      "Y",
					MD5:     "YYY",
					Name:    "FILE Y",
					Parent:  "B",
					Size:    100,
					Trashed: true,
				},
			},
		},
		{
			name:    "Page Token",
			fixture: "testdata/all-content/pageToken.json",
			files: []ds.File{
				{
					ID:     "Z",
					Parent: "testDrive",
				},
				{
					ID:     "X",
					Parent: "testDrive",
				},
			},
		},
		{
			name:    "Order",
			fixture: "testdata/all-content/order.json",
			folders: []ds.Folder{
				{
					ID:     "A",
					Parent: "testDrive",
				},
				{
					ID:     "B",
					Parent: "A",
				},
				{
					ID:     "C",
					Parent: "B",
				},
			},
			files: []ds.File{
				{
					ID:     "Z",
					Parent: "C",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var called int

			fixturePath := tc.fixture
			handler := func(w http.ResponseWriter, r *http.Request) {
				called++

				// check if request is retried
				if called == 1 {
					w.WriteHeader(500)
					return
				}

				pageToken := r.URL.Query().Get("pageToken")
				if pageToken != "" {
					fixturePath = pageToken
				}

				http.ServeFile(w, r, fixturePath)
			}

			fetch, server := setupTest(handler)
			defer server.Close()

			folders, files, err := fetch.allContent()
			if err != nil {
				t.Errorf("AllContent returned an error: %s", err.Error())
				return
			}

			if !reflect.DeepEqual(folders, tc.folders) {
				t.Log(folders)
				t.Log(tc.folders)
				t.Error("Folders do not match the expected output")
				return
			}

			if !reflect.DeepEqual(files, tc.files) {
				t.Log(files)
				t.Log(tc.files)
				t.Error("Files do not match the expected output")
				return
			}
		})
	}
}

func TestChangedContent(t *testing.T) {
	type test struct {
		name     string
		fixture  string
		expected changedContent
	}

	var testCases = []test{
		{
			name:    "all fields",
			fixture: "testdata/changed-content/fields.json",
			expected: changedContent{
				Drive: ds.Drive{
					ID:        driveID,
					PageToken: "page token go brrr",
				},
				ChangedFiles: []ds.File{
					{
						ID:      "B",
						MD5:     "BBB",
						Name:    "file B",
						Parent:  "A",
						Size:    10,
						Trashed: true,
					},
				},
				ChangedFolders: []ds.Folder{
					{
						ID:      "A",
						Name:    "folder A",
						Parent:  driveID,
						Trashed: false,
					},
				},
			},
		},
		{
			name:    "removed",
			fixture: "testdata/changed-content/removed.json",
			expected: changedContent{
				Drive: ds.Drive{
					ID:        driveID,
					PageToken: "page token go brrr",
				},
				RemovedIDs: []string{
					"A",
				},
			},
		},
		{
			name:    "different drive ID",
			fixture: "testdata/changed-content/wrong-drive-id.json",
			expected: changedContent{
				Drive: ds.Drive{
					ID:        driveID,
					PageToken: "page token go brrr",
				},
				RemovedIDs: []string{
					"A",
				},
			},
		},
		{
			name:    "drive name change",
			fixture: "testdata/changed-content/drive-name.json",
			expected: changedContent{
				Drive: ds.Drive{
					ID:        driveID,
					PageToken: "page token!!",
					Name:      "bernard go brrr",
				},
			},
		},
		{
			name:    "page token",
			fixture: "testdata/changed-content/pageToken.json",
			expected: changedContent{
				Drive: ds.Drive{
					ID:        driveID,
					PageToken: "page token go brrr",
				},
				ChangedFiles: []ds.File{
					{
						ID:     "Z",
						Parent: driveID,
					},
					{
						ID:     "Y",
						Parent: driveID,
					},
				},
			},
		},
		{
			name:    "folder order",
			fixture: "testdata/changed-content/order.json",
			expected: changedContent{
				Drive: ds.Drive{
					ID:        driveID,
					PageToken: "page token go brrr",
				},
				ChangedFolders: []ds.Folder{
					{
						ID:     "A",
						Parent: driveID,
					},
					{
						ID:     "B",
						Parent: "A",
					},
					{
						ID:     "C",
						Parent: "B",
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var called int

			fixturePath := tc.fixture
			handler := func(w http.ResponseWriter, r *http.Request) {
				called++

				// check if request is retried
				if called == 1 {
					w.WriteHeader(500)
					return
				}

				pageToken := r.URL.Query().Get("pageToken")
				if pageToken != "" {
					fixturePath = pageToken
				}

				http.ServeFile(w, r, fixturePath)
			}

			fetch, server := setupTest(handler)
			defer server.Close()

			diff, err := fetch.changedContent(tc.fixture)
			if err != nil {
				t.Errorf("ChangedContent returned an error: %s", err.Error())
				return
			}

			if !reflect.DeepEqual(diff.ChangedFiles, tc.expected.ChangedFiles) {
				t.Log(diff.ChangedFiles)
				t.Log(tc.expected.ChangedFiles)
				t.Error("Changed files do not match the expected output")
				return
			}

			if !reflect.DeepEqual(diff.ChangedFolders, tc.expected.ChangedFolders) {
				t.Log(diff.ChangedFolders)
				t.Log(tc.expected.ChangedFolders)
				t.Error("Changed folders do not match the expected output")
				return
			}

			if !reflect.DeepEqual(diff.RemovedIDs, tc.expected.RemovedIDs) {
				t.Log(diff.RemovedIDs)
				t.Log(tc.expected.RemovedIDs)
				t.Error("Removed IDs do not match the expected output")
				return
			}

			if !reflect.DeepEqual(diff.Drive, tc.expected.Drive) {
				t.Log(diff.Drive)
				t.Log(tc.expected.Drive)
				t.Error("Drive does not match the expected output")
				return
			}
		})
	}
}

func TestErrorResponses(t *testing.T) {
	type test struct {
		name        string
		statusCode  int
		targetError error
	}

	var testCases = []test{
		{"401 returns ErrInvalidCredentials", 401, ErrInvalidCredentials},
		{"403 returns ErrNetwork", 403, ErrNetwork},
		{"404 returns ErrNotFound", 404, ErrNotFound},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("authorization") != bearer(accessToken) {
					w.WriteHeader(401)
					return
				}

				w.WriteHeader(tc.statusCode)
			}

			fetch, server := setupTest(handler)
			defer server.Close()

			compareError := func(err error) {
				if !errors.Is(err, tc.targetError) {
					t.Errorf("Wrong error returned")
				}
			}

			t.Run("drive", func(t *testing.T) {
				_, err := fetch.drive()
				compareError(err)
			})

			t.Run("pageToken", func(t *testing.T) {
				_, err := fetch.pageToken()
				compareError(err)
			})

			t.Run("allContent", func(t *testing.T) {
				_, _, err := fetch.allContent()
				compareError(err)
			})

			t.Run("changedContent", func(t *testing.T) {
				_, err := fetch.changedContent("pageToken")
				compareError(err)
			})
		})
	}
}

func TestRootFolders(t *testing.T) {
	type test struct {
		name     string
		input    []ds.Folder
		roots    []ds.Folder
		nonRoots []ds.Folder
	}

	var testCases = []test{
		{
			name: "mixed",
			input: []ds.Folder{
				{ID: "A", Parent: "Z"},
				{ID: "B", Parent: "A"},
				{ID: "C", Parent: "B"},
				{ID: "D", Parent: "Z"},
			},
			roots: []ds.Folder{
				{ID: "A", Parent: "Z"},
				{ID: "D", Parent: "Z"},
			},
			nonRoots: []ds.Folder{
				{ID: "B", Parent: "A"},
				{ID: "C", Parent: "B"},
			},
		},
		{
			name: "single file",
			input: []ds.Folder{
				{ID: "A", Parent: "Z"},
			},
			roots: []ds.Folder{
				{ID: "A", Parent: "Z"},
			},
		},
		{
			name: "roots only",
			input: []ds.Folder{
				{ID: "A", Parent: "Z"},
				{ID: "B", Parent: "X"},
				{ID: "C", Parent: "Y"},
			},
			roots: []ds.Folder{
				{ID: "A", Parent: "Z"},
				{ID: "B", Parent: "X"},
				{ID: "C", Parent: "Y"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			roots, nonRoots := rootFolders(tc.input)

			if !reflect.DeepEqual(roots, tc.roots) {
				t.Log(roots)
				t.Log(tc.roots)
				t.Errorf("roots do not align")
			}

			if !reflect.DeepEqual(nonRoots, tc.nonRoots) {
				t.Log(nonRoots)
				t.Log(tc.nonRoots)
				t.Errorf("non-roots do not align")
			}
		})
	}
}

func TestHierarchicalOrder(t *testing.T) {
	type test struct {
		name   string
		input  []ds.Folder
		output []ds.Folder
	}

	var testCases = []test{
		{
			name: "roots only",
			input: []ds.Folder{
				{ID: "A", Parent: "Z"},
				{ID: "B", Parent: "Y"},
				{ID: "C", Parent: "X"},
			},
			output: []ds.Folder{
				{ID: "A", Parent: "Z"},
				{ID: "B", Parent: "Y"},
				{ID: "C", Parent: "X"},
			},
		},
		{
			name: "ordered",
			input: []ds.Folder{
				{ID: "A", Parent: "Z"},
				{ID: "B", Parent: "A"},
				{ID: "C", Parent: "B"},
				{ID: "D", Parent: "C"},
				{ID: "E", Parent: "B"},
				{ID: "F", Parent: "E"},
			},
			output: []ds.Folder{
				{ID: "A", Parent: "Z"},
				{ID: "B", Parent: "A"},
				{ID: "C", Parent: "B"},
				{ID: "E", Parent: "B"},
				{ID: "D", Parent: "C"},
				{ID: "F", Parent: "E"},
			},
		},
		{
			name: "totally wrong order",
			input: []ds.Folder{
				{ID: "C", Parent: "B"},
				{ID: "B", Parent: "A"},
				{ID: "A", Parent: "Z"},
			},
			output: []ds.Folder{
				{ID: "A", Parent: "Z"},
				{ID: "B", Parent: "A"},
				{ID: "C", Parent: "B"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ordered := orderFoldersOnHierarchy(tc.input)

			if !reflect.DeepEqual(ordered, tc.output) {
				t.Log(ordered)
				t.Log(tc.output)
				t.Errorf("output does not match expected output")
			}
		})
	}
}

func TestConvert(t *testing.T) {
	const folderMime string = "application/vnd.google-apps.folder"

	type test struct {
		name    string
		input   []driveItem
		folders []ds.Folder
		files   []ds.File
	}

	var testCases = []test{
		{
			name: "multiple parents",
			input: []driveItem{
				{
					ID:       "A",
					Name:     "FOLDER A",
					Trashed:  false,
					MimeType: folderMime,
					Parents:  []string{"Z", "Y"},
				},
				{
					ID:          "B",
					Name:        "FILE B",
					Trashed:     false,
					MimeType:    "image/png",
					MD5Checksum: "MD5 B",
					Size:        1010,
					Parents:     []string{"A"},
				},
				{
					ID:       "C",
					Name:     "FOLDER C",
					Trashed:  true,
					MimeType: folderMime,
					Parents:  []string{"A"},
				},
				{
					ID:          "D",
					Name:        "FILE D",
					Trashed:     true,
					MimeType:    "image/jpeg",
					MD5Checksum: "MD5 D",
					Size:        101010,
					Parents:     []string{"C"},
				},
			},
			folders: []ds.Folder{
				{
					ID:      "A",
					Name:    "FOLDER A",
					Trashed: false,
					Parent:  "Z",
				},
				{
					ID:      "C",
					Name:    "FOLDER C",
					Trashed: true,
					Parent:  "A",
				},
			},
			files: []ds.File{
				{
					ID:      "B",
					Name:    "FILE B",
					Trashed: false,
					MD5:     "MD5 B",
					Size:    1010,
					Parent:  "A",
				},
				{
					ID:      "D",
					Name:    "FILE D",
					Trashed: true,
					MD5:     "MD5 D",
					Size:    101010,
					Parent:  "C",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			folders, files := convert(tc.input)

			if !reflect.DeepEqual(folders, tc.folders) {
				t.Log(folders)
				t.Log(tc.folders)
				t.Errorf("folders do not match expected output")
			}

			if !reflect.DeepEqual(files, tc.files) {
				t.Log(files)
				t.Log(tc.files)
				t.Errorf("files do not match expected output")
			}
		})
	}
}
