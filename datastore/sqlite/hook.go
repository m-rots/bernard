package sqlite

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/m-rots/bernard"
	ds "github.com/m-rots/bernard/datastore"
)

// The Difference contains all added, changes and removed files and folders
// between two states.
type Difference struct {
	AddedFiles   []ds.File
	ChangedFiles []ds.File
	RemovedFiles []ds.File

	AddedFolders   []ds.Folder
	ChangedFolders []ds.Folder
	RemovedFolders []ds.Folder
}

// NewDifferencesHook creates a Hook which checks which files and folders
// have been added, changed or removed.
//
// Like all hooks, the corresponding output struct is only updated
// when the hook is executed.
func (store *Datastore) NewDifferencesHook() (bernard.Hook, *Difference) {
	var diff Difference

	hook := func(drive ds.Drive, files []ds.File, folders []ds.Folder, removed []string) error {
		// prepare the `sqlGetFolderByID` statement for better performance
		getFolder, err := store.DB.Prepare(sqlGetFolderByID)
		if err != nil {
			return fmt.Errorf("%v: %w", sqlGetFolderByID, ErrInvalidStatement)
		}

		defer getFolder.Close()

		for _, folder := range folders {
			f := ds.Folder{ID: folder.ID}
			row := getFolder.QueryRow(folder.ID)
			err := row.Scan(&f.Name, &f.Parent, &f.Trashed)
			if err != nil {
				// If no row is returned, the folder does not yet exist in the old state.
				// Therefore, this folder must have been added.
				if errors.Is(err, sql.ErrNoRows) {
					diff.AddedFolders = append(diff.AddedFolders, folder)
					continue
				}

				return fmt.Errorf("folder scan err in hook: %w", ds.ErrDatabase)
			}

			// If any of the fields do not align between the old and new state,
			// then this folder must have been changed.
			if f.Name != folder.Name || f.Parent != folder.Parent || f.Trashed != folder.Trashed {
				diff.ChangedFolders = append(diff.ChangedFolders, folder)
			}
		}

		// prepare the `sqlGetFileByID` statement for better performance
		getFile, err := store.DB.Prepare(sqlGetFileByID)
		if err != nil {
			return fmt.Errorf("%v: %w", sqlGetFileByID, ErrInvalidStatement)
		}

		defer getFile.Close()

		for _, file := range files {
			f := ds.File{ID: file.ID}
			row := getFile.QueryRow(file.ID)
			err := row.Scan(&f.Name, &f.Parent, &f.Trashed, &f.Size, &f.MD5)
			if err != nil {
				// If no row is returned, the file does not yet exist in the old state.
				// Therefore, this file must have been added.
				if errors.Is(err, sql.ErrNoRows) {
					diff.AddedFiles = append(diff.AddedFiles, file)
					continue
				}

				return fmt.Errorf("file scan err in hook: %w", ds.ErrDatabase)
			}

			// If any of the fields do not align between the old and new state,
			// then this file must have been changed.
			if f.Name != file.Name || f.Parent != file.Parent || f.Trashed != file.Trashed || f.Size != file.Size || f.MD5 != file.MD5 {
				diff.ChangedFiles = append(diff.ChangedFiles, file)
			}
		}

		for _, id := range removed {
			// check whether it could be a file first
			file := ds.File{ID: id}
			fileRow := getFile.QueryRow(id)
			err := fileRow.Scan(&file.Name, &file.Parent, &file.Trashed, &file.Size, &file.MD5)

			// no error -> thus a file
			if err == nil {
				diff.RemovedFiles = append(diff.RemovedFiles, file)
				continue
			}

			if !errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("removed file scan err in hook: %w", ds.ErrDatabase)
			}

			// file did not return, try again for folder
			folder := ds.Folder{ID: id}
			folderRow := getFolder.QueryRow(id)
			err = folderRow.Scan(&folder.Name, &folder.Parent, &folder.Trashed)

			// no error -> thus a folder
			if err == nil {
				diff.RemovedFolders = append(diff.RemovedFolders, folder)
				continue
			}

			// we should not get a ErrNoRows when we have checked both the file and folder
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("ID does not exist from hook: %w", ds.ErrDataAnomaly)
			}

			// for some reason we could not scan the values of the folder
			return fmt.Errorf("removed folder scan err in hook: %w", ds.ErrDatabase)
		}

		return nil
	}

	return hook, &diff
}

const sqlGetFileByID = `
SELECT name, parent, trashed, size, md5 FROM file WHERE id=?
`

const sqlGetFolderByID = `
SELECT name, parent, trashed FROM folder WHERE id=?
`
