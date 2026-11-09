package setup

import (
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// Deps is what the setup and part gate routes need: services and core types, and funcs
// for anything a higher module owns.
type Deps struct {
	Store *database.Store
	// Parts switches the parts (nil: every part on, no switches).
	Parts *parts.Control
	// SafeMode says the app started with only the core.
	SafeMode bool
	// States holds the state of every supervised stage (may be nil).
	States *jobs.Registry
	// ExitApp stops the process for a whole-app restart.
	ExitApp func(how string) error
	// TitleAllowed says the signed-in person may see the Library title.
	TitleAllowed func(*http.Request, mediafiles.Title) bool
	// The test routes a link restart runs, per kind; a nil
	// TestMediaServer means media servers are not available here.
	TestDownloader, TestSource, TestMediaServer, TestSubtitleSite http.HandlerFunc
}

// Handlers serves the setup and part gate routes. Its state fields are private and
// zero-value ready, so &Handlers{Deps: d} works.
type Handlers struct {
	Deps
	// confirm holds the two-step controls' confirmation ids.
	confirm webserver.Confirms
}
