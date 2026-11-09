package discover

import (
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

const (
	// PerMinute is each person's Explore asks a minute: high enough that
	// flipping genre pages never hits it, low enough to stop a runaway client.
	PerMinute = 240
	maxCache  = 500
)

// Entry is one cached Explore row or details page.
type Entry struct {
	At       time.Time
	Items    []metadata.DiscoverItem
	Details  *metadata.Details
	Services []metadata.WatchService
	Episodes []metadata.DetailEpisode
	Reviews  *metadata.ReviewPage
}

// Cache holds the row cache and each person's request times. Zero value ready.
type Cache struct {
	mu    sync.Mutex
	cache map[string]Entry
	hits  map[int64][]time.Time
}

// Allow counts one request for a person; false when over the minute limit.
func (d *Cache) Allow(who int64, now time.Time) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.hits == nil {
		d.hits = map[int64][]time.Time{}
	}
	kept := d.hits[who][:0]
	for _, t := range d.hits[who] {
		if now.Sub(t) < time.Minute {
			kept = append(kept, t)
		}
	}
	if len(kept) >= PerMinute {
		d.hits[who] = kept
		return false
	}
	d.hits[who] = append(kept, now)
	return true
}

// Get returns the cached entry for key.
func (d *Cache) Get(key string) (Entry, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	e, ok := d.cache[key]
	return e, ok
}

// Put stores e under key; a full cache starts over.
func (d *Cache) Put(key string, e Entry) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.cache == nil || len(d.cache) >= maxCache {
		d.cache = map[string]Entry{}
	}
	d.cache[key] = e
}

// Len is the number of cached entries.
func (d *Cache) Len() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.cache)
}

// HitCount is the number of people with counted requests.
func (d *Cache) HitCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.hits)
}

// ResetHits forgets every person's request times.
func (d *Cache) ResetHits() {
	d.mu.Lock()
	d.hits = nil
	d.mu.Unlock()
}
