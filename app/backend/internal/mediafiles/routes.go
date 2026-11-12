package mediafiles

import (
	"context"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Deps is what the Library files routes need: services and core types, and funcs
// for anything a higher module owns.
type Deps struct {
	Store *database.Store
	Queue *jobs.Queue
	// Metadata answers the TMDB rating lookups for Kid limits.
	Metadata metadata.Provider
	// KidMax is the signed-in person's age-rating limit ("" for none).
	KidMax func(*http.Request) string
	// DetailsRating is a rating a cached details page already knows
	// (discover's cache); nil for none.
	DetailsRating func(kind, tmdbID string) (string, bool)
	// KindShown says whether a title kind's part is on (or safe mode
	// shows every part); nil shows all.
	KindShown func(kind string) bool
	// TitleView is how a Library title is shown (api's title view until
	// the join step).
	TitleView func(ctx context.Context, t Title) (any, error)
	// TitleBatch readies ctx for many TitleView calls (api's lookup memo);
	// nil for none.
	TitleBatch func(ctx context.Context) context.Context
	// AlbumNames hands the albums a search matched to the music picture
	// warmer; nil for none.
	AlbumNames func([]AlbumName)
	// CardArt is the card pictures for the titles (discover).
	CardArt func(r *http.Request, titles []Title) map[string]metadata.CardArt
	// RouteUsable says the signed-in person may use the route pattern, so
	// KidGate leaves the 403 to the permission gate.
	RouteUsable func(r *http.Request, pattern string) bool
}

// Handlers serves the Library files routes. Its state fields are private and
// zero-value ready, so &Handlers{Deps: d} works.
type Handlers struct {
	Deps
	// rated keeps TMDB ratings looked up for Kid limits.
	rated ratingState
}

// Routes registers the Library files routes.
func (h *Handlers) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/library/search", h.librarySearch)
	m.HandleFunc("GET /api/v1/library/copies", h.listLibraryCopies)
	m.HandleFunc("GET /api/v1/library/stats", h.libraryStats)
	m.HandleFunc("POST /api/v1/library/copies/search/preview", h.previewLibraryCopySearch)
	m.HandleFunc("POST /api/v1/library/copies/search", h.searchLibraryCopies)
}
