package main

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"

	engine "github.com/HolyArrs/holyarrs/app/backend/internal/suggest"
)

func (f *recordingTMDB) count(part string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, u := range f.urls {
		if strings.Contains(u, part) {
			n++
		}
	}
	return n
}

// Concurrent builds asking for one list share one outside call, and the
// list is kept in SQLite so a restarted server does not ask again.
func TestExploreSourceSharesAndStoresLists(t *testing.T) {
	t.Parallel()
	a, s, f := exploreRig(t)
	a.explore.Paced = &engine.Paced{Source: discover.ListerSource{L: f.provider()}, Gap: 30 * time.Millisecond}
	q := engine.Request{List: "trending", Kind: "movie", Scope: engine.ScopeShared}
	var wg sync.WaitGroup
	calls := &engine.Calls{}
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			items, err := discover.ExploreSource{E: &a.explore, Lister: f.provider(), Calls: calls}.List(context.Background(), q)
			if err != nil || len(items) == 0 {
				t.Errorf("%d items, %v", len(items), err)
			}
		}()
	}
	wg.Wait()
	if n := f.count("/trending/movie/week"); n != 1 {
		t.Fatalf("6 concurrent asks made %d outside calls, want 1", n)
	}
	if calls.Shared != 1 || calls.Member != 0 {
		t.Fatalf("counted %s", calls)
	}
	b := buildHandlers(services{Store: s, Metadata: f.provider(), Version: "test"})
	src := discover.ExploreSource{E: &b.explore, Lister: f.provider()}
	if !src.Cached(context.Background(), q) {
		t.Fatal("a new server did not find the stored list")
	}
	if _, err := src.List(context.Background(), q); err != nil || f.count("/trending/movie/week") != 1 {
		t.Fatalf("the stored list went out again (%v)", err)
	}
}

// The pool job builds up to discover.ExploreWorkers pools at once, all through the
// one paced limiter, and records each build's outside calls.
func TestExploreBuildsInParallel(t *testing.T) {
	t.Parallel()
	a, _, f := exploreRig(t)
	a.explore.Paced = &engine.Paced{Source: discover.ListerSource{L: f.provider()}, Gap: 5 * time.Millisecond}
	now := time.Now()
	for _, m := range []int64{1, 2, 3} {
		a.explore.QueuePool(discover.PoolKey{Member: m, Switch: "all"}, now, false)
	}
	a.explore.QueuePool(discover.PoolKey{Member: 1, Switch: "movies"}, now, false)
	ctx := context.Background()
	a.explore.RunDue(ctx)
	_, _, running := a.explore.BuildCounts()
	if running != discover.ExploreWorkers {
		t.Fatalf("%d builds running, want %d", running, discover.ExploreWorkers)
	}
	deadline := time.Now().Add(20 * time.Second)
	for {
		a.explore.RunDue(ctx)
		done, queued, busy := a.explore.BuildCounts()
		left := queued + busy
		if done == 4 && left == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d of 4 builds after 20 s", done)
		}
		time.Sleep(10 * time.Millisecond)
	}
	calls := a.explore.LastCalls()
	for k, c := range calls {
		if c.Member > engine.MemberBudget {
			t.Errorf("pool %v: %s over the budget", k, c)
		}
	}
	if len(calls) != 4 {
		t.Fatalf("calls kept for %d pools", len(calls))
	}
	if a.explore.Paced.Calls != f.count("") {
		t.Logf("paced %d, tmdb %d (genre and person lookups add a few)", a.explore.Paced.Calls, f.count(""))
	}
}

// A looked-up rating is kept in SQLite: a new server knows it.
func TestKidRatingStored(t *testing.T) {
	t.Parallel()
	a, s, f := exploreRig(t)
	ref := ratingRef{Kind: "movie", TMDBID: "1234"}
	r, ok := a.lookupRating(context.Background(), ref)
	if !ok {
		t.Fatal("lookup failed")
	}
	b := buildHandlers(services{Store: s, Metadata: f.provider(), Version: "test"})
	got, ok := b.knownRating(context.Background(), ref)
	if !ok || got != r {
		t.Fatalf("new server: %q %v, want %q", got, ok, r)
	}
}
