package requests

import (
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// Deps is what the Requests routes need. Anything a higher module owns
// comes in as a func, so this package never imports that module.
type Deps struct {
	Store *database.Store
	// KidMax is the signed-in person's age-rating limit ("" for none).
	KidMax func(*http.Request) string
}

// Handlers serves the Requests routes. Its state fields are private and
// zero-value ready, so &Handlers{Deps: d} works.
type Handlers struct {
	Deps
}
