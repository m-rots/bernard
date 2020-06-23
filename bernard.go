package bernard

import (
	"errors"
	"net/http"
	"time"

	ds "github.com/m-rots/bernard/datastore"
)

// Authenticator represents any struct which can create an access token on demand
type Authenticator interface {
	AccessToken() (string, int64, error)
}

// Bernard is a synchronisation backend for Google Drive.
type Bernard struct {
	driveID string
	fetch   fetcher
	store   ds.Datastore
}

// New creates a new instance of Bernard
func New(driveID string, auth Authenticator, store ds.Datastore) *Bernard {
	const baseURL string = "https://www.googleapis.com/drive/v3"

	fetch := fetcher{
		auth:    auth,
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
		driveID: driveID,
		sleep:   time.Sleep,
	}

	return &Bernard{
		driveID: driveID,
		fetch:   fetch,
		store:   store,
	}
}

// ErrInvalidCredentials can occur when the wrong authentication scopes are used,
// the access token does not have access to the specified resource, or the token
// is simply invalid or expired.
var ErrInvalidCredentials = errors.New("bernard: invalid credentials")

// ErrNotFound only occurs when the provided auth does not have access to the
// Shared Drive or if the Shared Drive does not exist.
var ErrNotFound = errors.New("bernard: cannot find Shared Drive")

// ErrNetwork is the result of a networking error while contacting the Google Drive API.
// This error is only thrown on status codes not equal to 200, 401 and 500.
var ErrNetwork = errors.New("bernard: network related error")
