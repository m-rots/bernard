package sqlite

import (
	"errors"
	"reflect"
	"testing"

	ds "github.com/m-rots/bernard/datastore"
)

func TestDifferenceHook(t *testing.T) {
	type Store struct {
		drive   ds.Drive
		folders []ds.Folder
		files   []ds.File
	}

	type Changes struct {
		drive   ds.Drive
		folders []ds.Folder
		files   []ds.File
		removed []string
	}

	type Test struct {
		name     string
		store    Store
		err      error
		changes  Changes
		expected *Difference
	}

	drive := ds.Drive{
		ID:        "drive",
		Name:      "Hooks Support",
		PageToken: "123",
	}

	var testCases = []Test{
		{
			name: "added files & folders",
			err:  nil,
			store: Store{
				drive: drive,
			},
			changes: Changes{
				drive: drive,
				folders: []ds.Folder{
					{ID: "A", Parent: "drive", Name: "Folder A", Trashed: false},
					{ID: "B", Parent: "A", Name: "Folder B", Trashed: true},
				},
				files: []ds.File{
					{ID: "Z", Parent: "drive", Name: "File Z", MD5: "ZZZ", Size: 10, Trashed: false},
					{ID: "Y", Parent: "A", Name: "File Y", MD5: "YYY", Size: 100, Trashed: true},
				},
			},
			expected: &Difference{
				AddedFolders: []ds.Folder{
					{ID: "A", Parent: "drive", Name: "Folder A", Trashed: false},
					{ID: "B", Parent: "A", Name: "Folder B", Trashed: true},
				},
				AddedFiles: []ds.File{
					{ID: "Z", Parent: "drive", Name: "File Z", MD5: "ZZZ", Size: 10, Trashed: false},
					{ID: "Y", Parent: "A", Name: "File Y", MD5: "YYY", Size: 100, Trashed: true},
				},
			},
		},
		{
			name: "changed folders (name, parent, trashed)",
			err:  nil,
			store: Store{
				drive: drive,
				folders: []ds.Folder{
					{ID: "A", Name: "old name", Parent: "drive", Trashed: false}, // name
					{ID: "B", Name: "folder b", Parent: "drive", Trashed: false}, // parent
					{ID: "C", Name: "folder c", Parent: "B", Trashed: false},     // trashed
				},
			},
			changes: Changes{
				drive: drive,
				folders: []ds.Folder{
					{ID: "A", Name: "new name", Parent: "drive", Trashed: false}, // name
					{ID: "B", Name: "folder b", Parent: "A", Trashed: false},     // parent
					{ID: "C", Name: "folder c", Parent: "B", Trashed: true},      // trashed
				},
			},
			expected: &Difference{
				ChangedFolders: []ds.Folder{
					{ID: "A", Name: "new name", Parent: "drive", Trashed: false}, // name
					{ID: "B", Name: "folder b", Parent: "A", Trashed: false},     // parent
					{ID: "C", Name: "folder c", Parent: "B", Trashed: true},      // trashed
				},
			},
		},
		{
			name: "changed files (name, parent, trashed, md5, size)",
			err:  nil,
			store: Store{
				drive: drive,
				folders: []ds.Folder{
					{ID: "A", Name: "folder A", Parent: "drive", Trashed: false},
				},
				files: []ds.File{
					{ID: "Z", MD5: "old md5", Name: "file Z", Parent: "drive", Size: 10, Trashed: false}, // md5
					{ID: "Y", MD5: "YYY md5", Name: "old name", Parent: "A", Size: 20, Trashed: true},    // name
					{ID: "X", MD5: "XXX md5", Name: "file X", Parent: "A", Size: 30, Trashed: false},     // parent
					{ID: "W", MD5: "WWW md5", Name: "file W", Parent: "A", Size: 40, Trashed: false},     // size
					{ID: "V", MD5: "VVV md5", Name: "file V", Parent: "A", Size: 50, Trashed: true},      // trashed
				},
			},
			changes: Changes{
				drive: drive,
				files: []ds.File{
					{ID: "Z", MD5: "new md5", Name: "file Z", Parent: "drive", Size: 10, Trashed: false}, // md5
					{ID: "Y", MD5: "YYY md5", Name: "new name", Parent: "A", Size: 20, Trashed: true},    // name
					{ID: "X", MD5: "XXX md5", Name: "file X", Parent: "drive", Size: 30, Trashed: false}, // parent
					{ID: "W", MD5: "WWW md5", Name: "file W", Parent: "A", Size: 80, Trashed: false},     // size
					{ID: "V", MD5: "VVV md5", Name: "file V", Parent: "A", Size: 50, Trashed: false},     // trashed
				},
			},
			expected: &Difference{
				ChangedFiles: []ds.File{
					{ID: "Z", MD5: "new md5", Name: "file Z", Parent: "drive", Size: 10, Trashed: false}, // md5
					{ID: "Y", MD5: "YYY md5", Name: "new name", Parent: "A", Size: 20, Trashed: true},    // name
					{ID: "X", MD5: "XXX md5", Name: "file X", Parent: "drive", Size: 30, Trashed: false}, // parent
					{ID: "W", MD5: "WWW md5", Name: "file W", Parent: "A", Size: 80, Trashed: false},     // size
					{ID: "V", MD5: "VVV md5", Name: "file V", Parent: "A", Size: 50, Trashed: false},     // trashed
				},
			},
		},
		{
			name: "removed files and folders should return last-known state",
			err:  nil,
			store: Store{
				drive: drive,
				folders: []ds.Folder{
					{ID: "A", Name: "folder A", Parent: "drive", Trashed: false},
					{ID: "B", Name: "folder B", Parent: "A", Trashed: true},
				},
				files: []ds.File{
					{ID: "Z", Name: "file Z", Parent: "drive", Trashed: false, MD5: "ZZZ", Size: 10},
					{ID: "Y", Name: "file Y", Parent: "A", Trashed: false, MD5: "YYY", Size: 100000},
					{ID: "X", Name: "file X", Parent: "B", Trashed: true, MD5: "XXX", Size: 2525252},
				},
			},
			changes: Changes{
				drive:   drive,
				removed: []string{"A", "B", "Z", "Y", "X"},
			},
			expected: &Difference{
				RemovedFolders: []ds.Folder{
					{ID: "A", Name: "folder A", Parent: "drive", Trashed: false},
					{ID: "B", Name: "folder B", Parent: "A", Trashed: true},
				},
				RemovedFiles: []ds.File{
					{ID: "Z", Name: "file Z", Parent: "drive", Trashed: false, MD5: "ZZZ", Size: 10},
					{ID: "Y", Name: "file Y", Parent: "A", Trashed: false, MD5: "YYY", Size: 100000},
					{ID: "X", Name: "file X", Parent: "B", Trashed: true, MD5: "XXX", Size: 2525252},
				},
			},
		},
		{
			name: "no actual changes",
			err:  nil,
			store: Store{
				drive: drive,
				folders: []ds.Folder{
					{ID: "A", Name: "folder A", Parent: "drive", Trashed: false},
				},
				files: []ds.File{
					{ID: "Z", Name: "file Z", Parent: "A", Trashed: false, MD5: "ZZZ", Size: 10},
				},
			},
			changes: Changes{
				drive: drive,
				folders: []ds.Folder{
					{ID: "A", Name: "folder A", Parent: "drive", Trashed: false},
				},
				files: []ds.File{
					{ID: "Z", Name: "file Z", Parent: "A", Trashed: false, MD5: "ZZZ", Size: 10},
				},
			},
			expected: &Difference{},
		},
		{
			name: "data anomaly on non-existent removed item",
			err:  ds.ErrDataAnomaly,
			store: Store{
				drive: drive,
			},
			changes: Changes{
				removed: []string{"A"},
			},
			expected: &Difference{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			store := setupTest(t)

			err := store.FullSync(tc.store.drive, tc.store.folders, tc.store.files)
			if err != nil {
				t.Errorf("Unexpected error at full sync: %s", err.Error())
				return
			}

			hook, diff := store.NewDifferencesHook()

			err = hook(tc.changes.drive, tc.changes.files, tc.changes.folders, tc.changes.removed)
			if !errors.Is(err, tc.err) {
				t.Errorf("Unexpected error when running hook: %s", err.Error())
				return
			}

			if !reflect.DeepEqual(diff, tc.expected) {
				t.Log(diff)
				t.Log(tc.expected)
				t.Errorf("Difference does not match the expected outcome")
				return
			}
		})
	}
}
