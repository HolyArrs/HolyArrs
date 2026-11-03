package api

import (
	"net/http"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs/internal/auth"
)

// artworkMissesPerMinute caps the pictures one person can make the server
// fetch (cache misses) in a minute. Pictures already kept are not counted,
// and a first look at a full Explore page stays well below it.
var artworkMissesPerMinute = 600

// artworkBudget counts each person's artwork cache misses this minute.
type artworkBudget struct {
	mu    sync.Mutex
	start time.Time
	n     map[int64]int
}

func (b *artworkBudget) allow(who int64, now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.n == nil || now.Sub(b.start) >= time.Minute {
		b.start, b.n = now, map[int64]int{}
	}
	if b.n[who] >= artworkMissesPerMinute {
		return false
	}
	b.n[who]++
	return true
}

// artworkMissOK counts one cache miss for the person asking; past the
// budget it answers 429 and returns false.
func (a *API) artworkMissOK(w http.ResponseWriter, r *http.Request) bool {
	who, _ := auth.From(r.Context())
	if a.artMisses.allow(who.UserID, time.Now()) {
		return true
	}
	w.Header().Set("Retry-After", "60")
	http.Error(w, "too many new pictures, try again in a minute", http.StatusTooManyRequests)
	return false
}
