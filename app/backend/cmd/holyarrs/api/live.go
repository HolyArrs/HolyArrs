package api

import (
	"net/http"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// liveViewer is who holds a /live stream: managers receive every event,
// anyone else only events for their own items. Without sign-in everyone
// runs the app.
func (a *API) liveViewer(r *http.Request) webserver.LiveViewer {
	who, ok := webserver.From(r.Context())
	if !ok {
		return webserver.LiveViewer{All: a.s.Auth == nil, Approve: a.s.Auth == nil}
	}
	return webserver.LiveViewer{UserID: who.UserID, All: who.Has(webserver.PermManageLibrary), Approve: who.Has(webserver.PermApprove)}
}

// liveViewerNow is liveViewer read again from the sign-in itself, for a
// stream that is already open: false once that sign-in no longer works.
func (a *API) liveViewerNow(r *http.Request) (webserver.LiveViewer, bool) {
	if a.s.Auth == nil {
		return a.liveViewer(r), true
	}
	// An administrator who must turn on two-factor first is refused every
	// route but My account, so an open stream ends too.
	who, ok := a.s.Auth.Still(r)
	if !ok || who.MustTwoFactor {
		return webserver.LiveViewer{}, false
	}
	return webserver.LiveViewer{UserID: who.UserID, All: who.Has(webserver.PermManageLibrary), Approve: who.Has(webserver.PermApprove)}, true
}

// liveRecheck has every open /live stream check its viewer again after a
// write to the sign-in or people routes: a sign-out, ended sessions, a
// disabled account or changed rights end the streams they no longer fit.
func (a *API) liveRecheck(h http.Handler) http.Handler {
	if a.s.Live == nil {
		return h
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r)
		if r.Method != http.MethodGet && r.Method != http.MethodHead &&
			(strings.HasPrefix(r.URL.Path, "/api/v1/auth/") || strings.HasPrefix(r.URL.Path, "/api/v1/users")) {
			a.s.Live.Recheck()
		}
	})
}

func (a *API) publishTitleView(v TitleView) { a.tv().PublishView(v) }

func (a *API) liveRoutes(m *http.ServeMux) {
	if a.s.Live != nil {
		h := a.s.Live.Checked(a.liveViewerNow)
		m.Handle("GET /live", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if a.s.Auth != nil {
				r = a.s.Auth.WithSessionFamily(r)
			}
			h.ServeHTTP(w, r)
		}))
	}
	m.HandleFunc("POST /api/v1/live/interest", a.liveInterest)
}

// liveInterest is webserver.Interest; a counted call wakes the download sync.
func (a *API) liveInterest(w http.ResponseWriter, r *http.Request) {
	a.interest.Serve(w, r, func() {
		if a.dl.DLSync != nil {
			a.dl.DLSync.Watched()
		}
	})
}

// publishTitle sends the changed title to live pages.
func (a *API) publishTitle(r *http.Request, id int64) {
	if a.s.Live == nil {
		return
	}
	var t mediafiles.Title
	if database.Get(r.Context(), a.s.Store.Read(), "titles", id, &t) != nil {
		return
	}
	if v, err := a.titleView(r.Context(), t); err == nil {
		a.publishTitleView(v)
	}
}

// versionID reads the {vid} Version id, answering 404 itself.
func versionID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	return webserver.PathID(w, r, "vid")
}
