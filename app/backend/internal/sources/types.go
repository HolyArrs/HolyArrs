package sources

import (
	"context"
	"net/http"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// Publisher sends live events.
type Publisher interface {
	Publish(topic string, data any) uint64
}

// SecretReader opens stored secrets.
type SecretReader interface {
	Get(ctx context.Context, id int64) (string, error)
}

// Catalog is what the routes ask of the library: a Title from the request
// path and the plain-words names of Slots and Parts. The library sits below
// this package, so the program wires it in.
type Catalog interface {
	// TitleFromPath reads the {id} path value and answers 404 itself when
	// there is no such Title.
	TitleFromPath(w http.ResponseWriter, r *http.Request) (mediafiles.Title, bool)
	EpisodeLabels(ctx context.Context, slotIDs []int64) string
	AlbumLabels(ctx context.Context, slotIDs []int64) string
	PartLabel(ctx context.Context, partID int64) (titleID int64, title, season, album string)
}

// OldApps is the arr (Sonarr and Radarr style) side of connecting
// Prowlarr: its keys and its on switch live outside this package.
type OldApps interface {
	// CanMakeKeys is false when the app runs without sign-in, so no key can be made.
	CanMakeKeys() bool
	// KeySides says which sides (sonarr, radarr) already have a key.
	KeySides(ctx context.Context) (map[string]bool, error)
	MakeKey(r *http.Request, side string) (string, error)
	DeleteKey(r *http.Request, side string) error
	On(ctx context.Context) bool
	SetOn(ctx context.Context, by domain.Actor, on bool) error
}
