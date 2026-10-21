package explore

import (
	"testing"
	"time"
)

func items(kind string, ids ...string) []Item {
	var out []Item
	for _, id := range ids {
		out = append(out, Item{TMDBID: id, Kind: kind})
	}
	return out
}

func TestDedupeRowsFirstWinsAndMin(t *testing.T) {
	rows := DedupeRows([]Row{
		{ID: "a", Items: items("movie", "1", "2", "3")},
		{ID: "b", Items: items("movie", "1", "4", "5", "6")},
		{ID: "c", Items: items("show", "1", "2", "2")},
	}, 2)
	if len(rows) != 3 || len(rows[1].Items) != 3 || rows[1].Items[0].TMDBID != "4" || len(rows[2].Items) != 2 {
		t.Fatalf("rows %+v", rows)
	}
	// A row under the minimum goes and does not claim its titles.
	rows = DedupeRows([]Row{{ID: "a", Items: items("movie", "1")}, {ID: "b", Items: items("movie", "1", "2")}}, 2)
	if len(rows) != 1 || rows[0].ID != "b" {
		t.Fatalf("min %+v", rows)
	}
}

func TestFreshnessWindows(t *testing.T) {
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	ago := func(d int) time.Time { return now.AddDate(0, 0, -d) }
	cases := []struct {
		f  NewFact
		in bool
	}{
		{NewFact{NewService, ago(13), 0}, true}, {NewFact{NewService, ago(15), 0}, false},
		{NewFact{NewRelease, ago(44), 0}, true}, {NewFact{NewRelease, ago(46), 0}, false},
		{NewFact{NewSeason, ago(29), 3}, true}, {NewFact{NewSeason, ago(31), 3}, false},
		{NewFact{NewEpisode, ago(13), 0}, true}, {NewFact{NewEpisode, ago(15), 0}, false},
		{NewFact{NewSeries, ago(59), 0}, true}, {NewFact{NewSeries, ago(61), 0}, false},
		{NewFact{NewUpcoming, ago(-13), 0}, true}, {NewFact{NewUpcoming, ago(-15), 0}, false}, {NewFact{NewUpcoming, ago(1), 0}, false},
		{NewFact{NewRelease, ago(-5), 0}, false}, {NewFact{NewRelease, time.Time{}, 0}, false}, {NewFact{"other", ago(1), 0}, false},
	}
	for _, c := range cases {
		if InWindow(c.f, now) != c.in {
			t.Fatalf("%+v: want %v", c.f, c.in)
		}
	}
	if Freshness(NewFact{NewUpcoming, ago(-3), 0}, now) != 1 {
		t.Fatal("upcoming F is 1")
	}
	// τ: a 7-day-old service add is at 1/e; a 14-day-old release too.
	if f := Freshness(NewFact{NewService, ago(7), 0}, now); f < 0.36 || f > 0.37 {
		t.Fatalf("service τ %v", f)
	}
	if f := Freshness(NewFact{NewRelease, ago(14), 0}, now); f < 0.36 || f > 0.37 {
		t.Fatalf("release τ %v", f)
	}
}

// The table test: freshness beats popularity. A 60-day-old title at full
// popularity ranks below a 3-day-old one at P = 0.4.
func TestNewScoreFreshnessFirst(t *testing.T) {
	now := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	old := NewScore(Freshness(NewFact{NewSeries, now.AddDate(0, 0, -60), 0}, now), 1.0, 0)
	fresh := NewScore(Freshness(NewFact{NewRelease, now.AddDate(0, 0, -3), 0}, now), 0.4, 0)
	if !(old < fresh) {
		t.Fatalf("old %v >= fresh %v", old, fresh)
	}
	for _, k := range []string{NewService, NewRelease, NewSeason, NewEpisode} {
		o := NewScore(Freshness(NewFact{k, now.AddDate(0, 0, -60), 0}, now), 1.0, 1)
		if o != 0 {
			t.Fatalf("%s 60 days old scores %v", k, o)
		}
	}
	if NewScore(1, 1, 1) != 1.3 || NewScore(1, 0, 0) != 0.35 {
		t.Fatal("score shape")
	}
}

func TestPopPercentile(t *testing.T) {
	p := PopPercentile([]Item{{TMDBID: "1", Kind: "movie", Pop: 10}, {TMDBID: "2", Kind: "movie", Pop: 50}, {TMDBID: "3", Kind: "movie", Pop: 30}, {TMDBID: "1", Kind: "movie", Pop: 5}})
	if p["movie:2"] != 1 || p["movie:1"] != 0 || p["movie:3"] != 0.5 {
		t.Fatalf("%v", p)
	}
	if PopPercentile([]Item{{TMDBID: "9", Kind: "show"}})["show:9"] != 1 {
		t.Fatal("single")
	}
}

func TestOrderNewRowsAndRankMerge(t *testing.T) {
	rows := OrderNewRows([]Row{
		{Type: TypeNPWorld, Items: items("movie", "1")},
		{Type: TypeNPServices, Items: items("movie", "2")},
		{Type: TypeNPPicks, Items: items("movie", "3")},
	}, func(it Item) float64 {
		if it.TMDBID == "3" {
			return 1
		}
		return 0.5
	}, map[string]float64{TypeNPPicks: 0.4})
	// Picks: 1 × 0.4 = 0.4 < world 0.5 × 1.
	if rows[0].Type != TypeNPServices || rows[1].Type != TypeNPWorld || rows[2].Type != TypeNPPicks {
		t.Fatalf("order %+v", rows)
	}
	m := RankMerge([][]Item{items("show", "1", "2"), items("show", "1", "3")}, 3)
	if len(m) != 3 || m[1].TMDBID != "2" || m[2].TMDBID != "3" {
		t.Fatalf("merge %+v", m)
	}
	c := CapPer(items("movie", "1", "2", "3", "4", "5"), func(it Item) string { return map[string]string{"1": "a", "2": "a", "3": "a", "4": "b", "5": "b"}[it.TMDBID] }, 2, 3)
	if len(c) != 3 || c[2].TMDBID != "4" {
		t.Fatalf("cap %+v", c)
	}
}