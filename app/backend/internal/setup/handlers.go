package setup

import (
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles"
)

// Deps is what the setup and part gate routes need: services and core types, and funcs
// for anything a higher module owns (t14 split design, section 4).
type Deps struct {
	Store     *database.Store
	Secrets   *secrets.Vault
	Subtitles *subtitles.Service
	States    *jobs.Registry
	Parts     *parts.Control
	SafeMode  bool
	ExitApp   func(how string) error
	// KidMax is the signed-in person's age-rating limit ("" for none).
	KidMax func(*http.Request) string
	// TitleAllowed says the signed-in person may see the Library title t.
	TitleAllowed func(*http.Request, mediafiles.Title) bool
	// TestDownloader, TestSource and TestMediaServer are the link test
	// routes; TestMediaServer is nil when media servers are not available.
	TestDownloader  http.HandlerFunc
	TestSource      http.HandlerFunc
	TestMediaServer http.HandlerFunc
}

// Handlers serves the setup and part gate routes. Its state fields are private and
// zero-value ready, so &Handlers{Deps: d} works.
type Handlers struct {
	Deps
	confirm webserver.Confirms // the two-step controls' confirmation ids
}

// Routes registers the control routes (part switches and owner and admin controls).
func (h *Handlers) Routes(m *http.ServeMux) { h.controlRoutes(m) }
