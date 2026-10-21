package api

import (
	"sync"
	"time"
)

// ttlMap is a small concurrent map whose entries expire after ttl and that
// never holds more than max entries: a Store sweeps expired entries once a
// minute, and when it is still full it starts over.
type ttlMap struct {
	ttl   time.Duration
	max   int
	mu    sync.Mutex
	m     map[any]ttlEntry
	swept time.Time
}

type ttlEntry struct {
	v  any
	at time.Time
}

func newTTLMap(ttl time.Duration, max int) *ttlMap {
	return &ttlMap{ttl: ttl, max: max, m: map[any]ttlEntry{}}
}

func (t *ttlMap) Load(k any) (any, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	e, ok := t.m[k]
	if !ok || time.Since(e.at) >= t.ttl {
		return nil, false
	}
	return e.v, true
}

func (t *ttlMap) Store(k, v any) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	if now.Sub(t.swept) >= time.Minute || len(t.m) >= t.max {
		t.swept = now
		for key, e := range t.m {
			if now.Sub(e.at) >= t.ttl {
				delete(t.m, key)
			}
		}
		if len(t.m) >= t.max {
			t.m = map[any]ttlEntry{}
		}
	}
	t.m[k] = ttlEntry{v: v, at: now}
}

func (t *ttlMap) Len() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.m)
}
