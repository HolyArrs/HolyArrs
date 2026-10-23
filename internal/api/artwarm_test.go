package api

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func warmID(n int) string { return fmt.Sprintf("00000000-0000-4000-8000-%012d", n) }

func TestArtWarmDedupesAndCapsConcurrency(t *testing.T) {
	old := artWarmGap
	artWarmGap = time.Millisecond
	defer func() { artWarmGap = old }()
	a := &API{}
	var mu sync.Mutex
	seen := map[artWarmKey]int{}
	var cur, peak atomic.Int32
	a.artWarm.do = func(_ context.Context, k artWarmKey) {
		n := cur.Add(1)
		for p := peak.Load(); n > p && !peak.CompareAndSwap(p, n); p = peak.Load() {
		}
		time.Sleep(5 * time.Millisecond)
		cur.Add(-1)
		mu.Lock()
		seen[k]++
		mu.Unlock()
	}
	var list []nameKey
	for i := 0; i < 30; i++ {
		list = append(list, nameKey{kind: "artist", mbid: warmID(i)}, nameKey{kind: "artist", mbid: warmID(i)}, nameKey{kind: "song", mbid: warmID(i)}, nameKey{kind: "album", mbid: "nope"})
	}
	a.warmArtItems(list)
	a.rememberMusicNames(list[:10]) // listed again while queued or done
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		a.artWarm.mu.Lock()
		idle := a.artWarm.workers == 0 && len(a.artWarm.pending) == 0
		a.artWarm.mu.Unlock()
		if idle {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 30 {
		t.Fatalf("warmed %d items, want 30", len(seen))
	}
	for k, n := range seen {
		if n != 1 {
			t.Fatalf("%v warmed %d times", k, n)
		}
	}
	if p := peak.Load(); p > artWarmWorkers || p < 2 {
		t.Fatalf("peak concurrency %d", p)
	}
	// A fresh item is not queued again.
	a.warmArtItems(list[:2])
	if len(a.artWarm.pending) != 0 || a.artWarm.workers != 0 {
		t.Fatal("fresh item queued again")
	}
}
