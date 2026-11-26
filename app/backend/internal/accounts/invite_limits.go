package accounts

import (
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Join attempts one invite link takes per window. A link pinned in a group
// chat sees a few people a minute at most; a flood of new accounts through
// it is turned away until the window passes.
const (
	inviteJoinsPer    = 10
	inviteJoinsWindow = time.Minute
	inviteJoinsKept   = 10000 // links tracked at once; the oldest windows go first
)

// inviteJoinLimit counts join attempts per invite link (by its hash) in a
// fixed window. The zero value is ready to use.
type inviteJoinLimit struct {
	mu   sync.Mutex
	seen map[string]*joinWindow
}

type joinWindow struct {
	start time.Time
	n     int
}

// allow counts one join attempt on the link. It writes 429 and returns false
// once the link has had its share of attempts in this window.
func (l *inviteJoinLimit) allow(w http.ResponseWriter, hash string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.seen == nil {
		l.seen = map[string]*joinWindow{}
	}
	c := l.seen[hash]
	if c == nil || now.Sub(c.start) >= inviteJoinsWindow || now.Before(c.start) {
		if c == nil && len(l.seen) >= inviteJoinsKept {
			for k, v := range l.seen {
				if now.Sub(v.start) >= inviteJoinsWindow {
					delete(l.seen, k)
				}
			}
			if len(l.seen) >= inviteJoinsKept {
				// Every tracked link is busy: refuse rather than grow without end.
				return l.refuse(w, inviteJoinsWindow)
			}
		}
		c = &joinWindow{start: now}
		l.seen[hash] = c
	}
	if c.n >= inviteJoinsPer {
		return l.refuse(w, inviteJoinsWindow-now.Sub(c.start))
	}
	c.n++
	return true
}

func (l *inviteJoinLimit) refuse(w http.ResponseWriter, wait time.Duration) bool {
	w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
	fail(w, http.StatusTooManyRequests, "too many tries; wait a little and try again")
	return false
}
