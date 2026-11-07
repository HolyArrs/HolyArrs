package musicscore

import (
	"sync"
	"time"
)

// Candidates caches raw candidate lists by source key, each with its own
// time to live.
type Candidates struct {
	mu sync.Mutex
	m  map[string]candEntry
}

// DayKey names one member's order for one UTC day and view.
type DayKey struct {
	Member int64
	Date   string
	View   string
}

// Days caches the row and item order per member per day. Asks and
// approvals never touch it, so the order stays steady through a day.
type Days struct {
	mu sync.Mutex
	m  map[DayKey]any
}

type candEntry struct {
	v   any
	exp time.Time
}

// Get returns the entry for key if it has not expired.
func (c *Candidates) Get(key string, now time.Time) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[key]
	if !ok || !now.Before(e.exp) {
		return nil, false
	}
	return e.v, true
}

// Put stores v under key for ttl.
func (c *Candidates) Put(key string, v any, ttl time.Duration, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// A hard cap keeps the map from growing without bound.
	if c.m == nil || len(c.m) > 20000 {
		c.m = map[string]candEntry{}
	}
	c.m[key] = candEntry{v, now.Add(ttl)}
}

// Forget drops the entries whose key starts with prefix, so a library
// change drops only the lists built from the library.
func (c *Candidates) Forget(prefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k := range c.m {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			delete(c.m, k)
		}
	}
}

// Get returns the stored order for k.
func (d *Days) Get(k DayKey) (any, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	v, ok := d.m[k]
	return v, ok
}

// Put stores the order for k and drops every other day.
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

// Invalidate drops one member's orders in every view.
func (d *Days) Invalidate(member int64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for k := range d.m {
		if k.Member == member {
			delete(d.m, k)
		}
	}
}

// Clear drops every member's orders, for changes that affect the whole
// household.
func (d *Days) Clear() {
	d.mu.Lock()
	d.m = nil
	d.mu.Unlock()
}

// Day returns the UTC calendar day of now as YYYY-MM-DD.
func Day(now time.Time) string { return now.UTC().Format("2006-01-02") }
