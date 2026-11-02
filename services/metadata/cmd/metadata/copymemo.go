package main

import (
	"context"
	"sync"
)

// copyMemo keeps answers read from the slim copy, one per key, for the
// copy they were read from (version is the copy in use, "" when none):
// a new copy starts it over, and an answer read with no copy, or by a read
// cut short, is not kept. At most limit keys, then it starts over.
type copyMemo[V any] struct {
	version func() string
	limit   int

	mu   sync.Mutex
	memo map[string]V
	ver  string
}

func newCopyMemo[V any](version func() string, limit int) *copyMemo[V] {
	return &copyMemo[V]{version: version, limit: limit, memo: map[string]V{}}
}

// get answers key's kept answer, else read's (kept as above).
func (m *copyMemo[V]) get(ctx context.Context, key string, read func() V) V {
	ver := m.version()
	m.mu.Lock()
	if ver != m.ver {
		m.memo, m.ver = map[string]V{}, ver
	}
	v, ok := m.memo[key]
	m.mu.Unlock()
	if ok {
		return v
	}
	v = read()
	if ver == "" || ctx.Err() != nil {
		return v
	}
	m.mu.Lock()
	if len(m.memo) >= m.limit {
		m.memo = map[string]V{}
	}
	if ver == m.ver {
		m.memo[key] = v
	}
	m.mu.Unlock()
	return v
}
