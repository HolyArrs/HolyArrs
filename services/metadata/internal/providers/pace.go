package providers

import (
	"context"
	"errors"
	"time"
)

type priorityKey struct{}

// WithPriority marks a caller (list matching) that gets at least every
// other request slot of a provider while it waits; when no priority caller
// waits, every slot goes to the others again.
func WithPriority(ctx context.Context) context.Context {
	return context.WithValue(ctx, priorityKey{}, true)
}

// Priority reports a WithPriority caller.
func Priority(ctx context.Context) bool {
	v, _ := ctx.Value(priorityKey{}).(bool)
	return v
}

// acquire takes the provider's gate. A normal caller that gets it right
// after another normal call while a priority caller waits hands it over
// first, so priority callers get at least half of the slots.
func (p *provider) acquire(ctx context.Context) error {
	prio := Priority(ctx)
	if prio {
		p.prioWait.Add(1)
		defer p.prioWait.Add(-1)
	}
	for {
		select {
		case p.gate <- struct{}{}:
		case <-ctx.Done():
			return ctx.Err()
		}
		p.mu.Lock()
		ok := prio || p.lastPrio || p.prioWait.Load() == 0
		if ok {
			p.lastPrio = prio
		}
		p.mu.Unlock()
		if ok {
			return nil
		}
		<-p.gate
		t := time.NewTimer(2 * time.Millisecond)
		select {
		case <-t.C:
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		}
	}
}

// Pace waits for the provider's next request slot, as a lookup would, so
// calls made outside this client (such as a MusicBrainz browse) share the
// same pacing. Unknown providers are not paced.
func (c *Client) Pace(ctx context.Context, provider string) error {
	p, ok := c.providers[provider]
	if !ok {
		return nil
	}
	if err := p.acquire(ctx); err != nil {
		return err
	}
	defer func() { <-p.gate }()
	if p.paused.Load() {
		return errors.New("provider paused")
	}
	return p.reserve(ctx)
}

// reserve takes the provider's next start slot (Interval after the last)
// and waits for it.
func (p *provider) reserve(ctx context.Context) error {
	p.mu.Lock()
	now := time.Now()
	slot := p.next
	if slot.Before(now) {
		slot = now
	}
	p.next = slot.Add(p.interval)
	p.mu.Unlock()
	d := time.Until(slot)
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// holdUntil keeps every start back until t (a Retry-After or a backoff).
func (p *provider) holdUntil(t time.Time) {
	p.mu.Lock()
	if t.After(p.next) {
		p.next = t
	}
	p.mu.Unlock()
}
