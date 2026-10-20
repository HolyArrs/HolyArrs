package explore

import (
	"context"
	"sync"
	"time"
)

// Paced is a Source that lets one call through every Gap, shared by every
// pool build on the server. It is the pool job's own limiter, apart from
// the per-person Explore limit.
type Paced struct {
	Source Source
	Gap    time.Duration // default 2 s
	// Burst is how many calls may go at once after a quiet spell (default
	// 1); the long-run rate stays one per Gap.
	Burst int
	mu     sync.Mutex
	next   time.Time
	// Calls counts the calls let through.
	Calls int
}

// List waits for its turn, then asks the Source.
func (p *Paced) List(ctx context.Context, q Request) ([]Item, error) {
	gap := p.Gap
	if gap <= 0 {
		gap = 2 * time.Second
	}
	p.mu.Lock()
	now := time.Now()
	burst := max(p.Burst, 1)
	at := p.next
	if floor := now.Add(-time.Duration(burst-1) * gap); at.Before(floor) {
		at = floor
	}
	p.next = at.Add(gap)
	p.Calls++
	p.mu.Unlock()
	if wait := time.Until(at); wait > 0 {
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return nil, ctx.Err()
		case <-t.C:
		}
	}
	return p.Source.List(ctx, q)
}
