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
	safeSleep time.Duration

	fetch *fetcher
	store ds.Datastore
}

// An Option can override some of the default Bernard values.
type Option func(*Bernard)

// WithClient allows one to override the default HTTP client.
func WithClient(client *http.Client) Option {
	return func(bernard *Bernard) {
		bernard.fetch.client = client
	}
}

// WithSafeSleep allows one to sleep between the pageToken fetch and
// the full sync. Setting this between 1 and 5 minutes prevents
// any data from going rogue when changes are actively being made
// to the Shared Drive.
//
// The default value of safeSleep is set at 0.
func WithSafeSleep(duration time.Duration) Option {
	return func(bernard *Bernard) {
		bernard.safeSleep = duration
	}
}

// New creates a new instance of Bernard
func New(auth Authenticator, store ds.Datastore, opts ...Option) *Bernard {
	const baseURL string = "https://www.googleapis.com/drive/v3"

	fetch := &fetcher{
		auth:    auth,
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
		sleep: time.Sleep,
	}

	bernard := &Bernard{
		fetch: fetch,
		store: store,
	}

	for _, opt := range opts {
		opt(bernard)
	}

	return bernard
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
