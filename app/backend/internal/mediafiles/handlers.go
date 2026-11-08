package mediafiles

import (
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Deps is what the Library files routes need: services and core types, and funcs
// for anything a higher module owns (t14 split design, section 4). Filled
// as the routes move out of internal/api.
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
	m.HandleFunc("GET /api/v1/library/copies", h.listLibraryCopies)
	m.HandleFunc("POST /api/v1/library/copies/search/preview", h.previewLibraryCopySearch)
	m.HandleFunc("POST /api/v1/library/copies/search", h.searchLibraryCopies)
}
