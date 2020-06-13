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
