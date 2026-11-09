package accounts

import (
	"net/http"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// LiveViewer is who holds a /live stream: managers receive every event,
// anyone else only events for their own items. Without sign-in (a nil)
// everyone runs the app.
func (a *Auth) LiveViewer(r *http.Request) webserver.LiveViewer {
	who, ok := webserver.From(r.Context())
	if !ok {
		return webserver.LiveViewer{All: a == nil, Approve: a == nil}
	}
	return webserver.LiveViewer{UserID: who.UserID, All: who.Has(webserver.PermManageLibrary), Approve: who.Has(webserver.PermApprove)}
}

// LiveViewerNow is LiveViewer read again from the sign-in itself, for a
// stream that is already open: false once that sign-in no longer works.
func (a *Auth) LiveViewerNow(r *http.Request) (webserver.LiveViewer, bool) {
	if a == nil {
		return a.LiveViewer(r), true
	}
	// An administrator who must turn on two-factor first is refused every
	// route but My account, so an open stream ends too.
	who, ok := a.Still(r)
	if !ok || who.MustTwoFactor {
		return webserver.LiveViewer{}, false
	}
	return webserver.LiveViewer{UserID: who.UserID, All: who.Has(webserver.PermManageLibrary), Approve: who.Has(webserver.PermApprove)}, true
}

// LiveRecheck has every open /live stream check its viewer again after a
// write to the sign-in or people routes: a sign-out, ended sessions, a
// disabled account or changed rights end the streams they no longer fit.
func LiveRecheck(live *webserver.LiveHub, h http.Handler) http.Handler {
	if live == nil {
		return h
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r)
		if r.Method != http.MethodGet && r.Method != http.MethodHead &&
			(strings.HasPrefix(r.URL.Path, "/api/v1/auth/") || strings.HasPrefix(r.URL.Path, "/api/v1/users")) {
			live.Recheck()
		}
	})
}

// LiveRoutes serves GET /live (when live is on) and POST
// /api/v1/live/interest; a counted interest call runs wake.
func (a *Auth) LiveRoutes(m *http.ServeMux, live *webserver.LiveHub, interest *webserver.Interest, wake func()) {
	if live != nil {
		h := live.Checked(a.LiveViewerNow)
		m.Handle("GET /live", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if a != nil {
				r = a.WithSessionFamily(r)
			}
			h.ServeHTTP(w, r)
		}))
	}
	m.HandleFunc("POST /api/v1/live/interest", func(w http.ResponseWriter, r *http.Request) { interest.Serve(w, r, wake) })
}
