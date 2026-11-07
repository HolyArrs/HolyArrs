package api

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs/internal/explore"
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
	a, s, f := exploreRig(t)
	a.explore.paced = &explore.Paced{Source: listerSource{f.provider()}, Gap: 30 * time.Millisecond}
	q := explore.Request{List: "trending", Kind: "movie", Scope: explore.ScopeShared}
	var wg sync.WaitGroup
	calls := &explore.Calls{}
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			items, err := exploreSource{a: a, lister: f.provider(), calls: calls}.List(context.Background(), q)
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
	b := New(Server{Store: s, Metadata: f.provider(), Version: "test"})
	src := exploreSource{a: b, lister: f.provider()}
	if !src.Cached(context.Background(), q) {
		t.Fatal("a new server did not find the stored list")
	}
	if _, err := src.List(context.Background(), q); err != nil || f.count("/trending/movie/week") != 1 {
		t.Fatalf("the stored list went out again (%v)", err)
	}
}

// The pool job builds up to exploreWorkers pools at once, all through the
// one paced limiter, and records each build's outside calls.
func TestExploreBuildsInParallel(t *testing.T) {
	a, _, f := exploreRig(t)
	a.explore.paced = &explore.Paced{Source: listerSource{f.provider()}, Gap: 5 * time.Millisecond}
	now := time.Now()
	for _, m := range []int64{1, 2, 3} {
		a.queuePool(poolKey{m, "all"}, now, false)
	}
	a.queuePool(poolKey{1, "movies"}, now, false)
	ctx := context.Background()
	a.runDue(ctx)
	a.explore.mu.Lock()
	running := len(a.explore.building)
	a.explore.mu.Unlock()
	if running != exploreWorkers {
		t.Fatalf("%d builds running, want %d", running, exploreWorkers)
	}
	deadline := time.Now().Add(20 * time.Second)
	for {
		a.runDue(ctx)
		a.explore.mu.Lock()
		done, left := a.explore.builds, len(a.explore.due)+len(a.explore.building)
		a.explore.mu.Unlock()
		if done == 4 && left == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d of 4 builds after 20 s", done)
		}
		time.Sleep(10 * time.Millisecond)
	}
	a.explore.mu.Lock()
	defer a.explore.mu.Unlock()
	for k, c := range a.explore.calls {
		if c.Member > explore.MemberBudget {
			t.Errorf("pool %v: %s over the budget", k, c)
		}
	}
	if len(a.explore.calls) != 4 {
		t.Fatalf("calls kept for %d pools", len(a.explore.calls))
	}
	if a.explore.paced.Calls != f.count("") {
		t.Logf("paced %d, tmdb %d (genre and person lookups add a few)", a.explore.paced.Calls, f.count(""))
	}
}

// A looked-up rating is kept in SQLite: a new server knows it.
func TestKidRatingStored(t *testing.T) {
	a, s, f := exploreRig(t)
	ref := ratingRef{kind: "movie", tmdbID: "1234"}
	r, ok := a.lookupRating(context.Background(), ref)
	if !ok {
		t.Fatal("lookup failed")
	}
	b := New(Server{Store: s, Metadata: f.provider(), Version: "test"})
	got, ok := b.knownRating(context.Background(), ref)
	if !ok || got != r {
		t.Fatalf("new server: %q %v, want %q", got, ok, r)
	}
}
