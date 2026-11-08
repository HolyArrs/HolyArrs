package discover

import (
	"sync"
	"time"
)

// TTLMap is a small concurrent map whose entries expire after ttl and that
// never holds more than max entries: a Store sweeps expired entries once a
// minute, and when it is still full it starts over.
type TTLMap struct {
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

// NewTTLMap makes a TTLMap keeping entries ttl and at most max of them.
func NewTTLMap(ttl time.Duration, max int) *TTLMap {
	return &TTLMap{ttl: ttl, max: max, m: map[any]ttlEntry{}}
}

func (t *TTLMap) Load(k any) (any, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	e, ok := t.m[k]
	if !ok || time.Since(e.at) >= t.ttl {
		return nil, false
	}
	return e.v, true
}

func (t *TTLMap) Store(k, v any) {
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

func (t *TTLMap) Len() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.m)
}
