// Package sqlite provides a reference implementation of a Bernard datastore.
// Other SQL implementations should ideally borrow from this code as the SQL
// should be compatible with other drivers as well.
package sqlite

import (
	"database/sql"
	"fmt"
	"strings"

	ds "github.com/m-rots/bernard/datastore"

	// database driver
	_ "github.com/mattn/go-sqlite3"
)

// New returns a Bernard Datastore with a SQLite3 backend.
func New(path string) (*Datastore, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("open: %w", ds.ErrDatabase)
	}

	if _, err := db.Exec(sqlSchema); err != nil {
		return nil, fmt.Errorf("schema: %w", ds.ErrDatabase)
	}

	return &Datastore{DB: db}, nil
}

// Datastore holds our SQLite3 database connection
// and implements the Bernard Datastore interface.
type Datastore struct {
	DB *sql.DB
}

// ErrTransaction can have values begin or commit, and indicates an error
// when beginning or commiting a transaction
var ErrTransaction = fmt.Errorf("transaction: %w", ds.ErrDatabase)

// ErrInvalidStatement occurs when the SQL statement is not compatible
// with the underlying driver or when the database is not initialised with tables yet.
var ErrInvalidStatement = fmt.Errorf("invalid statement: %w", ds.ErrDatabase)

// addParameters adds the bind vars to the query string for the provided number of items.
//
// items must be >0
func addParameters(query string, items int) string {
	i := strings.IndexByte(query, '?') + 1

	var str strings.Builder
	str.Grow(len(query) + len(", ?")*(items-1))
	str.WriteString(query[:i])

	for i := 0; i < items-1; i++ {
		str.WriteString(", ?")
	}

	str.WriteString(query[i:])
	return str.String()
}

// FullSync synchronises the provided Drive state to the datastore.
func (store *Datastore) FullSync(drive ds.Drive, folders []ds.Folder, files []ds.File) (err error) {
	// Start transaction so all statements can be rolled back.
	tx, err := store.DB.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", ErrTransaction)
	}

	// Prepare sql statement to upsert folders.
	upsertFolder, err := tx.Prepare(sqlUpsertFolder)
	if err != nil {
		return fmt.Errorf("%v: %w", sqlUpsertFolder, ErrInvalidStatement)
	}

	// Prepare sql statement to upsert files.
	upsertFile, err := tx.Prepare(sqlUpsertFile)
	if err != nil {
		return fmt.Errorf("%v: %w", sqlUpsertFile, ErrInvalidStatement)
	}

	// Prepare sql statement to upsert a variable (pageToken).
	upsertDrive, err := tx.Prepare(sqlUpsertDrive)
	if err != nil {
		return fmt.Errorf("%v: %w", sqlUpsertDrive, ErrInvalidStatement)
	}

	// Update the pageToken for future sync jobs.
	// TODO(m-rots) error should not be a data anomaly
	_, err = upsertDrive.Exec(drive.ID, drive.PageToken)
	if err != nil {
		return fmt.Errorf("pageToken: %w", ds.ErrDataAnomaly)
	}

	// Insert the Shared Drive as the root folder.
	_, err = upsertFolder.Exec(drive.ID, drive.ID, drive.Name, nil, false)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("%v: %w", drive.ID, ds.ErrDataAnomaly)
	}

	// Upsert all folders.
	// Rollback when a data anomaly is detected (such as a FOREIGN KEY constraint).
	for _, f := range folders {
		_, err = upsertFolder.Exec(f.ID, drive.ID, f.Name, f.Parent, f.Trashed)

		if err != nil {
			tx.Rollback()
			return fmt.Errorf("%v: %w", f.ID, ds.ErrDataAnomaly)
		}
	}

	// Upsert all files.
	// Rollback when a data anomaly is detected (such as a FOREIGN KEY constraint).
	for _, f := range files {
		_, err = upsertFile.Exec(f.ID, drive.ID, f.Name, f.MD5, f.Parent, f.Size, f.Trashed)

		if err != nil {
			tx.Rollback()
			return fmt.Errorf("%v: %w", f.ID, ds.ErrDataAnomaly)
		}
	}

	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("commit: %w", ErrTransaction)
	}

	return nil
}

// PartialSync synchronises the provided changes to the datastore.
//
// 1. Update the pageToken and (if applicable) the name of the Shared Drive.
//
// 2. Process changed folders with UPSERT.
//
// 3. Process changed folders with UPSERT.
//
// 4. Remove any items of which the IDs match with the removedIDs slice.
func (store *Datastore) PartialSync(drive ds.Drive, changedFolders []ds.Folder, changedFiles []ds.File, removedIDs []string) error {
	tx, err := store.DB.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", ErrTransaction)
	}

	// Prepare sql statement to upsert folders.
	upsertFolder, err := tx.Prepare(sqlUpsertFolder)
	if err != nil {
		return fmt.Errorf("%v: %w", sqlUpsertFolder, ErrInvalidStatement)
	}

	// Prepare sql statement to upsert files.
	upsertFile, err := tx.Prepare(sqlUpsertFile)
	if err != nil {
		return fmt.Errorf("%v: %w", sqlUpsertFile, ErrInvalidStatement)
	}

	// Prepare sql statement to upsert a variable (pageToken).
	upsertDrive, err := tx.Prepare(sqlUpsertDrive)
	if err != nil {
		return fmt.Errorf("%v: %w", sqlUpsertDrive, ErrInvalidStatement)
	}

	// Update the pageToken for future sync jobs.
	_, err = upsertDrive.Exec(drive.ID, drive.PageToken)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("pageToken: %w", ds.ErrDataAnomaly)
	}

	// Drive name is empty if not changed, so when not empty we should update it.
	if drive.Name != "" {
		_, err = upsertFolder.Exec(drive.ID, drive.ID, drive.Name, nil, false)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("%v: %w", drive.ID, ds.ErrDataAnomaly)
		}
	}

	// upsert all changed folders and change childrens' trashed state
	for _, f := range changedFolders {
		_, err := upsertFolder.Exec(f.ID, drive.ID, f.Name, f.Parent, f.Trashed)

		if err != nil {
			tx.Rollback()
			return fmt.Errorf("%v: %w", f.ID, ds.ErrDataAnomaly)
		}
	}

	// upsert all changed files
	for _, f := range changedFiles {
		_, err = upsertFile.Exec(f.ID, drive.ID, f.Name, f.MD5, f.Parent, f.Size, f.Trashed)

		if err != nil {
			tx.Rollback()
			return fmt.Errorf("%v: %w", f.ID, ds.ErrDataAnomaly)
		}
	}

	if len(removedIDs) > 0 {
		// convert []string to []interface{} as Exec requires a []interface{} input
		args := make([]interface{}, len(removedIDs)+1)
		for i, id := range removedIDs {
			args[i] = id
		}

		// append DriveID for the WHERE clause
		args[len(removedIDs)] = drive.ID

		// first try to delete all files to prevent data anomalies
		deleteFiles := addParameters(sqlDeleteFiles, len(removedIDs))

		_, err = tx.Exec(deleteFiles, args...)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("deleting files: %w", ds.ErrDataAnomaly)
		}

		// then try to delete all folders, which should have no files as children now
		deleteFolders := addParameters(sqlDeleteFolders, len(removedIDs))

		_, err = tx.Exec(deleteFolders, args...)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("deleting folders: %w", ds.ErrDataAnomaly)
		}
	}

	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("commit: %w", ErrTransaction)
	}

	return nil
}

// PageToken retrieves the pageToken the datastore currently reflects.
func (store *Datastore) PageToken(driveID string) (string, error) {
	var pageToken string

	row := store.DB.QueryRow(sqlGetPageToken, driveID)
	if err := row.Scan(&pageToken); err != nil {
		return "", ds.ErrFullSync
	}

	return pageToken, nil
}

const sqlSchema string = `
PRAGMA foreign_keys=ON;

CREATE TABLE IF NOT EXISTS file (
	"id" text NOT NULL,
	"drive" text NOT NULL,
	"name" text NOT NULL,
	"parent" text NOT NULL,
	"size" integer NOT NULL,
	"md5" text NOT NULL,
	"trashed" boolean NOT NULL,
	PRIMARY KEY(id, drive),
	FOREIGN KEY(parent, drive) REFERENCES folder(id, drive) DEFERRABLE INITIALLY IMMEDIATE
);

CREATE TABLE IF NOT EXISTS folder (
	"id" text NOT NULL,
	"drive" text NOT NULL,
  "name" text NOT NULL,
  "trashed" boolean NOT NULL,
	"parent" text,
	PRIMARY KEY(id, drive),
  FOREIGN KEY(parent, drive) REFERENCES folder(id, drive) DEFERRABLE INITIALLY IMMEDIATE
);

CREATE TABLE IF NOT EXISTS drive (
	"id" text NOT NULL,
	"pageToken" text NOT NULL,
	PRIMARY KEY(id)
)
`

const sqlUpsertDrive = `
INSERT INTO drive (id, pageToken) VALUES (?, ?)
	ON CONFLICT(id) DO UPDATE SET
		pageToken=excluded.pageToken
`

const sqlUpsertFolder = `
INSERT INTO folder (id, drive, name, parent, trashed) VALUES (?, ?, ?, NULLIF(?, ""), ?)
	ON CONFLICT(id, drive) DO UPDATE SET
		name=excluded.name,
		parent=excluded.parent,
		trashed=excluded.trashed
`

const sqlUpsertFile = `
INSERT INTO file (id, drive, name, md5, parent, size, trashed) VALUES (?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id, drive) DO UPDATE SET
    name=excluded.name,
    md5=excluded.md5,
    parent=excluded.parent,
    size=excluded.size,
		trashed=excluded.trashed
`

const sqlDeleteFiles = `
DELETE FROM file WHERE id IN (?) AND drive=? 
`

const sqlDeleteFolders = `
DELETE FROM folder WHERE id IN (?) AND drive=?
`

const sqlGetPageToken = `
SELECT pageToken FROM drive WHERE id=?
`
