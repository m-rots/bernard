package main

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"time"

	lowe "github.com/m-rots/bernard"
	"github.com/m-rots/bernard/cmd/bernard/devstore"
	ds "github.com/m-rots/bernard/datastore"
	"github.com/m-rots/stubbs"
)

type googleServiceAccount struct {
	Email      string `json:"client_email"`
	PrivateKey string `json:"private_key"`
}

const (
	colourReset  string = "\033[0m"
	colourRed    string = "\033[1;31m"
	colourGreen  string = "\033[1;32m"
	colourYellow string = "\033[1;33m"
)

func main() {
	args := os.Args[1:]

	if len(args) != 3 {
		fmt.Println("1st arg: full or diff, 2nd arg: driveID, 3rd arg: path to sa")
		os.Exit(1)
	}

	fullSync := false
	driveID := args[1]
	saPath := args[2]

	switch args[0] {
	case "full":
		fullSync = true
	case "diff":
		fullSync = false
	default:
		fmt.Println("1st arg should be 'diff' or 'full'")
		os.Exit(1)
	}

	if fullSync {
		os.Remove("./bernard.db")
	}

	store, err := devstore.New("./bernard.db")
	if err != nil {
		panic(err)
	}

	auth := getStubbs(saPath, []string{
		"https://www.googleapis.com/auth/drive",
		"https://www.googleapis.com/auth/iam",
	})

	bernard := lowe.New(driveID, auth, store)

	if fullSync {
		fmt.Println("Starting full sync, this might take a while...")
		err = bernard.FullSync()
		if err != nil {
			panic(err)
		}

		fmt.Println("Done")

		return
	}

	fmt.Println("Creating screenshot of the old state")
	oldState, err := store.CreateSnapshot()
	if err != nil {
		panic(err)
	}

	fmt.Println("Syncing changes from Google Drive")
	t0 := time.Now()

	err = bernard.PartialSync()
	if err != nil {
		panic(err)
	}

	fmt.Printf("\nTime taken to sync: %v\n", time.Since(t0).String())

	fmt.Println("Creating snapshot of the new state")

	newState, err := store.CreateSnapshot()
	if err != nil {
		panic(err)
	}

	fmt.Println("\nOld -> New")
	if reflect.DeepEqual(oldState, newState) {
		fmt.Println("EQUAL")
	} else {
		fmt.Println("NOT EQUAL")
	}

	compareScreenshots(oldState, newState)

	// if len(newState.Files) < 5000 {
	// 	fmt.Println("\nCreating full snapshot of the current Drive state")

	// 	fmt.Println("Creating in-memory datastore")
	// 	memStore, err := devstore.New("file:screenshot?mode=memory")
	// 	if err != nil {
	// 		panic(err)
	// 	}

	// 	fmt.Println("Fetching reference state")
	// 	reference := lowe.New(driveID, auth, memStore)
	// 	err = reference.FullSync()
	// 	if err != nil {
	// 		panic(err)
	// 	}

	// 	fmt.Println("Creating reference snapshot")
	// 	referenceState, err := memStore.CreateSnapshot()
	// 	if err != nil {
	// 		panic(err)
	// 	}

	// 	fmt.Println("\nBernard -> Reference")
	// 	compareScreenshots(newState, referenceState)

	// 	if reflect.DeepEqual(newState, referenceState) {
	// 		fmt.Println("EQUAL")
	// 	} else {
	// 		fmt.Println("NOT EQUAL")
	// 	}
	// }
}

func getStubbs(path string, scopes []string) *stubbs.Stubbs {
	file, err := os.Open(path)
	if err != nil {
		fmt.Println("Could not open service account")
		os.Exit(1)
	}

	decoder := json.NewDecoder(file)
	sa := new(googleServiceAccount)

	if decoder.Decode(sa) != nil {
		fmt.Println("Error decoding service account")
		os.Exit(1)
	}

	priv, err := stubbs.ParseKey(sa.PrivateKey)
	if err != nil {
		fmt.Println("Invalid private key")
		os.Exit(1)
	}

	return stubbs.New(sa.Email, &priv, scopes, 3600)
}

func createMaps(ss *devstore.Snapshot) (map[string]ds.File, map[string]ds.Folder) {
	filesByID := make(map[string]ds.File)
	foldersByID := make(map[string]ds.Folder)

	for _, f := range ss.Files {
		filesByID[f.ID] = f
	}

	for _, f := range ss.Folders {
		foldersByID[f.ID] = f
	}

	return filesByID, foldersByID
}

func changedPretty(name string, prev interface{}, next interface{}) string {
	if prev != next {
		return fmt.Sprintf("%v: %v -> %v\n", name, prev, next)
	}

	return ""
}

func compareScreenshots(oldSS *devstore.Snapshot, newSS *devstore.Snapshot) {
	oldFiles, oldFolders := createMaps(oldSS)
	newFiles, newFolders := createMaps(newSS)

	var removedFiles []ds.File
	var changedFiles []ds.File
	var addedFiles []ds.File

	// check for changed and removed files
	for _, prev := range oldFiles {
		next, ok := newFiles[prev.ID]
		if !ok {
			removedFiles = append(removedFiles, prev)
			continue
		}

		if prev.MD5 != next.MD5 ||
			prev.Name != next.Name ||
			prev.Parent != next.Parent ||
			prev.Size != next.Size ||
			prev.Trashed != next.Trashed {
			changedFiles = append(changedFiles, prev)
		}
	}

	// check for new files
	for _, next := range newFiles {
		_, ok := oldFiles[next.ID]
		if !ok {
			addedFiles = append(addedFiles, next)
		}
	}

	var removedFolders []ds.Folder
	var changedFolders []ds.Folder
	var addedFolders []ds.Folder

	for _, prev := range oldFolders {
		next, ok := newFolders[prev.ID]
		if !ok {
			removedFolders = append(removedFolders, prev)
			continue
		}

		if prev.Name != next.Name ||
			prev.Parent != next.Parent ||
			prev.Trashed != next.Trashed {
			changedFolders = append(changedFolders, prev)
		}
	}

	// check for new folders
	for _, next := range newFolders {
		_, ok := oldFolders[next.ID]
		if !ok {
			addedFolders = append(addedFolders, next)
		}
	}

	// print added folders
	if len(addedFolders) > 0 {
		fmt.Println("\nAdded folders:")
		for _, f := range addedFolders {
			fmt.Printf("%screated%s - %s - %s\n", colourGreen, colourReset, f.ID, f.Name)
		}
	}

	// print added files
	if len(addedFiles) > 0 {
		fmt.Println("\nAdded files:")
		for _, f := range addedFiles {
			fmt.Printf("%screated%s - %s - %s\n", colourGreen, colourReset, f.ID, f.Name)
		}
	}

	// print changed folders
	if len(changedFolders) > 0 {
		fmt.Println("\nChanged folders:")
		for _, prev := range changedFolders {
			var output string
			output += fmt.Sprintf("%schanged%s - %s - %s\n", colourYellow, colourReset, prev.ID, prev.Name)

			next, _ := newFolders[prev.ID]

			output += changedPretty("Name", prev.Name, next.Name)
			output += changedPretty("Parent", prev.Parent, next.Parent)
			output += changedPretty("Trashed", prev.Trashed, next.Trashed)

			fmt.Print(output)
		}
	}

	// print changed files
	if len(changedFiles) > 0 {
		fmt.Println("\nChanged files:")
		for _, prev := range changedFiles {
			var output string
			output += fmt.Sprintf("%schanged%s - %s - %s\n", colourYellow, colourReset, prev.ID, prev.Name)

			next, _ := newFiles[prev.ID]

			output += changedPretty("Name", prev.Name, next.Name)
			output += changedPretty("Parent", prev.Parent, next.Parent)
			output += changedPretty("Size", prev.Size, next.Size)
			output += changedPretty("Trashed", prev.Trashed, next.Trashed)
			output += changedPretty("MD5", prev.MD5, next.MD5)

			fmt.Print(output)
		}
	}

	// print removed folders
	if len(removedFolders) > 0 {
		fmt.Println("\nRemoved folders:")
		for _, f := range removedFolders {
			fmt.Printf("%sremoved%s - %s - %s\n", colourRed, colourReset, f.ID, f.Name)
		}
	}

	// print removed files
	if len(removedFiles) > 0 {
		fmt.Println("\nRemoved files:")
		for _, f := range removedFiles {
			fmt.Printf("%sremoved%s - %s - %s\n", colourRed, colourReset, f.ID, f.Name)
		}
	}
}
