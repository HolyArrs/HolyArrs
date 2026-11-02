package api

import (
	"context"
	"sync"
	"time"
)

// absentTTL is how long an id the slim copy lacks is answered from memory.
const absentTTL = 10 * time.Minute

// absentMax caps the remembered ids; past it the memory starts again.
const absentMax = 20000

// absentCache is a short negative cache of ids the slim copy lacks.
type absentCache struct {
	mu  sync.Mutex
	ids map[string]time.Time
}

func (c *absentCache) has(k string, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	at, ok := c.ids[k]
	if ok && now.Sub(at) >= absentTTL {
		delete(c.ids, k)
		return false
	}
	return ok
}

func (c *absentCache) add(k string, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ids == nil || len(c.ids) >= absentMax {
		c.ids = map[string]time.Time{}
	}
	c.ids[k] = now
}

// SlimAbsent reports an id the loaded slim copy does not hold, remembered
// for absentTTL.
func (s *Server) SlimAbsent(ctx context.Context, kind, mbid string) bool {
	if s.opts.SlimMissing == nil {
		return false
	}
	k, now := kind+"/"+mbid, time.Now()
	if s.absent.has(k, now) {
		return true
	}
	if !s.opts.SlimMissing(ctx, kind, mbid) {
		return false
	}
	s.absent.add(k, now)
	return true
}

// Anonymous reports a request without a credential (a shared edge route).
func Anonymous(ctx context.Context) bool { return CallerOf(ctx) == "anon" }

// CallerOf answers who made the request ("anon" without a credential, ""
// when unknown): the install-fairness name for queued work.
func CallerOf(ctx context.Context) string {
	who, _ := ctx.Value(callerKey{}).(caller)
	return who.who
}

// WithCaller marks ctx as a request made by who ("anon" for no
// credential), as the server does for its routes.
func WithCaller(ctx context.Context, who string) context.Context {
	return context.WithValue(ctx, callerKey{}, caller{registered: true, who: who})
}
