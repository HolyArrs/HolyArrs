package api

import (
	"context"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/parts"
	setupmod "github.com/HolyArrs/holyarrs/app/backend/internal/setup"
)

// The control, part gate and report gate routes live in setup; these are
// the small pieces api pages still call.

// partOffBody is what a route of a part that is off answers.
func partOffBody(o parts.Owner) map[string]string { return setupmod.PartOffBody(o) }

// partIsOn reports whether the part p is on (always, without a Control).
// In safe mode the Library still lists every title (parts.SafeLibrary).
func (a *API) partIsOn(p parts.Part) bool { return parts.PartOn(a.s.Parts, p) }

// pathValue picks the path segment that stands where name is in pattern.
func pathValue(pattern, path, name string) string { return setupmod.PathValue(pattern, path, name) }

// whoName names the person for an Event summary.
func whoName(r *http.Request) string { return setupmod.WhoName(r) }

// followsChanged drops a person's music rows built from their follows.
func (a *API) followsChanged(userID int64) {
	a.musicv2.days.Invalidate(userID)
	a.forgetMusicV7Snaps(userID)
}

// viewer is the signed-in person's id (1, the owner, without sign-in).
func viewer(ctx context.Context) int64 { return setupmod.Viewer(ctx) }

// setupHandlers is the setup module's routes, built once on first use.
func (a *API) setupHandlers() *setupmod.Handlers {
	if a.setupH == nil {
		d := setupmod.Deps{Store: a.s.Store, Secrets: a.s.Secrets, Subtitles: a.s.Subtitles, States: a.s.States,
			Parts: a.s.Parts, SafeMode: a.s.SafeMode, ExitApp: a.s.ExitApp, KidMax: a.kidMax, TitleAllowed: a.titleAllowed,
			TestDownloader: a.dl.TestDownloader, TestSource: func(w http.ResponseWriter, r *http.Request) { a.sourceHandlers().TestSource(w, r) }}
		if a.s.Media != nil {
			d.TestMediaServer = func(w http.ResponseWriter, r *http.Request) { a.media.Test(w, r) }
		}
		a.setupH = &setupmod.Handlers{Deps: d}
	}
	return a.setupH
}
