package suggest

import (
	"context"
	"sync"
	"time"
)

// Paced is a Source that lets one call through per Gap, shared by every pool
// build on the server. It is the pool job's own limit, apart from the
// per-person Explore limit.
type Paced struct {
	Source Source
	// Gap is the time between calls; zero or less means 2 seconds.
	Gap time.Duration
	// Burst is how many calls may go at once after a quiet spell, at least 1.
	// The long-run rate stays one per Gap.
	Burst int
	mu    sync.Mutex
	next  time.Time
	// Calls counts the calls let through.
	Calls int
}

// List waits for its turn, then asks the Source.
func (p *Paced) List(ctx context.Context, q Request) ([]Item, error) {
	gap := p.Gap
	if gap <= 0 {
		gap = 2 * time.Second
	}
	// The slot is reserved under the lock so concurrent callers queue in order;
	// the wait itself happens outside it.
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
