package metadata

import "time"

// Forget drops every entry kept in memory (lookup_cache keeps its rows).
func (t *Trailers) Forget() {
	t.mu.Lock()
	t.cache = nil
	t.mu.Unlock()
}

// Drop drops one entry kept in memory.
func (t *Trailers) Drop(ckey string) {
	t.mu.Lock()
	delete(t.cache, ckey)
	t.mu.Unlock()
}

// ResetDoor forgets the lookups that failed in a row at YouTube's door.
func (t *Trailers) ResetDoor() { t.failed.Store(0) }

// SetHealthTimes sets when the last good lookup was and when the health
// state was last written; a zero time leaves that one as it is.
func (t *Trailers) SetHealthTimes(lastGood, saved time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !lastGood.IsZero() {
		t.lastGood = lastGood
	}
	if !saved.IsZero() {
		t.saved = saved
	}
}

// RelayAddresses is how many addresses the relay keeps for stream id.
func (t *Trailers) RelayAddresses(id string) int {
	t.relay.mu.Lock()
	defer t.relay.mu.Unlock()
	if s := t.relay.streams[id]; s != nil {
		return len(s.index)
	}
	return 0
}

// NotedTitles is how many titles are noted for the next pass, kept or not.
func (t *Trailers) NotedTitles() int {
	t.asked.mu.Lock()
	defer t.asked.mu.Unlock()
	return len(t.asked.m)
}
