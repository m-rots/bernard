package bernard

import (
	ds "github.com/m-rots/bernard/datastore"
)

// FullSync syncs the entire content of Drive to the datastore.
func (bernard *Bernard) FullSync(driveID string) error {
	startPageToken, err := bernard.fetch.pageToken(driveID)
	if err != nil {
		return err
	}

	// To prevent possible missing data, a sleep of 1-5 minutes
	// between the pageToken and FullSync can be enabled.
	if bernard.safeSleep > 0 {
		bernard.fetch.sleep(bernard.safeSleep)
	}

	name, err := bernard.fetch.drive(driveID)
	if err != nil {
		return err
	}

	drive := ds.Drive{
		ID:        driveID,
		Name:      name,
		PageToken: startPageToken,
	}

	folders, files, err := bernard.fetch.allContent(driveID)
	if err != nil {
		return err
	}

	err = bernard.store.FullSync(drive, folders, files)
	if err != nil {
		return err
	}

	return nil
}

// Hook allows the injection of functions between the fetch and datastore operations.
//
// The hook provides the changes as provided by Google, which could contain data anomalies.
//
// The first hook parameter, Drive, always provides the ID of the Drive.
// If the name of the Drive is changed in a partial sync, the drive.Name will be updated to
// reflect the new value. If the name did not change, then drive.Name is an empty string.
//
// The second parameter, files, contains all the updated files in their new state.
//
// The third parameter, folders, contains all the updated folders in their new state.
// Sometimes Google says a folder has changed when it actually has not.
// If you want to be sure that the folder changed, compare the old datastore state versus
// the new state as provided in this slice of folders.
//
// The fourth parameter, removed, contains all the removed files and folders by ID.
// Google does not provide the last known state the files and folders were in.
// So to check the state of the removed items, use the datastore to get their last-known state.
type Hook = func(drive ds.Drive, files []ds.File, folders []ds.Folder, removed []string) error

// PartialSync syncs the latest changes within the Drive to the underlying datastore.
//
// Optionally, you can provide one or multiple Hooks to get insight into the fetched changes.
func (bernard *Bernard) PartialSync(driveID string, hooks ...Hook) error {
	pageToken, err := bernard.store.PageToken(driveID)
	if err != nil {
		return err
	}

	diff, err := bernard.fetch.changedContent(driveID, pageToken)
	if err != nil {
		return err
	}

	if pageToken == diff.Drive.PageToken {
		return nil
	}

	for _, hk := range hooks {
		err = hk(diff.Drive, diff.ChangedFiles, diff.ChangedFolders, diff.RemovedIDs)
		if err != nil {
			return err
		}
	}

	err = bernard.store.PartialSync(diff.Drive, diff.ChangedFolders, diff.ChangedFiles, diff.RemovedIDs)
	if err != nil {
		return err
	}

	return nil
}
