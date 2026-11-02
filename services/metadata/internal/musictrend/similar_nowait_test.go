package musictrend

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// A page open never waits on the outside source: a miss answers
// ErrPending at once and fetches in the background; a stale answer is
// served while it is fetched again.
func TestSimilarNowNeverWaits(t *testing.T) {
	release := make(chan struct{})
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		_, _ = w.Write([]byte(`[{"artist_mbid":"` + artistB + `","name":"B","score":9}]`))
	}))
	defer src.Close()
	defer close(release)
	now := time.Unix(1_800_000_000, 0)
	s := &Similar{URL: src.URL, HTTP: src.Client(), Now: func() time.Time { return now }}
	ctx := context.Background()
	t0 := time.Now()
	if _, _, err := s.ArtistsNow(ctx, artistA); !errors.Is(err, ErrPending) {
		t.Fatalf("miss: %v", err)
	}
	if d := time.Since(t0); d > 200*time.Millisecond {
		t.Fatalf("miss waited %v", d)
	}
	release <- struct{}{}
	deadline := time.Now().Add(3 * time.Second)
	for {
		b, cache, err := s.ArtistsNow(ctx, artistA)
		if err == nil && cache == "hit" && len(b) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("never filled: %s %v", cache, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	now = now.Add(8 * 24 * time.Hour)
	t0 = time.Now()
	b, cache, err := s.ArtistsNow(ctx, artistA)
	if err != nil || cache != "stale" || len(b) == 0 || time.Since(t0) > 200*time.Millisecond {
		t.Fatalf("stale: %s %v %v", cache, err, time.Since(t0))
	}
}

// L4: many asks at once start one background fetch, and a miss asked
// while every slot is busy is still fetched once a slot frees.
func TestSimilarNowOneFlightAndWaitsForSlot(t *testing.T) {
	var hits atomic.Int32
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(`[{"artist_mbid":"` + artistB + `","name":"B","score":9}]`))
	}))
	defer src.Close()
	s := &Similar{URL: src.URL, HTTP: src.Client()}
	ctx := context.Background()
	// Every slot busy.
	for i := 0; i < cap(similarBackground); i++ {
		similarBackground <- struct{}{}
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, _ = s.ArtistsNow(ctx, artistA)
		}()
	}
	wg.Wait()
	time.Sleep(50 * time.Millisecond)
	if hits.Load() != 0 {
		t.Fatal("fetched without a slot")
	}
	for i := 0; i < cap(similarBackground); i++ {
		<-similarBackground
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, cache, err := s.ArtistsNow(ctx, artistA); err == nil && cache == "hit" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("miss never started after a slot freed")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("%d fetches for one artist", n)
	}
}

// L3: a full cache drops the least recently set entry.
func TestSimilarCacheDropsLeastRecent(t *testing.T) {
	s := &Similar{Max: 3}
	s.mu.Lock()
	for _, k := range []string{"a", "b", "c"} {
		s.setEntryLocked(k, similarEntry{body: []byte(k)})
	}
	s.setEntryLocked("a", similarEntry{body: []byte("a2")}) // a is the newest now
	s.setEntryLocked("d", similarEntry{body: []byte("d")})
	_, hasB := s.entries["b"]
	_, hasA := s.entries["a"]
	n, l := len(s.entries), s.lru.Len()
	s.mu.Unlock()
	if hasB || !hasA || n != 3 || l != 3 {
		t.Fatalf("b kept %v, a kept %v, %d entries, %d in order", hasB, hasA, n, l)
	}
}
