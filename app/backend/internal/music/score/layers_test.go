package musicscore

import (
	"math"
	"testing"
	"time"
)

func TestLnorm_Bounds(t *testing.T) {
	if Lnorm(100, 0) != 0 || Lnorm(0, 0) != 0 {
		t.Fatal("Lmax 0 or empty pool must give 0")
	}
	if !near(Lnorm(999, 999), 1) || !near(Lnorm(9, 99), 0.5) {
		t.Fatal(Lnorm(9, 99))
	}
}

func TestGenreAffinity_NoTags(t *testing.T) {
	if g := GenreAffinity(map[string]float64{"a": 1}, nil); len(g) != 0 {
		t.Fatal("no tags: every G is 0")
	}
	g := GenreAffinity(map[string]float64{"a": 1, "b": 0.5}, map[string]map[string]float64{"a": {"rock": 1}, "b": {"pop": 1}})
	if len(g) != 0 {
		t.Fatal(g)
	}
	if g := GenreAffinity(map[string]float64{}, map[string]map[string]float64{"a": {"rock": 1}, "b": {"rock": 1}}); len(g) != 0 {
		t.Fatal(g)
	}
	g = GenreAffinity(map[string]float64{"a": 1, "b": 0.5, "c": 1}, map[string]map[string]float64{"a": {"rock": 1, "pop": 0.5}, "b": {"rock": 1}, "c": {"pop": 0.5}})
	if !near(g["rock"], 1) || !near(g["pop"], 1.0/1.5) {
		t.Fatal(g)
	}
}

func TestFreshness_Decay(t *testing.T) {
	now := time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)
	if Freshness("", 14, now) != 0 || Freshness("unknown!!!", 14, now) != 0 {
		t.Fatal("unknown date → 0")
	}
	if !near(Freshness("2027-01-01", 7, now), 1) || !near(Freshness("2026-10-15", 7, now), math.Exp(-1)) {
		t.Fatal("F")
	}
}

func TestFormulas_ExactValues(t *testing.T) {
	if !near(Affinity(Acts{Follows: true, Asked: true, HaveAny: true, AlbumsHad: 3}), 1) {
		t.Fatal("A caps at 1")
	}
	if !near(Affinity(Acts{HaveAny: true, AlbumsHad: 1}), 1.5/6) || Affinity(Acts{}) != 0 {
		t.Fatal("A")
	}
	if Popularity(1, 0) != 0 || !near(Popularity(3, 4), 0.5) || !near(Uncharted(1), 0.3) {
		t.Fatal("P")
	}
	if n := Norm([]float64{0, 0}); n[0] != 0 || n[1] != 0 {
		t.Fatal("norm max 0")
	}
	if Tie(1, "2026-10-22", "x") != Tie(1, "2026-10-22", "x") || Tie(1, "2026-10-22", "x") == Tie(1, "2026-10-23", "x") {
		t.Fatal("tie")
	}
}

func TestRowGate_Minimum(t *testing.T) {
	r := Row{ID: "x"}
	if r.Passes(5) || !r.Passes(6) {
		t.Fatal("gate 6")
	}
	if !(Row{Min: 1}).Passes(1) {
		t.Fatal("asks gate 1")
	}
	rows := Dedup([]Row{{ID: "five", Items: items("f", 5)}, {ID: "six", Items: items("s", 6)}}, nil)
	if len(rows) != 1 || rows[0].ID != "six" {
		t.Fatal(rows)
	}
}

func TestStrength_OneItem(t *testing.T) {
	if !near(Strength([]float64{0.5}, PriorPersonal), 0.6) {
		t.Fatal("R with n = 1")
	}
	if !near(Strength([]float64{1, 1, 1, 1, 1, 1, 0, 0}, 1), 1) || Strength(nil, 1) != 0 {
		t.Fatal("R top 6")
	}
}

func TestDedup_SharedAlbumGoesToEarlierRow(t *testing.T) {
	shared := Item{Key: "album:7", Album: "rg-shared", Artist: "z", Score: 1}
	row6 := Row{ID: "missing", Prior: PriorPersonal, Items: append(items("m", 6), shared)}
	row7 := Row{ID: "top_albums", Prior: PriorChart, Items: append([]Item{{Key: "chart:1", Album: "rg-shared", Artist: "z", Score: 1}}, items("t", 6)...)}
	ordered := Order([]Row{row7, row6}, 1, "2026-10-22")
	if ordered[0].ID != "missing" {
		t.Fatal("personal prior first")
	}
	out := Dedup(ordered, nil)
	count := 0
	for _, r := range out {
		for _, it := range r.Items {
			if it.Album == "rg-shared" {
				count++
				if r.ID != "missing" {
					t.Fatal("shared album must stay in the earlier row")
				}
			}
		}
	}
	if count != 1 {
		t.Fatal(count)
	}
	a := Row{ID: "a", Items: append(items("x", 6), Item{Key: "k1", FoldKey: "album|artist"})}
	b := Row{ID: "b", Items: append(items("y", 6), Item{Key: "k2", FoldKey: "album|artist"})}
	out = Dedup([]Row{a, b}, nil)
	if len(out[1].Items) != 6 {
		t.Fatal("fold key")
	}
}

func TestDedup_DroppedRowDoesNotChangeLaterRows(t *testing.T) {
	short := Row{ID: "short", Anchor: true, Items: append(items("s", 3), Item{Key: "p", Album: "rg-p", Artist: "q"})}
	mid := Row{ID: "mid", Anchor: true, Items: items("m", 6)}
	late := Row{ID: "late", Anchor: true, Items: append([]Item{{Key: "p2", Album: "rg-p", Artist: "q"}}, items("l", 6)...)}
	with := Dedup(Order([]Row{short, mid, late}, 1, "d"), nil)
	if len(with) != 2 || with[0].ID != "mid" || with[1].ID != "late" {
		t.Fatal(with)
	}
	if len(with[1].Items) != 6 || with[1].Items[0].Key != "la" {
		t.Fatal("the dropped row's album stays taken", with[1].Items)
	}
	without := Dedup(Order([]Row{mid, late}, 1, "d"), map[string]bool{"rg:rg-p": true})
	for i := range without[1].Items {
		if without[1].Items[i].Key != with[1].Items[i].Key {
			t.Fatal("later rows changed")
		}
	}
}

func TestDedup_PerArtistAndSeeds(t *testing.T) {
	var it []Item
	for i := 0; i < 8; i++ {
		it = append(it, Item{Key: string(rune('a' + i)), Artist: "one"})
	}
	if out := Dedup([]Row{{ID: "r", Min: 1, Items: it}}, nil); len(out[0].Items) != 2 {
		t.Fatal("max 2 per artist")
	}
	if out := Dedup([]Row{{ID: "r", Min: 1, ArtistRow: true, Items: it}}, nil); len(out[0].Items) != 8 {
		t.Fatal("artist rows exempt")
	}
	out := Dedup([]Row{{ID: "a", Seed: "s", Min: 1, Items: items("a", 1)}, {ID: "b", Seed: "s", Min: 1, Items: items("b", 1)}}, nil)
	if len(out) != 1 {
		t.Fatal("one row per seed")
	}
}

func TestOrder_Ties(t *testing.T) {
	a := Row{ID: "a", Items: []Item{{Score: 1}}}
	b := Row{ID: "b", Items: []Item{{Score: 1}}}
	x := Order([]Row{a, b}, 7, "2026-10-22")
	y := Order([]Row{b, a}, 7, "2026-10-22")
	if x[0].ID != y[0].ID {
		t.Fatal("ties must not depend on input order")
	}
}

func TestDays_Cache(t *testing.T) {
	var d Days
	k := DayKey{1, "2026-10-22", ""}
	d.Put(k, "order")
	if v, ok := d.Get(k); !ok || v != "order" {
		t.Fatal("get")
	}
	d.Put(DayKey{2, "2026-10-23", ""}, "x")
	if _, ok := d.Get(k); ok {
		t.Fatal("a new day drops the old one")
	}
	d.Invalidate(2)
	if _, ok := d.Get(DayKey{2, "2026-10-23", ""}); ok {
		t.Fatal("invalidate")
	}
	var c Candidates
	now := time.Now()
	c.Put("lib:x", 1, time.Minute, now)
	c.Put("chart:x", 1, time.Hour, now)
	c.Forget("lib:")
	if _, ok := c.Get("lib:x", now); ok {
		t.Fatal("forget")
	}
	if _, ok := c.Get("chart:x", now.Add(2*time.Hour)); ok {
		t.Fatal("ttl")
	}
}

// near reports whether two floats match within a tiny tolerance.
func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// items builds n items with the given key prefix.
func items(prefix string, n int) []Item {
	var out []Item
	for i := 0; i < n; i++ {
		k := prefix + string(rune('a'+i))
		out = append(out, Item{Key: k, Album: "rg-" + k, Artist: "artist-" + k, Score: 1})
	}
	return out
}
