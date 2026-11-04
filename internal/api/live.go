package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs/internal/auth"
	"github.com/CoderLuii/holyarrs/internal/live"
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
func (a *API) liveViewer(r *http.Request) live.Viewer {
	who, ok := auth.From(r.Context())
	if !ok {
		return live.Viewer{All: a.s.Auth == nil, Approve: a.s.Auth == nil}
	}
	return live.Viewer{UserID: who.UserID, All: who.Has(auth.PermManageLibrary), Approve: who.Has(auth.PermApprove)}
}

func (a *API) liveRoutes(m *http.ServeMux) {
	if a.s.Live != nil {
		m.Handle("GET /live", a.s.Live.Handler(a.liveViewer))
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
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": `view must be "downloads"`})
		return
	}
	key := "anon"
	if who, ok := auth.From(r.Context()); ok {
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
	if a.s.DLSync != nil {
		a.s.DLSync.Watched()
	}
	writeJSON(w, http.StatusOK, map[string]any{"ttl_seconds": 60})
}
