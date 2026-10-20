package explore

import (
	"context"
	"fmt"
	"hash/fnv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeSource answers every Request with 40 items drawn from a pool of
// 3000 ids per kind, picked by a hash of the request, so lists overlap a
// little the way TMDB lists do. It records every request.
type fakeSource struct {
	mu   sync.Mutex
	seen []string
}

var fakeGenres = []string{"Drama", "Comedy", "Action", "Thriller", "Animation", "Documentary", "Crime", "Family"}

func (f *fakeSource) List(_ context.Context, q Request) ([]Item, error) {
	f.mu.Lock()
	f.seen = append(f.seen, q.String())
	f.mu.Unlock()
	h := fnv.New64a()
	h.Write([]byte(q.String()))
	base := h.Sum64()
	out := make([]Item, 0, 40)
	for i := 0; i < 40; i++ {
		n := (base + uint64(i)*7919) % 3000
		g := fakeGenres[n%uint64(len(fakeGenres))]
		if q.Genre != "" {
			g = q.Genre
		}
		kind := q.Kind
		if kind == "" { // a person's credits hold both kinds
			kind = []string{"movie", "show"}[i%2]
		}
		out = append(out, Item{TMDBID: fmt.Sprint(n + 1), Kind: kind, Name: fmt.Sprintf("T%d", n), Year: 1980 + int(n%45),
			Genres: []string{g}, Pop: float64(n%97) + 1, Vote: 5 + float64(n%50)/10, Votes: int(n%3000) + 50, Released: "2026-09-01"})
	}
	return out, nil
}

func seedsN(n int, member int64) []Seed {
	var out []Seed
	for i := 0; i < n; i++ {
		out = append(out, Seed{TMDBID: fmt.Sprint(5000 + int(member)*100 + i), Kind: []string{"movie", "show"}[i%2], Name: fmt.Sprintf("Seed%d-%d", member, i),
			Weight: 2 - float64(i)*0.1, Genres: []string{fakeGenres[(i+int(member))%len(fakeGenres)]}, Year: 2000 + i, Recent: true})
	}
	return out
}

func build(t *testing.T, in Input) (Pool, *fakeSource) {
	t.Helper()
	src := &fakeSource{}
	if in.Now.IsZero() {
		in.Now = day0
	}
	if in.Switch == "" {
		in.Switch = "all"
	}
	p, err := Build(context.Background(), src, in)
	if err != nil {
		t.Fatal(err)
	}
	return p, src
}

func top10() []Item {
	var out []Item
	for i := 0; i < 10; i++ {
		out = append(out, Item{TMDBID: fmt.Sprint(i + 1), Kind: "movie", Name: fmt.Sprint("Top", i)})
	}
	return out
}

func order(rows []Row) []string {
	var ids []string
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	return ids
}

// Criterion 2: no title twice in the head; none more than twice on the
// page; repeats at least 4 rows apart. 50 generated members.
func TestDedupFiftyMembers(t *testing.T) {
	for m := int64(1); m <= 50; m++ {
		p, _ := build(t, Input{Member: m, Seeds: seedsN(int(m%15), m), Services: "8|9", Region: "US"})
		pg := Assemble(p, top10(), m)
		at := map[string][]int{}
		rows := append(append([]Row{}, pg.Head...), pg.Tail...)
		for i, r := range rows {
			inRow := map[string]bool{}
			for _, it := range r.Items {
				if inRow[it.Key()] {
					t.Fatalf("member %d row %s repeats %s", m, r.ID, it.Key())
				}
				inRow[it.Key()] = true
				at[it.Key()] = append(at[it.Key()], i)
			}
		}
		for k, idx := range at {
			heads := 0
			for _, i := range idx {
				if i < len(pg.Head) {
					heads++
				}
			}
			if heads > 1 {
				t.Fatalf("member %d: %s twice in the head", m, k)
			}
			if len(idx) > 2 {
				t.Fatalf("member %d: %s %d times", m, k, len(idx))
			}
			if len(idx) == 2 && idx[1]-idx[0] < 4 {
				t.Fatalf("member %d: %s rows %v, under 4 apart", m, k, idx)
			}
		}
	}
}

// Criterion 3: an empty household gets 6 head rows and 6-8 tail rows, each
// at its minimum, and no personal row.
func TestColdStart(t *testing.T) {
	p, _ := build(t, Input{Member: 1})
	pg := Assemble(p, top10(), 1)
	if len(pg.Head) != HeadRows {
		t.Fatalf("head %d rows: %v", len(pg.Head), order(pg.Head))
	}
	if len(pg.Tail) < 6 || len(pg.Tail) > 8 {
		t.Fatalf("tail %d rows: %v", len(pg.Tail), order(pg.Tail))
	}
	for _, r := range append(pg.Head, pg.Tail...) {
		if len(r.Items) < MinItems(r.Type) {
			t.Fatalf("row %s under its minimum", r.ID)
		}
		if Personal(r.Type) || strings.Contains(strings.ToLower(r.Title), "for you") || strings.Contains(r.Title, "Because") || strings.Contains(r.Title, "More from") {
			t.Fatalf("personal row in a cold page: %s %q", r.ID, r.Title)
		}
	}
	if pg.Head[4].Type != TypeBest && pg.Head[4].Type != TypeHidden {
		t.Fatalf("cold head slot 5 is %s, want Best new releases or Hidden gems", pg.Head[4].Type)
	}
}

// Criterion 4: same member and day give the same order; the next day
// moves at least one tail row (typeFreshness frozen within the day).
func TestStableWithinDay(t *testing.T) {
	in := Input{Member: 3, Seeds: seedsN(12, 3)}
	p1, _ := build(t, in)
	in.Today = &p1
	p2, _ := build(t, in)
	a, b := Assemble(p1, top10(), 3), Assemble(p2, top10(), 3)
	if fmt.Sprint(order(a.Head), order(a.Tail)) != fmt.Sprint(order(b.Head), order(b.Tail)) {
		t.Fatal("two calls on the same day differ")
	}
	next := Input{Member: 3, Seeds: seedsN(12, 3), Now: day0.Add(24 * time.Hour), Prev: &p1}
	p3, _ := build(t, next)
	c := Assemble(p3, top10(), 3)
	if fmt.Sprint(order(a.Tail)) == fmt.Sprint(order(c.Tail)) {
		t.Fatal("the next day did not move any tail row")
	}
}

func TestFreshnessFrozenPerDay(t *testing.T) {
	r := Row{ID: "x", Items: []Item{{TMDBID: "1", Kind: "movie"}}}
	if freshness(r, nil, nil) != 1 {
		t.Fatal("day one is not 1")
	}
	prev := &Pool{Rows: []Row{r}}
	if freshness(r, nil, prev) != 0.5 {
		t.Fatal("unchanged row is not 0.5")
	}
	changed := Row{ID: "x", Items: []Item{{TMDBID: "2", Kind: "movie"}}}
	if freshness(changed, nil, prev) != 1 {
		t.Fatal("changed row is not 1")
	}
	today := &Pool{Rows: []Row{{ID: "x", Fresh: 0.5}}}
	if freshness(changed, today, prev) != 0.5 {
		t.Fatal("a same-date rebuild did not keep the stored value")
	}
}

// Criterion 5: a Kid capped at 12 sees nothing over 12 or uncertified;
// each filter is tested alone.
func TestKidFilters(t *testing.T) {
	cert := func(it Item) string {
		var n int
		fmt.Sscan(it.TMDBID, &n)
		switch n % 3 {
		case 0:
			return "PG"
		case 1:
			return "R"
		}
		return ""
	}
	allow := func(it Item) bool { return cert(it) == "PG" }
	check := func(name string, pg Page) {
		if len(pg.Head)+len(pg.Tail) == 0 {
			t.Fatalf("%s: no rows left", name)
		}
		for _, r := range append(pg.Head, pg.Tail...) {
			for _, it := range r.Items {
				if !allow(it) {
					t.Fatalf("%s: %s (%q) in row %s", name, it.Key(), cert(it), r.ID)
				}
			}
		}
	}
	// Candidate stage alone.
	p, _ := build(t, Input{Member: 9, Kid: true, Seeds: seedsN(5, 9), Allow: allow})
	check("candidate", Assemble(p, nil, 9))
	// Response stage alone.
	p, _ = build(t, Input{Member: 9, Kid: true, Seeds: seedsN(5, 9)})
	check("response", Assemble(p, nil, 9).Filter(allow))
	if TailCap(12, true) != 20 {
		t.Fatal("Kid tail not capped at 20")
	}
}

// Criterion 6: no outside request carries a member id, name, play or ask.
func TestPrivacyOfRequests(t *testing.T) {
	in := Input{Member: 424242, Seeds: seedsN(12, 4242), Services: "8", Region: "US", Members: 2,
		Household: []HouseholdTitle{{Item: Item{TMDBID: "77", Kind: "movie", Name: "Asked"}, Asks: 3, Members: 2}}}
	_, src := build(t, in)
	if len(src.seen) == 0 {
		t.Fatal("no requests recorded")
	}
	for _, q := range src.seen {
		for _, bad := range []string{"424242", "Seed4242", "Asked", "member", "user"} {
			if strings.Contains(q, bad) {
				t.Fatalf("request %q carries %q", q, bad)
			}
		}
	}
}

// Criterion 8: two members with different seeds get different Top picks
// and tail orders.
func TestPerMemberOrder(t *testing.T) {
	p1, _ := build(t, Input{Member: 1, Seeds: seedsN(6, 1)})
	p2, _ := build(t, Input{Member: 2, Seeds: seedsN(6, 2)})
	a, b := Assemble(p1, top10(), 1), Assemble(p2, top10(), 2)
	if a.Head[1].Type != TypeTopPicks || b.Head[1].Type != TypeTopPicks {
		t.Fatalf("H2 is not Top picks: %s %s", a.Head[1].Type, b.Head[1].Type)
	}
	if fmt.Sprint(a.Head[1].Items) == fmt.Sprint(b.Head[1].Items) {
		t.Fatal("same Top picks for different seeds")
	}
	if fmt.Sprint(order(a.Tail)) == fmt.Sprint(order(b.Tail)) {
		t.Fatal("same tail order for different seeds")
	}
}

// Criterion 10: 2, 5 and 12 seeds get at most 8, 20 and 40 tail rows; 12
// seeds get more than 20.
func TestTailLength(t *testing.T) {
	for _, c := range []struct{ seeds, max, floor int }{{2, 8, 1}, {5, 20, 1}, {12, 40, 21}} {
		p, _ := build(t, Input{Member: 5, Seeds: seedsN(c.seeds, 5), Services: "8|9", Region: "US",
			OwnedPeople: map[string]int{"Ann": 3, "Bo": 2, "Cy": 2, "Di": 2}, OwnedDecadeGenre: map[string]int{"1990s|Drama": 4, "2000s|Comedy": 3}, Members: 2, Household: household(5)})
		pg := Assemble(p, top10(), 5)
		if len(pg.Tail) > c.max || len(pg.Tail) < c.floor {
			t.Fatalf("%d seeds: %d tail rows, want %d-%d (%v)", c.seeds, len(pg.Tail), c.floor, c.max, order(pg.Tail))
		}
	}
}

func TestDiversityNoSameTypeBackToBack(t *testing.T) {
	p, _ := build(t, Input{Member: 6, Seeds: seedsN(12, 6)})
	pg := Assemble(p, top10(), 6)
	types := map[string]int{}
	for _, r := range pg.Tail {
		types[r.Type]++
	}
	if len(types) < 2 {
		t.Skip("one type only")
	}
	for i := 1; i < len(pg.Tail); i++ {
		if pg.Tail[i].Type == pg.Tail[i-1].Type {
			// Allowed only once every other type ran out.
			rest := map[string]bool{}
			for _, r := range pg.Tail[i:] {
				rest[r.Type] = true
			}
			if len(rest) > 1 {
				t.Fatalf("rows %d and %d both %s", i-1, i, pg.Tail[i].Type)
			}
		}
	}
}

func TestPacedLimiter(t *testing.T) {
	p := &Paced{Source: &fakeSource{}, Gap: 20 * time.Millisecond}
	start := time.Now()
	for i := 0; i < 3; i++ {
		if _, err := p.List(context.Background(), Request{List: "trending", Kind: "movie"}); err != nil {
			t.Fatal(err)
		}
	}
	if time.Since(start) < 40*time.Millisecond || p.Calls != 3 {
		t.Fatalf("3 calls took %v (%d calls)", time.Since(start), p.Calls)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.List(ctx, Request{}); err == nil {
		t.Fatal("a cancelled build kept waiting")
	}
}

func TestFranchise(t *testing.T) {
	a := Item{Kind: "movie", TMDBID: "1", Name: "Dune: Part Two"}
	b := Item{Kind: "movie", TMDBID: "2", Name: "Dune"}
	c := Item{Kind: "movie", TMDBID: "3", Name: "Toy Story 3"}
	d := Item{Kind: "movie", TMDBID: "4", Name: "Toy Story"}
	if a.Franchise() != b.Franchise() || c.Franchise() != d.Franchise() || a.Franchise() == c.Franchise() {
		t.Fatalf("%s %s %s %s", a.Franchise(), b.Franchise(), c.Franchise(), d.Franchise())
	}
	if (Item{Kind: "movie", Name: "X", Collection: 9}).Franchise() != "c9" {
		t.Fatal("collection not used")
	}
}

// Revision 4 head order: Just arrived, Top 10, Your asks, Top picks,
// Trending, Coming soon, then the best tail row; Just arrived keeps the
// newest-first order it was given.
func TestHeadOrderRevision4(t *testing.T) {
	arrived := []Item{{TMDBID: "9001", Kind: "movie", Name: "Newest"}, {TMDBID: "9002", Kind: "show", Name: "Older"}, {TMDBID: "9003", Kind: "movie", Name: "Oldest"}}
	asks := []Item{{TMDBID: "9100", Kind: "movie", Name: "Asked"}}
	p, _ := build(t, Input{Member: 4, Seeds: seedsN(5, 4), JustArrived: arrived, MyAsks: asks})
	pg := Assemble(p, top10(), 4)
	want := []string{TypeJustArrived, TypeTop10, TypeYourAsks, TypeTopPicks, TypeTrending, TypeComingSoon}
	if len(pg.Head) != HeadRows {
		t.Fatalf("head %v", order(pg.Head))
	}
	for i, w := range want {
		if pg.Head[i].Type != w {
			t.Fatalf("head %v, want %v first", order(pg.Head), want)
		}
	}
	if fmt.Sprint(pg.Head[0].Items) != fmt.Sprint(arrived) {
		t.Fatal("Just arrived lost its newest-first order")
	}
	// Without open asks, the next candidate moves up.
	p, _ = build(t, Input{Member: 4, Seeds: seedsN(5, 4), JustArrived: arrived})
	if pg := Assemble(p, top10(), 4); pg.Head[2].Type != TypeTopPicks {
		t.Fatalf("head without asks %v", order(pg.Head))
	}
}

func household(n int) []HouseholdTitle {
	var out []HouseholdTitle
	for i := 0; i < n; i++ {
		out = append(out, HouseholdTitle{Item: Item{TMDBID: fmt.Sprint(8000 + i), Kind: "movie", Name: fmt.Sprint("H", i)}, Asks: 2, Members: 2})
	}
	return out
}
