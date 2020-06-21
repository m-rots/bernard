package devstore

import (
	// datastore interfaces
	ds "github.com/m-rots/bernard/datastore"

	// reference sqlite datastore
	"github.com/m-rots/bernard/datastore/sqlite"
)

// The Devstore extends the reference sqlite datastore by adding extra
// functions related to Snapshot functionality.
type Devstore struct {
	*sqlite.Datastore
}

// A Snapshot is a representation of the current state within the datastore.
// The Snapshot includes all files and folders alphabetically sorted (ascending) on ID.
//
// Snapshots can be used to check whether two datastore states are equal
// and to list any differences between two datastores.
type Snapshot struct {
	Files   []ds.File
	Folders []ds.Folder
}

// New creates a new Devstore
func New(path string) (*Devstore, error) {
	datastore, err := sqlite.New(path)
	if err != nil {
		return nil, err
	}

	return &Devstore{datastore}, nil
}

// CreateSnapshot returns a *Snapshot of the current state of the datastore.
func (store *Devstore) CreateSnapshot(driveID string) (*Snapshot, error) {
	var files []ds.File
	var folders []ds.Folder

	fileRows, err := store.DB.Query(sqlSelectFiles, driveID)
	if err != nil {
		return nil, err
	}

	defer fileRows.Close()
	for fileRows.Next() {
		f := ds.File{}
		err = fileRows.Scan(&f.ID, &f.Name, &f.Parent, &f.Size, &f.MD5, &f.Trashed)
		if err != nil {
			return nil, err
		}

		files = append(files, f)
	}

	if err = fileRows.Err(); err != nil {
		return nil, err
	}

	folderRows, err := store.DB.Query(sqlSelectFolders, driveID)
	if err != nil {
		return nil, err
	}

	defer folderRows.Close()
	for folderRows.Next() {
		f := ds.Folder{}
		err = folderRows.Scan(&f.ID, &f.Name, &f.Trashed, &f.Parent)
		if err != nil {
			return nil, err
		}

		folders = append(folders, f)
	}

	if err = folderRows.Err(); err != nil {
		return nil, err
	}

	ss := &Snapshot{
		Files:   files,
		Folders: folders,
	}

	return ss, nil
}

const sqlSelectFolders = `
SELECT id, name, trashed, parent
FROM folder
WHERE drive=? AND parent IS NOT NULL
ORDER BY id ASC
`

const sqlSelectFiles = `
SELECT id, name, parent, size, md5, trashed
FROM file
WHERE drive=?
ORDER BY id ASC
`
