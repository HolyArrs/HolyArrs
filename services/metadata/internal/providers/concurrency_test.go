package providers

import (
	"context"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// With Concurrency 6 up to six calls run at once, and their starts still
// keep the interval apart.
func TestConcurrentCallsKeepTheirSpacing(t *testing.T) {
	p := &provider{interval: 20 * time.Millisecond, gate: make(chan struct{}, 6)}
	var running, most atomic.Int32
	var mu sync.Mutex
	var starts []time.Time
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := context.Background()
			if p.acquire(ctx) != nil || p.reserve(ctx) != nil {
				t.Error("acquire")
				return
			}
			mu.Lock()
			starts = append(starts, time.Now())
			mu.Unlock()
			n := running.Add(1)
			for {
				m := most.Load()
				if n <= m || most.CompareAndSwap(m, n) {
					break
				}
			}
			time.Sleep(150 * time.Millisecond)
			running.Add(-1)
			<-p.gate
		}()
	}
	wg.Wait()
	if most.Load() != 6 {
		t.Fatalf("at most %d at once, want 6", most.Load())
	}
	sort.Slice(starts, func(a, b int) bool { return starts[a].Before(starts[b]) })
	for i := 1; i < len(starts); i++ {
		if d := starts[i].Sub(starts[i-1]); d < 18*time.Millisecond {
			t.Fatalf("starts %d and %d only %v apart", i-1, i, d)
		}
	}
}
