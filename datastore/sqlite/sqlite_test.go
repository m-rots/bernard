package sqlite

import (
	"errors"
	"reflect"
	"testing"

	ds "github.com/m-rots/bernard/datastore"
)

func setupTest(t *testing.T) *Datastore {
	t.Helper()

	datastore, err := New(":memory:")
	if err != nil {
		t.Fatal("Could not create datastore")
	}

	return datastore
}

func getFiles(t *testing.T, store *Datastore) (files []ds.File) {
	t.Helper()

	rows, err := store.DB.Query("SELECT id, name, parent, trashed, md5, size FROM file")
	if err != nil {
		t.Fatalf("Could not query file rows: %s", err.Error())
	}

	defer rows.Close()
	for rows.Next() {
		f := ds.File{}

		err = rows.Scan(&f.ID, &f.Name, &f.Parent, &f.Trashed, &f.MD5, &f.Size)
		if err != nil {
			t.Fatalf("Error when scanning file rows: %s", err.Error())
		}

		files = append(files, f)
	}

	err = rows.Err()
	if err != nil {
		t.Fatalf("Error when doing final row error check: %s", err.Error())
	}

	return files
}

func getFolders(t *testing.T, store *Datastore) (folders []ds.Folder) {
	t.Helper()

	rows, err := store.DB.Query("SELECT id, name, IFNULL(parent, \"\"), trashed FROM folder")
	if err != nil {
		t.Fatalf("Could not query folder rows: %s", err.Error())
	}

	defer rows.Close()
	for rows.Next() {
		f := ds.Folder{}

		err = rows.Scan(&f.ID, &f.Name, &f.Parent, &f.Trashed)
		if err != nil {
			t.Fatalf("Error when scanning folder rows: %s", err.Error())
		}

		folders = append(folders, f)
	}

	err = rows.Err()
	if err != nil {
		t.Fatalf("Error when doing final row error check: %s", err.Error())
	}

	return folders
}

func TestAddParameters(t *testing.T) {
	type Test struct {
		query  string
		items  int
		result string
	}

	var testCases = []Test{
		{
			query:  "DELETE FROM file WHERE id IN (?)",
			items:  5,
			result: "DELETE FROM file WHERE id IN (?, ?, ?, ?, ?)",
		},
		{
			query:  "?",
			items:  10,
			result: "?, ?, ?, ?, ?, ?, ?, ?, ?, ?",
		},
		{
			query:  "?",
			items:  1,
			result: "?",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.result, func(t *testing.T) {
			withParams := addParameters(tc.query, tc.items)
			if withParams != tc.result {
				t.Errorf("%s does not match expected value: %s", withParams, tc.result)
			}
		})
	}
}

func TestPageToken(t *testing.T) {
	type test struct {
		driveID   string
		pageToken string
	}

	var testCases = []test{
		{"test drive go brrr", "page token go brrr"},
	}

	for _, tc := range testCases {
		t.Run(tc.driveID, func(t *testing.T) {
			store := setupTest(t)

			_, err := store.DB.Exec(sqlUpsertDrive, tc.driveID, tc.pageToken)
			if err != nil {
				t.Errorf("Could not upsert Drive: %s", err.Error())
				return
			}

			pageToken, err := store.PageToken(tc.driveID)
			if err != nil {
				t.Errorf("Could not get Drive: %s", err.Error())
				return
			}

			if pageToken != tc.pageToken {
				t.Errorf("PageTokens do not match")
			}
		})
	}
}

func TestFullSync(t *testing.T) {
	type Given struct {
		drive   ds.Drive
		files   []ds.File
		folders []ds.Folder
	}

	type Expected struct {
		err     error
		files   []ds.File
		folders []ds.Folder
	}

	type test struct {
		name     string
		given    Given
		expected Expected
	}

	drive := ds.Drive{
		ID:        "bernard",
		Name:      "bernard go brrr",
		PageToken: "page token :)",
	}

	driveFolder := ds.Folder{
		ID:      drive.ID,
		Name:    drive.Name,
		Trashed: false,
	}

	var testCases = []test{
		{
			name: "invalid file parent -> data anomaly",
			given: Given{
				drive: drive,
				files: []ds.File{
					{ID: "A", Parent: "unknown parent"},
				},
			},
			expected: Expected{
				err: ds.ErrDataAnomaly,
			},
		},
		{
			name: "invalid folder parent -> data anomaly",
			given: Given{
				drive: drive,
				folders: []ds.Folder{
					{ID: "A", Parent: "unknown parent"},
				},
			},
			expected: Expected{
				err: ds.ErrDataAnomaly,
			},
		},
		{
			name: "all fields",
			given: Given{
				drive: drive,
				folders: []ds.Folder{
					{ID: "A", Name: "Folder A", Parent: drive.ID, Trashed: false},
					{ID: "B", Name: "Folder B", Parent: "A", Trashed: true},
				},
				files: []ds.File{
					{ID: "Z", Name: "File Z", MD5: "ZZZ", Parent: drive.ID, Size: 1000, Trashed: false},
					{ID: "Y", Name: "File Y", MD5: "YYY", Parent: "A", Size: 10, Trashed: true},
				},
			},
			expected: Expected{
				folders: []ds.Folder{
					driveFolder,
					{ID: "A", Name: "Folder A", Parent: drive.ID, Trashed: false},
					{ID: "B", Name: "Folder B", Parent: "A", Trashed: true},
				},
				files: []ds.File{
					{ID: "Z", Name: "File Z", MD5: "ZZZ", Parent: drive.ID, Size: 1000, Trashed: false},
					{ID: "Y", Name: "File Y", MD5: "YYY", Parent: "A", Size: 10, Trashed: true},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			store := setupTest(t)

			err := store.FullSync(tc.given.drive, tc.given.folders, tc.given.files)

			if !errors.Is(err, tc.expected.err) {
				t.Errorf("Unexpected error: %s", err.Error())
				return
			}

			// pageToken is only checked when transaction is not rolled back.
			// files and folders are checked to see whether no files/folders are left behind.
			if err == nil {
				pageToken, err := store.PageToken(tc.given.drive.ID)
				if err != nil {
					t.Errorf("Could not fetch pageToken: %s", err.Error())
					return
				}

				if pageToken != tc.given.drive.PageToken {
					t.Errorf("pageTokens do not match")
					return
				}
			}

			files := getFiles(t, store)
			if !reflect.DeepEqual(files, tc.expected.files) {
				t.Log(files)
				t.Log(tc.expected.files)
				t.Errorf("Files to not match")
			}

			folders := getFolders(t, store)
			if !reflect.DeepEqual(folders, tc.expected.folders) {
				t.Log(folders)
				t.Log(tc.expected.folders)
				t.Errorf("Folders to not match")
			}
		})
	}
}

func TestPartialSync(t *testing.T) {
	type State struct {
		files   []ds.File
		folders []ds.Folder
		drive   ds.Drive
	}

	type Changed struct {
		drive   ds.Drive
		files   []ds.File
		folders []ds.Folder
		removed []string
	}

	type Test struct {
		name     string
		given    State
		changes  Changed
		expected State
		err      error
	}

	var testCases = []Test{
		{
			name: "All Fields are updated",
			err:  nil,
			given: State{
				drive: ds.Drive{
					ID:        "Drive 123",
					PageToken: "12345",
					Name:      "Shared Drive",
				},
				folders: []ds.Folder{
					{ID: "A", Name: "Folder A", Parent: "Drive 123", Trashed: true},
					{ID: "B", Name: "Folder B", Parent: "Drive 123", Trashed: false},
				},
				files: []ds.File{
					{ID: "Z", MD5: "ZZZZ", Name: "File Z", Parent: "A", Size: 100, Trashed: false},
				},
			},
			changes: Changed{
				drive: ds.Drive{
					ID:        "Drive 123",
					PageToken: "23456",
				},
				folders: []ds.Folder{
					{ID: "A", Name: "Updated Folder A", Parent: "Drive 123", Trashed: false},
					{ID: "B", Name: "Updated Folder B", Parent: "A", Trashed: true},
				},
				files: []ds.File{
					{ID: "Z", MD5: "New MD5", Name: "Updated Z", Parent: "Drive 123", Size: 200, Trashed: true},
				},
			},
			expected: State{
				drive: ds.Drive{
					ID:        "Drive 123",
					PageToken: "23456",
				},
				folders: []ds.Folder{
					{ID: "Drive 123", Name: "Shared Drive", Trashed: false},
					{ID: "A", Name: "Updated Folder A", Parent: "Drive 123", Trashed: false},
					{ID: "B", Name: "Updated Folder B", Parent: "A", Trashed: true},
				},
				files: []ds.File{
					{ID: "Z", MD5: "New MD5", Name: "Updated Z", Parent: "Drive 123", Size: 200, Trashed: true},
				},
			},
		},
		{
			name: "No data anomaly when all children get deleted",
			err:  nil,
			given: State{
				drive: ds.Drive{
					ID:        "tha drive",
					PageToken: "old",
					Name:      "Not a Shared Drive",
				},
				folders: []ds.Folder{
					{ID: "A", Parent: "tha drive"},
					{ID: "B", Parent: "A"},
				},
				files: []ds.File{
					{ID: "Z", Parent: "A"},
					{ID: "Y", Parent: "B"},
				},
			},
			changes: Changed{
				drive: ds.Drive{
					ID:        "tha drive",
					PageToken: "new",
				},
				removed: []string{"A", "B", "Z", "Y"},
			},
			expected: State{
				drive: ds.Drive{
					ID:        "tha drive",
					PageToken: "new",
				},
				folders: []ds.Folder{
					{ID: "tha drive", Name: "Not a Shared Drive", Trashed: false},
				},
			},
		},
		{
			// files and folders should stay because of the transaction rollback
			name: "Data anomaly when children do not get deleted",
			err:  ds.ErrDataAnomaly,
			given: State{
				drive: ds.Drive{
					ID:        "tha drive",
					PageToken: "old",
					Name:      "Not a Shared Drive",
				},
				folders: []ds.Folder{
					{ID: "A", Parent: "tha drive"},
					{ID: "B", Parent: "A"},
				},
				files: []ds.File{
					{ID: "Z", Parent: "A"},
					{ID: "Y", Parent: "B"},
				},
			},
			changes: Changed{
				drive: ds.Drive{
					ID:        "tha drive",
					PageToken: "new",
				},
				removed: []string{"A"},
			},
			expected: State{
				drive: ds.Drive{
					ID:        "tha drive",
					PageToken: "old",
				},
				folders: []ds.Folder{
					{ID: "tha drive", Name: "Not a Shared Drive", Trashed: false},
					{ID: "A", Parent: "tha drive"},
					{ID: "B", Parent: "A"},
				},
				files: []ds.File{
					{ID: "Z", Parent: "A"},
					{ID: "Y", Parent: "B"},
				},
			},
		},
		{
			name: "Shared Drive name change",
			err:  nil,
			given: State{
				drive: ds.Drive{
					ID:        "Shared Drive",
					PageToken: "0",
					Name:      "Old name",
				},
			},
			changes: Changed{
				drive: ds.Drive{
					ID:        "Shared Drive",
					PageToken: "123",
					Name:      "New name",
				},
			},
			expected: State{
				drive: ds.Drive{
					ID:        "Shared Drive",
					PageToken: "123",
				},
				folders: []ds.Folder{
					{ID: "Shared Drive", Name: "New name"},
				},
			},
		},
		{
			name: "Data anomaly",
			err:  ds.ErrDataAnomaly,
			given: State{
				drive: ds.Drive{
					ID:        "root",
					PageToken: "0",
					Name:      "Data anomaly test",
				},
			},
			changes: Changed{
				drive: ds.Drive{
					ID:        "root",
					PageToken: "new pageToken",
					Name:      "New name which should not be updated",
				},
				files: []ds.File{
					{ID: "Z", Parent: "Non existent"},
				},
			},
			expected: State{
				drive: ds.Drive{
					ID:        "root",
					PageToken: "0",
				},
				folders: []ds.Folder{
					{ID: "root", Name: "Data anomaly test"},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			store := setupTest(t)

			err := store.FullSync(tc.given.drive, tc.given.folders, tc.given.files)
			if err != nil {
				t.Errorf("Error in full sync: %s", err.Error())
				return
			}

			err = store.PartialSync(tc.changes.drive, tc.changes.folders, tc.changes.files, tc.changes.removed)
			if !errors.Is(err, tc.err) {
				t.Errorf("Unexpected error: %s", err.Error())
				return
			}

			pageToken, err := store.PageToken(tc.expected.drive.ID)
			if err != nil {
				t.Errorf("Could not fetch pageToken: %s", err.Error())
				return
			}

			if pageToken != tc.expected.drive.PageToken {
				t.Errorf("pageTokens do not match")
				return
			}

			files := getFiles(t, store)
			if !reflect.DeepEqual(files, tc.expected.files) {
				t.Log(files)
				t.Log(tc.expected.files)
				t.Errorf("Files to not match")
			}

			folders := getFolders(t, store)
			if !reflect.DeepEqual(folders, tc.expected.folders) {
				t.Log(folders)
				t.Log(tc.expected.folders)
				t.Errorf("Folders to not match")
			}
		})
	}
}
