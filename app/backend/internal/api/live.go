package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// interestEvery is how often one session's interest counts; more calls in
// between answer 204 and change nothing.
const interestEvery = 10 * time.Second

// interestState remembers when each session last said it watches a view.
type interestState struct {
	mu   sync.Mutex
	last map[string]time.Time
}

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

// publishTitleView tells managers' pages a title changed. Its asks name who
// asked and who decided, with the decline note, so they stay out: pages
// read the title again from its route, which shows them to approvers only.
func (a *API) publishTitleView(v TitleView) {
	if a.s.Live == nil {
		return
	}
	v.Asks = nil
	a.s.Live.Publish("title", v)
}

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

// liveInterest records that a signed-in page shows a view that wants fast
// updates (today: downloads). Each call counts for 60 seconds; the leader
// tab repeats it every 30 seconds.
func (a *API) liveInterest(w http.ResponseWriter, r *http.Request) {
	var in struct {
		View string `json:"view"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&in); err != nil || in.View != "downloads" {
		webserver.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": `view must be "downloads"`})
		return
	}
	key := "anon"
	if who, ok := webserver.From(r.Context()); ok {
		key = who.Via + ":" + strconv.FormatInt(who.UserID, 10) + ":" + who.CSRF
	}
	now := time.Now()
	a.interest.mu.Lock()
	if a.interest.last == nil {
		a.interest.last = map[string]time.Time{}
	}
	if t, ok := a.interest.last[key]; ok && now.Sub(t) < interestEvery {
		a.interest.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
		return
	}
	for k, t := range a.interest.last {
		if now.Sub(t) > time.Minute {
			delete(a.interest.last, k)
		}
	}
	a.interest.last[key] = now
	a.interest.mu.Unlock()
	if a.dl.DLSync != nil {
		a.dl.DLSync.Watched()
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"ttl_seconds": 60})
}
