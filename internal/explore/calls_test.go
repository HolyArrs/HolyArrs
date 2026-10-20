package explore

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// countingSource counts each distinct Request once, the way the server's
// list cache makes a repeat a hit.
type countingSource struct {
	src   Source
	mu    sync.Mutex
	seen  map[string]bool
	calls *Calls
}

func (c *countingSource) List(ctx context.Context, q Request) ([]Item, error) {
	c.mu.Lock()
	if c.seen == nil {
		c.seen = map[string]bool{}
	}
	if !c.seen[q.String()] {
		c.seen[q.String()] = true
		c.calls.Add(q)
	}
	c.mu.Unlock()
	return c.src.List(ctx, q)
}

func TestCallsSplitByTypeAndScope(t *testing.T) {
	c := &Calls{}
	c.Add(Request{List: "trending", Kind: "movie", Scope: ScopeShared})
	c.Add(Request{List: "similar", Kind: "movie", Ref: "1"})
	c.Add(Request{List: "person", Ref: "Ann", Scope: ScopeShared})
	c.Add(Request{List: "discover", Genre: "Drama"})
	if c.Shared != 3 || c.Member != 3 {
		t.Fatalf("shared %d member %d, want 3 and 3", c.Shared, c.Member)
	}
	if c.ByType["recommendations"] != 1 || c.ByType["similar"] != 1 || c.ByType["person"] != 2 || c.ByType["discover"] != 1 || c.ByType["trending"] != 1 {
		t.Fatalf("by type %v", c.ByType)
	}
	if got := c.String(); got != "member=3 shared=3 (discover=1 person=2 recommendations=1 similar=1 trending=1)" {
		t.Fatal(got)
	}
	var none *Calls
	none.Add(Request{List: "trending"}) // a nil counter is a no-op
}

// TestMemberCallBudget is plan 5.2's arithmetic: at most 30 member-specific
// outside calls per build, region lists counted apart, for 0, 5 and 12
// seeds on every switch.
func TestMemberCallBudget(t *testing.T) {
	people := map[string]int{"Ann": 9, "Bo": 7, "Cy": 5, "Di": 4}
	dg := map[string]int{"1990s|Drama": 9, "2000s|Comedy": 8, "1980s|Action": 7}
	for _, n := range []int{0, 5, 12} {
		for _, sw := range []string{"all", "movies", "shows"} {
			c := &Calls{}
			in := Input{Member: 1, Switch: sw, Now: day0, Seeds: seedsN(n, 1), OwnedPeople: people, OwnedDecadeGenre: dg, Region: "US", Services: "8|9"}
			if _, err := Build(context.Background(), &countingSource{src: &fakeSource{}, calls: c}, in); err != nil {
				t.Fatal(err)
			}
			if c.Member > 30 {
				t.Errorf("%d seeds, %s: %d member calls, budget 30 (%s)", n, sw, c.Member, c)
			}
			if c.Shared > 40 {
				t.Errorf("%d seeds, %s: %d region calls (%s)", n, sw, c.Shared, c)
			}
			t.Logf("%d seeds %s: %s", n, sw, c)
		}
	}
}

// TestRegionCallsShared: a second member with other seeds adds no region
// calls when both go through one cache.
func TestRegionCallsShared(t *testing.T) {
	src := &countingSource{src: &fakeSource{}, calls: &Calls{}}
	if _, err := Build(context.Background(), src, Input{Member: 1, Switch: "all", Now: day0, Seeds: seedsN(5, 1)}); err != nil {
		t.Fatal(err)
	}
	first := src.calls.Shared
	if _, err := Build(context.Background(), src, Input{Member: 2, Switch: "all", Now: day0, Seeds: seedsN(5, 2)}); err != nil {
		t.Fatal(err)
	}
	if src.calls.Shared != first {
		t.Fatalf("region calls went from %d to %d for a second member", first, src.calls.Shared)
	}
}

// TestPacedSharedByParallelBuilds: three builders at once still get one
// call per Gap between them all (the global rate is unchanged).
func TestPacedSharedByParallelBuilds(t *testing.T) {
	gap := 15 * time.Millisecond
	p := &Paced{Source: &fakeSource{}, Gap: gap}
	var mu sync.Mutex
	var at []time.Time
	var wg sync.WaitGroup
	start := time.Now()
	for b := 0; b < 3; b++ {
		wg.Add(1)
		go func(b int) {
			defer wg.Done()
			for i := 0; i < 4; i++ {
				if _, err := p.List(context.Background(), Request{List: "discover", Kind: "movie", Ref: fmt.Sprint(b, i)}); err != nil {
					t.Error(err)
				}
				mu.Lock()
				at = append(at, time.Now())
				mu.Unlock()
			}
		}(b)
	}
	wg.Wait()
	if p.Calls != 12 {
		t.Fatalf("%d calls, want 12", p.Calls)
	}
	if el := time.Since(start); el < 11*gap {
		t.Fatalf("12 calls from 3 builders took %v, under 11 gaps of %v: the rate went up", el, gap)
	}
}
