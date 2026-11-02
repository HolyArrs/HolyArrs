// Package sharedfill keeps the members of the shared lists stored (music
// speed plan, Phase 3): on a jittered schedule it reads who is on the
// charts, trending, new and coming-soon lists and each genre's top
// artists, and queues P2 jobs for them. The queue's worker fetches only
// what is missing or marked changed, so unchanged data is never fetched
// again. A rebuild of the lists themselves re-ranks stored data only.
package sharedfill

import (
	"context"
	"log"
	"math/rand/v2"
	"sync"
	"time"
)

// Member is one item on a shared list.
type Member struct{ Kind, MBID string }

// Filler queues the shared lists' members.
type Filler struct {
	// Lists name each shared list and read its members (no upstream call).
	Lists map[string]func(ctx context.Context) []Member
	// Enqueue queues one member at P2 (a no-op when it is already stored
	// fresh is the worker's job, not this one's).
	Enqueue func(ctx context.Context, m Member)
	// Related reads one hop of related artists for a listed artist (public
	// similarity data only); they are queued like any other member.
	Related func(ctx context.Context, m Member) []Member
	// Every is the time between passes (default 6 h, +/-10% jitter);
	// Start the wait before the first (default 1 min).
	Every, Start time.Duration
	Logf         func(string, ...any)

	mu   sync.Mutex
	last Pass
	set  map[Member]bool
}

// Pass is the last pass's numbers.
type Pass struct {
	At      time.Time      `json:"at,omitzero"`
	Members int            `json:"members"`
	Related int            `json:"related"`
	Lists   map[string]int `json:"lists"`
}

// Once reads every list and queues each member once.
func (f *Filler) Once(ctx context.Context) Pass {
	p := Pass{At: time.Now(), Lists: map[string]int{}}
	seen := map[Member]bool{}
	for name, read := range f.Lists {
		ms := read(ctx)
		p.Lists[name] = len(ms)
		for _, m := range ms {
			if seen[m] || m.MBID == "" {
				continue
			}
			seen[m] = true
			f.Enqueue(ctx, m)
		}
	}
	p.Members = len(seen)
	// Publish the list members now (joined with the last good set) so
	// readers never wait on the slow related-artist hop below.
	f.publish(seen, false)
	if f.Related != nil {
		listed := make([]Member, 0, len(seen))
		for m := range seen {
			if m.Kind == "artist" {
				listed = append(listed, m)
			}
		}
		for _, m := range listed {
			if ctx.Err() != nil {
				break
			}
			for _, r := range f.Related(ctx, m) {
				if seen[r] || r.MBID == "" || r.Kind != "artist" {
					continue
				}
				seen[r] = true
				p.Related++
				f.Enqueue(ctx, r)
			}
		}
	}
	f.mu.Lock()
	f.last = p
	f.mu.Unlock()
	f.publish(seen, ctx.Err() == nil)
	return p
}

// publish makes seen the readable set; unless the pass is complete, the
// last good set's members stay too.
func (f *Filler) publish(seen map[Member]bool, complete bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	next := make(map[Member]bool, len(seen)+len(f.set))
	if !complete {
		for m := range f.set {
			next[m] = true
		}
	}
	for m := range seen {
		next[m] = true
	}
	f.set = next
}

// Last answers the last pass.
func (f *Filler) Last() Pass {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.last
}

// Listed reports whether an item is on a current shared list.
func (f *Filler) Listed(kind, mbid string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.set[Member{kind, mbid}]
}

// Members answers the last pass's members (lists and related artists).
func (f *Filler) Members() []Member {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Member, 0, len(f.set))
	for m := range f.set {
		out = append(out, m)
	}
	return out
}

// Run passes on the schedule until ctx ends.
func (f *Filler) Run(ctx context.Context) {
	every, wait := f.Every, f.Start
	if every <= 0 {
		every = 6 * time.Hour
	}
	if wait <= 0 {
		wait = time.Minute
	}
	logf := f.Logf
	if logf == nil {
		logf = log.Printf
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		p := f.Once(ctx)
		logf("sharedfill: %d members queued (%d related) from %v", p.Members, p.Related, p.Lists)
		wait = time.Duration(float64(every) * (0.9 + 0.2*rand.Float64()))
	}
}
