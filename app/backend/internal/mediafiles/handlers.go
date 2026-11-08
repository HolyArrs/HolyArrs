package mediafiles

import (
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// Deps is what the Library files routes need: services and core types, and funcs
// for anything a higher module owns (t14 split design, section 4). Filled
// as the routes move out of internal/api.
type Deps struct {
	Store *database.Store
	// KidMax is the signed-in person's age-rating limit ("" for none).
	KidMax func(*http.Request) string
}

// Handlers serves the Library files routes. Its state fields are private and
// zero-value ready, so &Handlers{Deps: d} works.
type Handlers struct {
	Deps
}

// Routes registers the Library files routes; empty until they move here.
func (h *Handlers) Routes(m *http.ServeMux) {}
