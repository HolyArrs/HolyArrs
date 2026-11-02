package providers

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// While list matching waits it gets at least half of the slots; with no
// priority caller waiting, the others get every slot again.
func TestPriorityGetsHalfTheSlotsWhileWaiting(t *testing.T) {
	c, err := New(Options{Providers: map[string]Config{"musicbrainz": {Interval: 10 * time.Millisecond}}, Timeout: time.Second, Attempts: 1, UserAgent: "test"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()
	var normal, prio atomic.Int64
	var wg sync.WaitGroup
	// Four busy normal callers (the shared fill) and one priority caller.
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for c.Pace(ctx, "musicbrainz") == nil {
				normal.Add(1)
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		pctx := WithPriority(ctx)
		for c.Pace(pctx, "musicbrainz") == nil {
			prio.Add(1)
		}
	}()
	wg.Wait()
	if p, n := prio.Load(), normal.Load(); p < n-2 || p+n < 20 {
		t.Fatalf("priority %d, normal %d: want at least half", p, n)
	}
	// No priority caller: a normal caller is never held back.
	c2, _ := New(Options{Providers: map[string]Config{"musicbrainz": {Interval: 10 * time.Millisecond}}, Timeout: time.Second, Attempts: 1, UserAgent: "test"})
	ctx2, cancel2 := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel2()
	n := 0
	for c2.Pace(ctx2, "musicbrainz") == nil {
		n++
	}
	if n < 20 {
		t.Fatalf("normal alone got %d slots", n)
	}
}
