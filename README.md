<p align="center"><img width=450 src="banner.svg" /></p>

## Introduction

Bernard is an essential character in my Journey of Transfer narrative as he is in charge of mirroring the state of a [Shared Drive](https://support.google.com/a/answer/7212025?hl=en) to a specified datastore.
Specifically, Bernard acts as an engine to fetch changes from the Google Drive API to then propagate these changes to a datastore such as SQLite.

Journey of Transfer is a narrative I am writing with projects named after characters of Westworld. The narrative is my exploration process of the [Go language](https://golang.org), while building a programme utilising service accounts to upload and sync files to Google Drive.

Bernard is the second character of this narrative and was created to provide an alternative to [RClone](https://rclone.org) to provide local low-latency access to Google Drive metadata.

## Early Access

Bernard is provided as an early-access preview as the API may still change.
Furthermore, not all components have associated tests.

This early-access preview comes with a small CLI to visually reflect the changes Bernard picks up. Once Bernard is proven to be stable and correct, this CLI will be removed.

### Using the CLI

The CLI uses [Stubbs](https://github.com/m-rots/stubbs) to manage the authentication process.
Make sure you create a Service Account which has read access to the Shared Drive in question.
Additionally, please check whether you have the Drive API enabled in Google Cloud.
Save a JSON key of this service account and store it somewhere you can easily access the file.

Bernard will store a SQLite database file called `bernard.db` in your current working directory.
It is advised to store the JSON key of the service account in the same directory.

The CLI requires three arguments:

1. `full` or `diff`
2. The ID of the Shared Drive you want to synchronise
3. The path to the JSON key of the service account

The first argument specifies the operation, where `full` will activate a full synchronisation of the Shared Drive and `diff` will fetch the latest changes. You must fully synchronise once before fetching the differences.

The second argument takes a string as input which should be the ID of your Shared Drive.
Make sure the Service Account has read access to the Shared Drive in question.

The third argument takes a string as input which should point to the JSON key of the service account on your file system.

#### CLI example

```bash
bernard "full" "1234xxxxxxxxxxxxxVA" "./account.json"
```

In this example, a full synchronisation is activated for the Shared Drive `1234xxxxxxxxxxxxxVA` with the Service Account `./account.json`.

## Using Bernard in your Go project

### Full & Partial Synchronisation

Bernard allows two ways of synchronising the datastore.
The `FullSync()` takes a considerable amount of time depending on the number of files placed in the Shared Drive.
Bernard roughly processes 1000 files every 1-2 seconds in the full synchronisation mode.

Please note that the full synchronisation can be incomplete if you make changes to the Shared Drive in the minutes leading up to the full synchronisation.

Once you have fully synchronised the Shared Drive, you can use the `PartialSync()` to fetch the differences between the last synchronisation (both full and partial) and the current Shared Drive state.

### Hooks

Coming soon!

### Datastore

The datastore is a core component of Bernard's operations. Bernard provides a reference implementation of a Datastore in the form of a SQLite database. This reference datastore can be expanded to allow other operations on the underlying `database/sql` interface.

If SQLite is not your database of choice, feel free to open a pull request with support for another database such as MongoDB, Fauna or CockroachDB. I highly advise you to have a look at `datastore/datastore.go` and `datastore/sqlite/sqlite.go` files to get a feel for the operations the Datastore interface should perform.

### Authenticator

Bernard exports an Authenticator interface which hosts an `AccessToken` function.
This function should fetch a valid access token at all times.
It should respond with the access token as a string, its UNIX expiry time as an int64 and an error in case the credentials are invalid.

```go
type Authenticator interface {
  AccessToken() (string, int64, error)
}
```

To get started quickly, you can use [Stubbs](https://github.com/m-rots/stubbs) as it implements the Authenticator interface.

### Example code

In this example, [Stubbs](https://github.com/m-rots/stubbs) is used as the Authenticator and the reference SQLite datastore is used.

```go
package main

import (
  "fmt"
  "os"

  "github.com/m-rots/bernard"
  "github.com/m-rots/bernard/datastore/sqlite"
  "github.com/m-rots/stubbs"
)

func getAuthenticator() (bernard.Authenticator, error) {
  clientEmail := "stubbs@westworld.iam.gserviceaccount.com"
  privateKey := "-----BEGIN PRIVATE KEY-----\n..."
  scopes := []string{"https://www.googleapis.com/auth/drive.readonly"}

  priv, err := stubbs.ParseKey(privateKey)
  if err != nil {
    // invalid private key
    return nil, err
  }

  account := stubbs.New(clientEmail, &priv, scopes, 3600)
  return account, nil
}

func main() {
  // Use Stubbs as the authenticator
  authenticator, err := getAuthenticator()
  if err != nil {
    fmt.Println("Invalid private key")
    os.Exit(1)
  }

  driveID := "1234xxxxxxxxxxxxxVA"
  datastorePath := "bernard.db"

  store, err := sqlite.New(datastorePath)
  if err != nil {
    // Either the database could not be created,
    // or the SQL schema is broken somehow...
    fmt.Println("Could not create SQLite datastore")
    os.Exit(1)
  }

  bernie := bernard.New(driveID, authenticator, store)

  err = bernie.FullSync()
  if err != nil {
    fmt.Println("Could not fully synchronise the drive")
    os.Exit(1)
  }

  err = bernie.PartialSync()
  if err != nil {
    fmt.Println("Could not partially synchronise the drive")
    os.Exit(1)
  }
}
```
