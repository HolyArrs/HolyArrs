// Package dlsync keeps one in-memory snapshot per download client and works
// out what changed between reads. In shadow mode (setting downloads.sync =
// shadow) it is fed only from lists the Manager already reads, so it adds
// no call to any client.
package dlsync

import (
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/downloader"
)

// FreshCeiling is the longest a read counts as fresh, whatever the cadence.
const FreshCeiling = 2 * time.Minute

// Snapshot is one client as last read.
//
// Complete: the last read returned the whole list without error. Fresh:
// Complete and ReadAt within min(3 x the client's cadence, FreshCeiling)
// of now. Stale: the items were loaded from dl_items after a start and no
// read has replaced them yet. Running: a reader is following the client.
type Snapshot struct {
	Gen      uint64
	Rev      uint64
	Items    map[string]downloader.Job
	Complete bool
	ReadAt   time.Time
	Fresh    bool
	Stale    bool
	Running  bool
	Fallback bool
	Failed   bool
	Shared   bool
}

type client struct {
	gen      uint64
	rev      uint64
	items    map[string]downloader.Job
	complete bool
	readAt   time.Time
	scope    string
	floor    time.Duration // this client's read cadence
	running  bool
	since    time.Time // reader start or last good read: the degraded bound counts from here
	fallback bool
	stale    bool
	failed   bool
	shared   bool
}

// viewerTTL is how long one view of the Downloads page counts as a viewer.
const viewerTTL = 60 * time.Second

// Store holds the snapshots. The zero value is ready; a nil *Store answers
// every Current with Complete=false, so callers need no checks.
type Store struct {
	// Floor is the reader's floor cadence; zero uses FreshCeiling alone.
	// A client's own cadence, when set, takes its place.
	Floor time.Duration
	// Now is for tests.
	Now func() time.Time

	mu       sync.Mutex
	clients  map[int64]*client
	viewedAt time.Time
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Store) bound() time.Duration {
	if s.Floor > 0 {
		return min(3*s.Floor, FreshCeiling)
	}
	return FreshCeiling
}

func (s *Store) boundOf(c *client) time.Duration {
	if c.floor > 0 {
		return min(3*c.floor, FreshCeiling)
	}
	return s.bound()
}

func (s *Store) fresh(c *client, now time.Time) bool {
	return c.complete && !c.readAt.IsZero() && now.Sub(c.readAt) <= s.boundOf(c)
}

func (s *Store) get(id int64) *client {
	if s.clients == nil {
		s.clients = map[int64]*client{}
	}
	c := s.clients[id]
	if c == nil {
		c = &client{}
		s.clients[id] = c
	}
	return c
}

// Current returns a copy of one client's snapshot.
func (s *Store) Current(id int64) Snapshot {
	if s == nil {
		return Snapshot{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.clients[id]
	if c == nil {
		return Snapshot{}
	}
	fresh := s.fresh(c, s.now())
	items := make(map[string]downloader.Job, len(c.items))
	for k, v := range c.items {
		if !fresh {
			// Last-known rows: their speeds are not current.
			v.Speed, v.UpSpeed, v.ETA = 0, 0, 0
		}
		items[k] = v
	}
	return Snapshot{Gen: c.gen, Rev: c.rev, Items: items, Complete: c.complete, ReadAt: c.readAt, Fresh: fresh,
		Stale: c.stale, Running: c.running, Fallback: c.fallback, Failed: c.failed, Shared: c.shared}
}

// View is one client as downloader.Manager reads it.
func (s *Store) View(id int64) downloader.SyncView {
	if s == nil {
		return downloader.SyncView{Degraded: true}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.clients[id]
	if c == nil {
		return downloader.SyncView{Degraded: true}
	}
	now := s.now()
	fresh := s.fresh(c, now)
	v := downloader.SyncView{Scope: c.scope, Complete: c.complete, Fresh: fresh, ReadAt: c.readAt,
		Degraded: !c.running || !fresh && (c.since.IsZero() || now.Sub(c.since) >= s.boundOf(c))}
	if fresh {
		v.Items = make(map[string]downloader.Job, len(c.items))
		for k, j := range c.items {
			v.Items[k] = j
		}
	}
	return v
}

// Rev is one client's revision; it moves only when something changed.
func (s *Store) Rev(id int64) uint64 {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if c := s.clients[id]; c != nil {
		return c.rev
	}
	return 0
}

// Revs is every client's revision.
func (s *Store) Revs() map[int64]uint64 {
	out := map[int64]uint64{}
	if s == nil {
		return out
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, c := range s.clients {
		out[id] = c.rev
	}
	return out
}

// Update records a whole read of a client at engine generation gen and
// returns what changed against the previous read, even across a generation
// change (so removals during the gap are still seen).
func (s *Store) Update(id int64, gen uint64, jobs []downloader.Job, at time.Time) Changes {
	ch, _, _ := s.Apply(id, gen, "", jobs, at)
	return ch
}

// Apply is Update for a read that covered scope ("" everything), and also
// returns the items before it and whether they were stale rows loaded
// from dl_items.
func (s *Store) Apply(id int64, gen uint64, scope string, jobs []downloader.Job, at time.Time) (Changes, map[string]downloader.Job, bool) {
	cur := make(map[string]downloader.Job, len(jobs))
	for _, j := range jobs {
		cur[j.ID] = j
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.get(id)
	prev, stale := c.items, c.stale
	ch := Diff(c.items, cur)
	if !ch.Empty() || c.items == nil {
		c.rev++
	}
	c.gen, c.items, c.complete, c.readAt, c.scope = gen, cur, true, at, scope
	c.since, c.stale, c.failed = at, false, false
	return ch, prev, stale
}

// Fail records a read that did not return the whole list: the items stay
// as last known but the snapshot is no longer Complete.
func (s *Store) Fail(id int64, gen uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c := s.clients[id]; c != nil {
		c.complete, c.failed = false, true
		if c.gen != gen {
			c.gen = gen
		}
	}
}

// Reset marks a client's snapshot incomplete after a generation change,
// Forget or "Try again"; the next whole read is diffed against the old items.
func (s *Store) Reset(id int64, gen uint64) {
	s.Fail(id, gen)
}

// Load puts last-known rows (from dl_items) in place as stale: shown, but
// not Complete until the first good read replaces them.
func (s *Store) Load(id int64, jobs []downloader.Job) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.get(id)
	c.items = make(map[string]downloader.Job, len(jobs))
	for _, j := range jobs {
		c.items[j.ID] = j
	}
	c.complete, c.stale = false, true
	c.rev++
}

// Running marks whether a reader follows the client; starting counts as
// the beginning of the degraded bound.
func (s *Store) Running(id int64, on bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.get(id)
	c.running = on
	if on {
		c.since = s.now()
	}
}

// SetFloor records the client's current read cadence.
func (s *Store) SetFloor(id int64, d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.get(id).floor = d
}

// SetShared records whether another program seems to share the session.
func (s *Store) SetShared(id int64, on bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.get(id).shared = on
}

// SetFallback marks a client the Manager reads itself.
func (s *Store) SetFallback(id int64, on bool) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.get(id).fallback = on
}

// Drop forgets a client (its Downloader was removed).
func (s *Store) Drop(id int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.clients, id)
}

// Viewed records that someone has the Downloads page open.
func (s *Store) Viewed() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.viewedAt = s.now()
	s.mu.Unlock()
}

// Viewer says the Downloads page was viewed within viewerTTL.
func (s *Store) Viewer() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.viewedAt.IsZero() && s.now().Sub(s.viewedAt) < viewerTTL
}
