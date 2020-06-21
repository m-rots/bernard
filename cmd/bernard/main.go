package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"

	lowe "github.com/m-rots/bernard"
	"github.com/m-rots/bernard/cmd/bernard/devstore"
	ds "github.com/m-rots/bernard/datastore"
	"github.com/m-rots/bernard/datastore/sqlite"
	"github.com/m-rots/stubbs"
)

type googleServiceAccount struct {
	Email      string `json:"client_email"`
	PrivateKey string `json:"private_key"`
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

const (
	colourReset   string = "\u001b[0m"
	colourRed     string = "\u001b[31;1m"
	colourGreen   string = "\u001b[32;1m"
	colourYellow  string = "\u001b[33;1m"
	colourMagenta string = "\u001b[35;1m"
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
		fmt.Printf("%slog%s - Starting full sync for the first time\n", colourMagenta, colourReset)
		fmt.Println("A full sync takes about 1-2 seconds for every 1000 files. This could take a while...")
		err = bernard.FullSync()
		if err != nil {
			if errors.Is(err, ds.ErrDataAnomaly) {
				fmt.Printf("\n%swarning%s - A data anomaly occured.\n", colourYellow, colourReset)
				fmt.Println("When critical changes were made to a Drive, this can happen. Please re-run the full-sync.")
				os.Exit(1)
			}

			panic(err) // what the hell happened here!
		}

		fmt.Printf("\n%slog%s - Successful full sync\n", colourMagenta, colourReset)

		return
	}

	fmt.Printf("%slog%s - Creating screenshot of the old state\n", colourMagenta, colourReset)
	oldState, err := store.CreateSnapshot(driveID)
	if err != nil {
		panic(err) // no error should occur here
	}

	fmt.Printf("%slog%s - Syncing changes from Google Drive\n", colourMagenta, colourReset)

	hook, diff := store.NewDifferencesHook()
	err = bernard.PartialSync(hook)
	if err != nil {
		if errors.Is(err, ds.ErrDataAnomaly) {
			fmt.Printf("\n%swarning%s - A data anomaly occured. Please try again in 30 seconds.\n", colourYellow, colourReset)
			fmt.Println("If this warning is still visible after multiple retries, please open an issue.")
			os.Exit(1)
		}

		panic(err) // no error should occur here
	}

	fmt.Printf("%slog%s - Creating snapshot of the new state\n\n", colourMagenta, colourReset)

	newState, err := store.CreateSnapshot(driveID)
	if err != nil {
		panic(err) // no error should occur here
	}

	if len(newState.Files) < 10000 {
		fmt.Printf("%slog%s - Comparing local datastore against full sync as there are less than 10.000 files\n", colourMagenta, colourReset)

		fmt.Printf("%slog%s - Creating in-memory datastore\n", colourMagenta, colourReset)
		memStore, err := devstore.New("file:screenshot?mode=memory")
		if err != nil {
			panic(err) // no error should occur here
		}

		fmt.Printf("%slog%s - Running full sync to act as reference state\n", colourMagenta, colourReset)
		reference := lowe.New(driveID, auth, memStore)
		err = reference.FullSync()
		if err != nil {
			if errors.Is(err, ds.ErrDataAnomaly) {
				fmt.Printf("\n%swarning%s - Changes were propagated but a data anomaly occured during the full sync.\n", colourYellow, colourReset)
				fmt.Println("It is normal behaviour for the changes to not appear when re-trying the sync.")
				fmt.Println("The changes were made to the database successfully and no data anomaly exists locally.")
				os.Exit(1)
			}

			panic(err) // no error should occur here
		}

		fmt.Printf("%slog%s - Creating reference snapshot\n", colourMagenta, colourReset)
		referenceState, err := memStore.CreateSnapshot(driveID)
		if err != nil {
			panic(err) // no error should occur here
		}

		if reflect.DeepEqual(newState, referenceState) {
			fmt.Printf("\n%slog%s - Local and remote states are equal\n", colourMagenta, colourReset)
		} else {
			fmt.Printf("\n%swarning%s - Local and remote states are not equal, please wait for propagation\n", colourYellow, colourReset)
			fmt.Printf("Is this message still appearing after a retry with more than 5 minutes in-between? Please create an issue!\n\n")
		}
	}

	if reflect.DeepEqual(oldState, newState) {
		fmt.Printf("%slog%s - Old and new states are equal\n", colourMagenta, colourReset)
	} else {
		fmt.Printf("%slog%s - Old and new states are not equal, differences should be visible\n", colourMagenta, colourReset)
	}

	printDifference(diff)
}

func printDifference(diff *sqlite.Difference) {
	// print added folders
	if len(diff.AddedFolders) > 0 {
		fmt.Println("\nAdded folders:")
		for _, f := range diff.AddedFolders {
			fmt.Printf("%screated%s - %s - %s\n", colourGreen, colourReset, f.ID, f.Name)
		}
	}

	// print added files
	if len(diff.AddedFiles) > 0 {
		fmt.Println("\nAdded files:")
		for _, f := range diff.AddedFiles {
			fmt.Printf("%screated%s - %s - %s\n", colourGreen, colourReset, f.ID, f.Name)
		}
	}

	// print changed folders
	if len(diff.ChangedFolders) > 0 {
		fmt.Println("\nChanged folders:")
		for _, f := range diff.ChangedFolders {
			fmt.Printf("%schanged%s - %s - %s\n", colourYellow, colourReset, f.ID, f.Name)
		}
	}

	// print changed files
	if len(diff.ChangedFiles) > 0 {
		fmt.Println("\nChanged files:")
		for _, f := range diff.ChangedFiles {
			fmt.Printf("%schanged%s - %s - %s\n", colourYellow, colourReset, f.ID, f.Name)
		}
	}

	// print removed folders
	if len(diff.RemovedFolders) > 0 {
		fmt.Println("\nRemoved folders:")
		for _, f := range diff.RemovedFolders {
			fmt.Printf("%sremoved%s - %s - %s\n", colourRed, colourReset, f.ID, f.Name)
		}
	}

	// print removed files
	if len(diff.RemovedFiles) > 0 {
		fmt.Println("\nRemoved files:")
		for _, f := range diff.RemovedFiles {
			fmt.Printf("%sremoved%s - %s - %s\n", colourRed, colourReset, f.ID, f.Name)
		}
	}
}
