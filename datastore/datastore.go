// Package datastore provides the file and folder representations used in Bernard.
//
// In addition, it provides the Datastore interface Bernard interacts with.
// The datastore interface can be implemented with other databases in mind,
// though a SQLite reference datastore does exist, which could work with other SQL
// drivers as well.
//
// Finally, this package also serves three common errors which may occur
// at the datastore layer.
package datastore

import (
	"errors"
)

// Folder is a minimal representation of a file with mimeType `application/vnd.google-apps.folder`
// within Google Drive.
type Folder struct {
	ID      string
	Name    string
	Parent  string
	Trashed bool
}

// File is a minimal representation of all other files within Google Drive which do not have
// the mimeType `application/vnd.google-apps.folder`.
type File struct {
	ID      string
	Name    string
	Parent  string
	Trashed bool
	Size    int
	MD5     string
}

// Drive is a minimal representation of the Shared Drive itself.
type Drive struct {
	ID        string
	Name      string
	PageToken string
}

// The Datastore is the storage engine interface used in Bernard.
//
// The Datastore is in charge of both handling the sync operations correctly.
//
// A pageToken is stored within the datastore as well. The pageToken acts as version control
// in the sense that each "write" to the datastore should have a matched pageToken.
type Datastore interface {
	// FullSync creates a transaction to batch insert all the folders and files.
	//
	// 1. FullSync should save the pageToken and insert the driveID as a root folder.
	//
	// 2. The files and folders are inserted.
	//
	// If an error occurs during the inserting, such as a foreign key constraint,
	// the entire transaction should be rolled back.
	FullSync(drive Drive, folders []Folder, files []File) error

	// PartialSync merges all the differences in one transaction.
	//
	// 1. Changed folders are upserted in the database.
	//
	// 2. Changed files are upserted in the database.
	//
	// 3. Removed IDs are recursively deleted from the database
	//
	// 4. The pageToken is saved and the transaction is commited.
	//
	// If an error occurs during the inserting, such as a foreign key constraint,
	// the entire transaction should be rolled back.
	PartialSync(drive Drive, changedFolders []Folder, changedFiles []File, removedIDs []string) error

	// PageToken returns the pageToken of the specified driveID.
	PageToken(driveID string) (string, error)
}

// ErrDataAnomaly indicates an error in the relationship constraints within the datastore.
// This error might occur when the Google Drive API has not processed all changes yet,
// and therefore returns an incomplete list of changes.
//
// When one encounters this error, it is best to wait a couple of seconds before retrying
// the same operation as Google Drive has to process the changes first.
//
// If this error does not resolve, it might be a flaw in the logic of Bernard.
// In that case, please open an issue.
var ErrDataAnomaly = errors.New("datastore: data anomaly")

// ErrDatabase indicates a fatal error within the datastore.
//
// If this error is encountered, the programme should panic and the datastore
// implementation must be looked at.
var ErrDatabase = errors.New("datastore: database related error")

// ErrFullSync indicates the database is missing the pageToken variable,
// which is exclusively the result of not running a full sync beforehand.
var ErrFullSync = errors.New("datastore: requires full sync")
