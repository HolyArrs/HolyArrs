package webserver

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// interestEvery is how often one session's interest counts; more calls in
// between answer 204 and change nothing.
const interestEvery = 10 * time.Second

// Interest remembers when each session last said it watches a view that
// wants fast updates (today: downloads). Zero value is ready.
type Interest struct {
	mu   sync.Mutex
	last map[string]time.Time
}

// Serve answers POST /api/v1/live/interest. Each call counts for 60
// seconds; the leader tab repeats it every 30 seconds. watched (may be
// nil) runs when the call counts.
func (i *Interest) Serve(w http.ResponseWriter, r *http.Request, watched func()) {
	var in struct {
		View string `json:"view"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&in); err != nil || in.View != "downloads" {
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": `view must be "downloads"`})
		return
	}
	key := "anon"
	if who, ok := From(r.Context()); ok {
		key = who.Via + ":" + strconv.FormatInt(who.UserID, 10) + ":" + who.CSRF
	}
	now := time.Now()
	i.mu.Lock()
	if i.last == nil {
		i.last = map[string]time.Time{}
	}
	if t, ok := i.last[key]; ok && now.Sub(t) < interestEvery {
		i.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
		return
	}
	for k, t := range i.last {
		if now.Sub(t) > time.Minute {
			delete(i.last, k)
		}
	}
	i.last[key] = now
	i.mu.Unlock()
	if watched != nil {
		watched()
	}
	WriteJSON(w, http.StatusOK, map[string]any{"ttl_seconds": 60})
}
