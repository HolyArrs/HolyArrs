package metadata

import (
	"context"
	"slices"
	"sync"
	"time"
)

// Limiter spaces requests: at most one per Every across every caller that
// shares it. Callers wait in a queue for their turn and book their moment
// only when they reach the front, so one that gives up uses nothing.
// Member lookups (MemberLookup) queue behind everyone else and are spaced
// MemberEvery apart, so the scheduled refresh and staff work always get
// most of the budget. Now and SleepUntil are the clock (real time when
// nil), so tests can run it on a fake clock.
type Limiter struct {
	Every time.Duration // default 1s
	// MemberEvery spaces member lookups; default 2 x Every (half the budget).
	MemberEvery time.Duration
	// MaxQueue bounds how many callers wait (default 64) and MaxMemberQueue
	// how many of them are member lookups (default 8); past it Wait
	// answers ErrMusicBusy at once.
	MaxQueue, MaxMemberQueue int
	Now                      func() time.Time
	SleepUntil               func(ctx context.Context, t time.Time) error

	mu         sync.Mutex
	next       time.Time // the next free moment
	memberNext time.Time // the next free moment for a member lookup
	holdUntil  time.Time // a back-off: nobody goes before it
	queue      []*ticket // waiting callers, staff and jobs before members
	sleeping   *ticket   // the caller at the front, sleeping until its moment
}

// ticket is one caller waiting in the Limiter.
type ticket struct {
	low     bool
	wake    chan struct{}
	preempt context.CancelFunc
}

type memberKey struct{}

// MemberLookup marks ctx as a member's lookup: it waits behind the
// scheduled refresh and staff, and members share part of the budget.
func MemberLookup(ctx context.Context) context.Context {
	return context.WithValue(ctx, memberKey{}, true)
}

// IsMemberLookup says whether ctx is marked as a member's lookup.
func IsMemberLookup(ctx context.Context) bool { return ctx.Value(memberKey{}) != nil }

func (l *Limiter) now() time.Time {
	if l.Now != nil {
		return l.Now()
	}
	return time.Now()
}

func (l *Limiter) every() time.Duration {
	if l.Every <= 0 {
		return time.Second
	}
	return l.Every
}

func (l *Limiter) memberEvery() time.Duration {
	if l.MemberEvery <= 0 {
		return 2 * l.every()
	}
	return l.MemberEvery
}

func (l *Limiter) sleepUntil(ctx context.Context, t time.Time) error {
	if l.SleepUntil != nil {
		return l.SleepUntil(ctx, t)
	}
	d := time.Until(t)
	if d <= 0 {
		return ctx.Err()
	}
	tm := time.NewTimer(d)
	defer tm.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-tm.C:
		return nil
	}
}

// remove takes t out of the queue and wakes the new front. l.mu is held.
func (l *Limiter) remove(t *ticket) {
	if i := slices.Index(l.queue, t); i >= 0 {
		l.queue = slices.Delete(l.queue, i, i+1)
	}
	l.wakeFront()
}

// wakeFront wakes the caller at the front of the queue. l.mu is held.
func (l *Limiter) wakeFront() {
	if len(l.queue) > 0 {
		select {
		case l.queue[0].wake <- struct{}{}:
		default:
		}
	}
}

// Wait waits for the caller's turn and sleeps until its moment: the later
// of the next free moment and any back-off, and for a member lookup the
// members' next moment. A caller whose ctx ends first gives its turn back.
func (l *Limiter) Wait(ctx context.Context) error {
	t := &ticket{low: IsMemberLookup(ctx), wake: make(chan struct{}, 1)}
	l.mu.Lock()
	maxQ, maxM := l.MaxQueue, l.MaxMemberQueue
	if maxQ <= 0 {
		maxQ = 64
	}
	if maxM <= 0 {
		maxM = 8
	}
	members := 0
	for _, q := range l.queue {
		if q.low {
			members++
		}
	}
	if len(l.queue) >= maxQ || (t.low && members >= maxM) {
		l.mu.Unlock()
		return ErrMusicBusy
	}
	if t.low {
		l.queue = append(l.queue, t)
	} else {
		i := slices.IndexFunc(l.queue, func(q *ticket) bool { return q.low })
		if i < 0 {
			i = len(l.queue)
		}
		l.queue = slices.Insert(l.queue, i, t)
		if l.sleeping != nil && l.sleeping.low && l.sleeping.preempt != nil {
			l.sleeping.preempt() // a member never keeps staff or the refresh waiting
		}
	}
	l.mu.Unlock()
	for {
		l.mu.Lock()
		if l.sleeping == nil && l.queue[0] == t {
			at := l.now()
			for _, m := range []time.Time{l.next, l.holdUntil} {
				if m.After(at) {
					at = m
				}
			}
			if t.low && l.memberNext.After(at) {
				at = l.memberNext
			}
			sctx, cancel := context.WithCancel(ctx)
			l.sleeping, t.preempt = t, cancel
			l.mu.Unlock()
			err := l.sleepUntil(sctx, at)
			cancel()
			l.mu.Lock()
			l.sleeping, t.preempt = nil, nil
			switch {
			case ctx.Err() != nil:
				l.remove(t)
				l.mu.Unlock()
				return ctx.Err()
			case err == nil && !l.holdUntil.After(at) && l.queue[0] == t:
				l.next = at.Add(l.every())
				if t.low {
					l.memberNext = at.Add(l.memberEvery())
				}
				l.remove(t)
				l.mu.Unlock()
				return nil
			}
			// A back-off began, or staff came in, while it slept: go round.
			l.wakeFront()
			l.mu.Unlock()
			continue
		}
		l.mu.Unlock()
		select {
		case <-t.wake:
		case <-ctx.Done():
			l.mu.Lock()
			l.remove(t)
			l.mu.Unlock()
			return ctx.Err()
		}
	}
}

// Hold keeps every caller waiting for at least d from now (a back-off),
// those already waiting for their moment too.
func (l *Limiter) Hold(d time.Duration) {
	l.mu.Lock()
	if t := l.now().Add(d); t.After(l.holdUntil) {
		l.holdUntil = t
	}
	l.mu.Unlock()
}
