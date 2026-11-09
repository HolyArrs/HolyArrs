package downloads

import (
	"sync"
	"time"
)

// FreshCeiling is the longest a read counts as fresh, whatever the cadence.
const FreshCeiling = 2 * time.Minute

// viewerTTL is how long one view of the Downloads page counts as a viewer.
const viewerTTL = 60 * time.Second

// Snapshot is one client as last read.
//
// Complete: the last read returned the whole list without error. Fresh:
// Complete and ReadAt within min(3 x the client's cadence, FreshCeiling) of
// now. Stale: the items were loaded from dl_items after a start and no read
// has replaced them yet. Running: a reader is following the client.
// Fallback: the Manager reads the client itself. Failed: the last read
// failed. Shared: another program seems to share the session.
type Snapshot struct {
	Gen      uint64
	Rev      uint64
	Items    map[string]Job
	Complete bool
	ReadAt   time.Time
	Fresh    bool
	Stale    bool
	Running  bool
	Fallback bool
	Failed   bool
	Shared   bool
}

// Store holds the snapshots. The zero value is ready, and a nil *Store
// answers every Current with Complete=false, so callers need no checks.
type Store struct {
	// Floor is the reader's floor cadence. Zero uses FreshCeiling alone, and a
	// client's own cadence, when set, takes its place.
	Floor time.Duration
	// Now replaces the clock in tests.
	Now func() time.Time

	mu       sync.Mutex
	clients  map[int64]*client
	viewedAt time.Time
}

// client is the stored state of one Downloader.
type client struct {
	gen      uint64
	rev      uint64
	items    map[string]Job
	complete bool
	readAt   time.Time
	scope    string
	floor    time.Duration // this client's read cadence
	running  bool
	since    time.Time // reader start or last good read; the degraded bound counts from here
	fallback bool
	stale    bool
	failed   bool
	shared   bool
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
	items := make(map[string]Job, len(c.items))
	for k, v := range c.items {
		if !fresh {
			// These are last-known rows, so their speeds would be wrong.
			v.Speed, v.UpSpeed, v.ETA = 0, 0, 0
		}
		items[k] = v
	}
	return Snapshot{Gen: c.gen, Rev: c.rev, Items: items, Complete: c.complete, ReadAt: c.readAt, Fresh: fresh,
		Stale: c.stale, Running: c.running, Fallback: c.fallback, Failed: c.failed, Shared: c.shared}
}

// View returns one client as Manager reads it. It is degraded
// when no reader runs or the last good read is older than the bound.
func (s *Store) View(id int64) SyncView {
	if s == nil {
		return SyncView{Degraded: true}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.clients[id]
	if c == nil {
		return SyncView{Degraded: true}
	}
	now := s.now()
	fresh := s.fresh(c, now)
	v := SyncView{Scope: c.scope, Complete: c.complete, Fresh: fresh, ReadAt: c.readAt,
		Degraded: !c.running || !fresh && (c.since.IsZero() || now.Sub(c.since) >= s.boundOf(c))}
	if fresh {
		v.Items = make(map[string]Job, len(c.items))
		for k, j := range c.items {
			v.Items[k] = j
		}
	}
	return v
}

// Rev returns one client's revision, which moves only when something changed.
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

// Revs returns every client's revision.
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
// returns what changed against the previous read. It diffs across a
// generation change too, so removals during the gap are still seen.
func (s *Store) Update(id int64, gen uint64, jobs []Job, at time.Time) Changes {
	ch, _, _ := s.Apply(id, gen, "", jobs, at)
	return ch
}

// Apply is Update for a read that covered scope ("" is everything). It also
// returns the items before the read and whether they were stale rows loaded
// from dl_items.
func (s *Store) Apply(id int64, gen uint64, scope string, jobs []Job, at time.Time) (Changes, map[string]Job, bool) {
	cur := make(map[string]Job, len(jobs))
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

// Fail records a read that did not return the whole list. The items stay as
// last known, but the snapshot is no longer Complete.
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

// Reset marks a client's snapshot incomplete after a generation change, a
// forget or a retry. The next whole read is diffed against the old items.
func (s *Store) Reset(id int64, gen uint64) {
	s.Fail(id, gen)
}

// Load puts last-known rows from dl_items in place as stale. They are shown
// but not Complete until the first good read replaces them.
func (s *Store) Load(id int64, jobs []Job) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.get(id)
	c.items = make(map[string]Job, len(jobs))
	for _, j := range jobs {
		c.items[j.ID] = j
	}
	c.complete, c.stale = false, true
	c.rev++
}

// Running marks whether a reader follows the client. Starting also starts
// the degraded bound.
func (s *Store) Running(id int64, on bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.get(id)
	c.running = on
	if on {
		c.since = s.now()
	}
}

// SetFloor records the client's current read cadence, which sets how long a
// read counts as fresh.
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

// SetFallback marks whether the Manager reads the client itself.
func (s *Store) SetFallback(id int64, on bool) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.get(id).fallback = on
}

// Drop forgets a client whose Downloader was removed.
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

// Viewer reports whether the Downloads page was viewed within viewerTTL.
func (s *Store) Viewer() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.viewedAt.IsZero() && s.now().Sub(s.viewedAt) < viewerTTL
}

// now returns the clock, which tests can replace.
func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// bound is how long a read counts as fresh with no cadence of its own.
func (s *Store) bound() time.Duration {
	if s.Floor > 0 {
		return min(3*s.Floor, FreshCeiling)
	}
	return FreshCeiling
}

// boundOf is how long a read of client c counts as fresh: three of its
// cadences, never more than FreshCeiling.
func (s *Store) boundOf(c *client) time.Duration {
	if c.floor > 0 {
		return min(3*c.floor, FreshCeiling)
	}
	return s.bound()
}

// fresh reports whether c has a complete read inside its bound.
func (s *Store) fresh(c *client, now time.Time) bool {
	return c.complete && !c.readAt.IsZero() && now.Sub(c.readAt) <= s.boundOf(c)
}

// get returns the client for id, making it if needed. The caller holds mu.
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
