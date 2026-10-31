// Package dlsync keeps one in-memory snapshot per download client and works
// out what changed between reads. In shadow mode (setting downloads.sync =
// shadow) it is fed only from lists the Manager already reads, so it adds
// no call to any client.
package dlsync

import (
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs/internal/downloader"
)

// FreshCeiling is the longest a read counts as fresh, whatever the cadence.
const FreshCeiling = 2 * time.Minute

// Snapshot is one client as last read.
//
// Complete: the last read returned the whole list without error. Fresh:
// Complete and ReadAt within min(3 x Floor, FreshCeiling) of now.
type Snapshot struct {
	Gen      uint64
	Rev      uint64
	Items    map[string]downloader.Job
	Complete bool
	ReadAt   time.Time
	Fresh    bool
}

type client struct {
	gen      uint64
	rev      uint64
	items    map[string]downloader.Job
	complete bool
	readAt   time.Time
}

// Store holds the snapshots. The zero value is ready; a nil *Store answers
// every Current with Complete=false, so callers need no checks.
type Store struct {
	// Floor is the reader's floor cadence; zero uses FreshCeiling alone.
	Floor time.Duration
	// Now is for tests.
	Now func() time.Time

	mu      sync.Mutex
	clients map[int64]*client
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
	items := make(map[string]downloader.Job, len(c.items))
	for k, v := range c.items {
		items[k] = v
	}
	fresh := c.complete && !c.readAt.IsZero() && s.now().Sub(c.readAt) <= s.bound()
	return Snapshot{Gen: c.gen, Rev: c.rev, Items: items, Complete: c.complete, ReadAt: c.readAt, Fresh: fresh}
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

// Update records a whole read of a client at engine generation gen and
// returns what changed against the previous read, even across a generation
// change (so removals during the gap are still seen).
func (s *Store) Update(id int64, gen uint64, jobs []downloader.Job, at time.Time) Changes {
	cur := make(map[string]downloader.Job, len(jobs))
	for _, j := range jobs {
		cur[j.ID] = j
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.clients == nil {
		s.clients = map[int64]*client{}
	}
	c := s.clients[id]
	if c == nil {
		c = &client{}
		s.clients[id] = c
	}
	ch := Diff(c.items, cur)
	if !ch.Empty() || c.items == nil {
		c.rev++
	}
	c.gen, c.items, c.complete, c.readAt = gen, cur, true, at
	return ch
}

// Fail records a read that did not return the whole list: the items stay
// as last known but the snapshot is no longer Complete.
func (s *Store) Fail(id int64, gen uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c := s.clients[id]; c != nil {
		c.complete = false
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
