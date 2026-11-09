package suggest

import (
	"math"
	"strconv"
	"testing"
	"time"
)

// stripFixture returns one candidate per Browse category, only kid-safe ones for a Kid,
// with popularity falling down the table.
func stripFixture(kid bool) []StripCand {
	var out []StripCand
	for i, c := range Categories {
		if kid && !c.Kid {
			continue
		}
		t := CategoryTile{BrowseTile: BrowseTile{Name: c.Name, Backdrop: "/b" + strconv.Itoa(i)}, Category: c.ID, Section: c.Section}
		cand := NewStripCand(t, nil, time.Time{})
		cand.Pop = float64(1000 - i)
		out = append(out, cand)
	}
	return out
}

// may is a Wednesday afternoon.
var may = time.Date(2026, 5, 13, 14, 0, 0, 0, time.UTC)

// stripIn builds a strip input for a member with no history.
func stripIn(cands []StripCand) StripIn {
	return StripIn{Cands: cands, Local: may, Seed: "anna|2026-05-13", Max: StripMax}
}

// stripKeys returns the keys of the picked tiles.
func stripKeys(tiles []CategoryTile) map[string]bool {
	out := map[string]bool{}
	for _, t := range tiles {
		out[t.Category] = true
	}
	return out
}

func TestPickStrip_SizeAndCaps(t *testing.T) {
	got := PickStrip(stripIn(stripFixture(false)))
	if len(got) != StripMax {
		t.Fatalf("got %d tiles", len(got))
	}
	sections, kins := map[string]int{}, map[string]int{}
	for _, x := range got {
		sections[x.Section]++
		kins[StripKin(x.Name)]++
	}
	for s, n := range sections {
		if n > stripSectionCap {
			t.Errorf("section %s has %d tiles", s, n)
		}
	}
	for k, n := range kins {
		if n > 1 {
			t.Errorf("kin %s has %d tiles", k, n)
		}
	}
	if len(sections) < 6 {
		t.Errorf("only %d sections: %v", len(sections), sections)
	}
}

func TestPickStrip_SkipsWeakSections(t *testing.T) {
	var cands []StripCand
	for i, c := range Categories {
		if c.Section == "eras" {
			continue
		}
		cand := NewStripCand(CategoryTile{BrowseTile: BrowseTile{Name: c.Name}, Category: c.ID, Section: c.Section}, nil, time.Time{})
		cand.Pop = 1e6 - float64(i)
		cands = append(cands, cand)
	}
	weak := NewStripCand(CategoryTile{BrowseTile: BrowseTile{Name: "Weak"}, Category: "weak", Section: "eras"}, nil, time.Time{})
	weak.Pop = 0
	in := stripIn(append(cands, weak))
	in.Recent = map[string]bool{"weak": true}
	if stripKeys(PickStrip(in))["weak"] {
		t.Fatal("a weak section still got a slot")
	}
}

func TestPickStrip_ExploreSlot(t *testing.T) {
	cands := stripFixture(false)
	in := stripIn(cands)
	first := PickStrip(in)
	in.Recent = stripKeys(first)
	next := PickStrip(in)
	fresh := 0
	for _, x := range next {
		if !in.Recent[x.Category] {
			fresh++
		}
	}
	if fresh < 3 || fresh > 4 {
		t.Fatalf("explore slot has %d new categories, want 3-4", fresh)
	}
	again := PickStrip(in)
	for i := range next {
		if next[i].Category != again[i].Category {
			t.Fatal("same seed, different strip")
		}
	}
	in.Seed = "anna|2026-05-14"
	other := PickStrip(in)
	diff := 0
	for k := range stripKeys(other) {
		if !stripKeys(next)[k] {
			diff++
		}
	}
	if diff == 0 {
		t.Fatal("another day drew the same explore slot")
	}
}

func TestPickStrip_DropsRowsBelow(t *testing.T) {
	in := stripIn(stripFixture(false))
	if !stripKeys(PickStrip(in))["action"] {
		t.Fatal("fixture: Action should lead")
	}
	in.Rows = []string{"Popular Action this week", "Because you asked for Alien"}
	if stripKeys(PickStrip(in))["action"] {
		t.Fatal("Action shows though a row below is Action")
	}
	if !InRows("Sci-Fi", []string{"Top Sci-Fi picks"}) || InRows("War", []string{"Star Wars marathon"}) {
		t.Fatal("InRows word match")
	}
}

func TestPickStrip_SeasonAndHour(t *testing.T) {
	c := StripCand{Key: "halloween", Tile: CategoryTile{Section: "collections"}}
	oct := time.Date(2026, 10, 14, 14, 0, 0, 0, time.UTC)
	if StripBoost(c, oct) <= 0 || StripBoost(c, may) != 0 {
		t.Fatal("Halloween boost")
	}
	if StripBoost(StripCand{Key: "summer"}, time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)) <= 0 {
		t.Fatal("summer boost")
	}
	if StripBoost(StripCand{Key: "holiday"}, time.Date(2026, 12, 1, 10, 0, 0, 0, time.UTC)) <= 0 {
		t.Fatal("holiday boost")
	}
	cozy := StripCand{Key: "cozy"}
	if StripBoost(cozy, time.Date(2026, 5, 13, 20, 0, 0, 0, time.UTC)) <= 0 || StripBoost(cozy, may) != 0 {
		t.Fatal("evening boost")
	}
	kids := StripCand{Key: "animation"}
	sat := time.Date(2026, 5, 16, 9, 0, 0, 0, time.UTC)
	if StripBoost(kids, sat) <= 0 || StripBoost(kids, sat.Add(5*time.Hour)) != 0 || StripBoost(kids, may.Add(-5*time.Hour)) != 0 {
		t.Fatal("weekend morning boost")
	}
	in := stripIn(stripFixture(false))
	in.Local = oct
	var cands []StripCand
	for _, x := range in.Cands {
		if x.Key == "halloween" {
			x.Pop = 1
		}
		cands = append(cands, x)
	}
	in.Cands = cands
	in.Taste = NewStripTaste([]TasteTitle{{Genres: []string{"Western"}, Weight: 1}})
	got := stripKeys(PickStrip(in))
	if got["summer"] {
		t.Fatal("Summer & beach out of season")
	}
	if !got["halloween"] {
		t.Fatal("Halloween missing in October")
	}
}

func TestPickStrip_Freshness(t *testing.T) {
	now := time.Date(2026, 5, 13, 0, 0, 0, 0, time.UTC)
	items := []Item{{Pop: 5, Released: "2026-05-05"}, {Pop: 9, Released: "2019-01-01"}}
	c := NewStripCand(CategoryTile{Category: "heists", Section: "stories"}, items, now)
	if !c.Fresh || StripBoost(c, now) <= 0 {
		t.Fatal("a release 8 days ago is fresh")
	}
	old := NewStripCand(CategoryTile{Category: "heists"}, []Item{{Released: "2026-04-01"}, {Released: "2026-06-01"}}, now)
	if old.Fresh {
		t.Fatal("old and future releases are not fresh")
	}
	if len(c.Keywords) == 0 {
		t.Fatal("category keywords missing")
	}
}

func TestPickStrip_MemberTaste(t *testing.T) {
	taste := NewStripTaste([]TasteTitle{
		{Genres: []string{"Horror"}, Weight: 2},
		{Genres: []string{"Horror"}, Weight: 2},
		{Genres: []string{"Comedy"}, Weight: 0.5},
	})
	if taste.Genres["Horror"] != 1 || taste.Genres["Comedy"] >= 0.5 {
		t.Fatalf("taste %v", taste.Genres)
	}
	horror := StripCand{Key: "x", Genres: []string{"Horror", "Horror", "Thriller"}}
	comedy := StripCand{Key: "y", Genres: []string{"Comedy"}}
	if taste.GenreMatch(horror) <= taste.GenreMatch(comedy) {
		t.Fatal("genre match")
	}
	cands := stripFixture(false)
	for i := range cands {
		cands[i].Genres = []string{"Drama"}
		if cands[i].Key == "remakes" {
			cands[i].Genres, cands[i].Pop = []string{"Horror"}, 1
		}
	}
	in := stripIn(cands)
	in.Taste = taste
	got := PickStrip(in)
	if len(got) == 0 || got[0].Category != "remakes" {
		t.Fatalf("liked category not first: %v", got[0].Category)
	}
}

func TestPickStrip_KeywordOverlap(t *testing.T) {
	taste := NewStripTaste([]TasteTitle{{Keywords: []string{"Heist", "bank robbery"}, Weight: 1}, {Keywords: []string{"space"}, Weight: 0.2}})
	heists := NewStripCand(CategoryTile{Category: "heists", Section: "stories"}, nil, time.Time{})
	space := NewStripCand(CategoryTile{Category: "space", Section: "worlds"}, nil, time.Time{})
	zombies := NewStripCand(CategoryTile{Category: "zombies", Section: "stories"}, nil, time.Time{})
	h, s, z := taste.KeywordMatch(heists), taste.KeywordMatch(space), taste.KeywordMatch(zombies)
	if !(h > s && s > z && z == 0) {
		t.Fatalf("overlap heists %v space %v zombies %v", h, s, z)
	}
	in := StripIn{Taste: taste, Local: may}
	if StripScore(heists, in, 0, false) <= StripScore(zombies, in, 0, false) {
		t.Fatal("keyword taste does not score")
	}
}

func TestPickStrip_OpenRateAndDecay(t *testing.T) {
	if got := StripOpenScore(StripStats{}); got != 0.2 {
		t.Fatalf("prior %v", got)
	}
	if got := StripOpenScore(StripStats{Shown: 5, Opened: 4}); math.Abs(got-0.5) > 1e-9 {
		t.Fatalf("rate %v", got)
	}
	now := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)
	day := func(n int) time.Time { return now.AddDate(0, 0, -n) }
	stats, recent := StripStatsFrom([]CategoryDay{
		{Category: "a", Day: day(0), Shown: 10},
		{Category: "b", Day: day(20), Shown: 10},
		{Category: "c", Day: day(40), Shown: 10, Opened: 10},
		{Category: "d", Day: day(25), Shown: 3, Opened: 3},
	}, now)
	if !(stats["a"].Shown == 10 && stats["b"].Shown < 3 && stats["b"].Shown > 0) {
		t.Fatalf("skips decay: %+v", stats)
	}
	if _, ok := stats["c"]; ok {
		t.Fatal("rows older than 30 days count")
	}
	if stats["d"].Opened != 3 || stats["d"].Shown != 3 {
		t.Fatalf("opens do not decay: %+v", stats["d"])
	}
	if !recent["a"] || recent["b"] {
		t.Fatalf("recent %v", recent)
	}
	in := StripIn{Local: may, Stats: map[string]StripStats{"liked": {Shown: 6, Opened: 5}, "skipped": {Shown: 30}}}
	liked, skipped, plain := StripCand{Key: "liked"}, StripCand{Key: "skipped"}, StripCand{Key: "plain"}
	if !(StripScore(liked, in, 0, false) > StripScore(plain, in, 0, false) && StripScore(plain, in, 0, false) > StripScore(skipped, in, 0, false)) {
		t.Fatal("open rate does not order")
	}
}

func TestPickStrip_ColdStartAndKid(t *testing.T) {
	got := PickStrip(stripIn(stripFixture(false)))
	n := 0
	for _, x := range got {
		if has(StripDefaults, x.Category) {
			n++
		}
	}
	if n < 10 {
		t.Fatalf("cold start shows %d editor picks", n)
	}
	kid := stripIn(stripFixture(true))
	kid.Kid = true
	for _, x := range PickStrip(kid) {
		c, ok := CategoryByID(x.Category)
		if !ok || !c.Kid {
			t.Fatalf("Kid got %s", x.Category)
		}
	}
	for _, id := range []string{"classic-animation", "90s-family", "french-animation", "japanese-family", "world-adventures"} {
		c, ok := CategoryByID(id)
		if !ok || !c.Kid || len(c.Requests("all", true)) == 0 {
			t.Fatalf("%s missing or not kid-safe", id)
		}
	}
}

// TestPickStrip_SectionCapPartialThenFull checks a partial answer and the full one
// both keep at most stripSectionCap tiles per section.
func TestPickStrip_SectionCapPartialThenFull(t *testing.T) {
	all := stripFixture(false)
	var partial []StripCand
	for _, c := range all {
		if c.Tile.Section == "genres" || c.Tile.Section == "moods" {
			partial = append(partial, c)
		}
	}
	for _, cands := range [][]StripCand{partial, all} {
		in := stripIn(cands)
		in.Recent = map[string]bool{}
		per := map[string]int{}
		for _, x := range PickStrip(in) {
			per[x.Section]++
		}
		for s, n := range per {
			if n > stripSectionCap {
				t.Fatalf("%d candidates: section %s has %d", len(cands), s, n)
			}
		}
	}
}
