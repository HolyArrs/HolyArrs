package musicscore

import (
	"sync"
	"time"
)

// Candidates is layer 1: raw candidate lists by source key, each with its
// own time to live (charts 6 h, library lists 5 min).
type Candidates struct {
	mu sync.Mutex
	m  map[string]candEntry
}

type candEntry struct {
	v   any
	exp time.Time
}

// Get answers a live entry.
func (c *Candidates) Get(key string, now time.Time) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[key]
	if !ok || !now.Before(e.exp) {
		return nil, false
	}
	return e.v, true
}

// Put keeps v for ttl.
func (c *Candidates) Put(key string, v any, ttl time.Duration, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil || len(c.m) > 20000 {
		c.m = map[string]candEntry{}
	}
	c.m[key] = candEntry{v, now.Add(ttl)}
}

// Forget drops the entries whose key starts with prefix (a library change
// drops only library-derived lists).
func (c *Candidates) Forget(prefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k := range c.m {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			delete(c.m, k)
		}
	}
}

// DayKey is layer 2's key: one member, one calendar day (UTC), plus the
// view (genre filter and region).
type DayKey struct {
	Member int64
	Date   string
	View   string
}

// Days is layer 2: the row and item order per member per day. It is
// dropped on a date change, by Invalidate (a follow, a cold-start unlock)
// or by Clear (a music title added or removed); asks and approvals never
// touch it.
type Days struct {
	mu sync.Mutex
	m  map[DayKey]any
}

// Get answers the member's order for the day.
func (d *Days) Get(k DayKey) (any, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	v, ok := d.m[k]
	return v, ok
}

// Put keeps the order, dropping other days.
func (d *Days) Put(k DayKey, v any) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.m == nil {
		d.m = map[DayKey]any{}
	}
	for old := range d.m {
		if old.Date != k.Date {
			delete(d.m, old)
		}
	}
	d.m[k] = v
}

// Invalidate drops one member's orders (every view).
func (d *Days) Invalidate(member int64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for k := range d.m {
		if k.Member == member {
			delete(d.m, k)
		}
	}
}

// Clear drops every member's orders (a Library change is the household's).
func (d *Days) Clear() {
	d.mu.Lock()
	d.m = nil
	d.mu.Unlock()
}

// Day is the calendar day (UTC) of now.
func Day(now time.Time) string { return now.UTC().Format("2006-01-02") }
