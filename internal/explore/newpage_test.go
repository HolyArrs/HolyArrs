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

func TestIsNew(t *testing.T) {
	now := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	cases := map[string]bool{"2026-10-01": true, "2026-06-01": false, "2026-12-01": false}
	for d, want := range cases {
		if IsNew(Item{Kind: "movie", Released: d}, now) != want {
			t.Fatalf("%s", d)
		}
	}
	if !IsNew(Item{Kind: "show"}, now) || IsNew(Item{Kind: "movie"}, now) {
		t.Fatal("unknown dates")
	}
}

func TestOrderNewRowsAndRankMerge(t *testing.T) {
	rows := OrderNewRows([]Row{
		{Type: TypeNPWeek, Items: items("movie", "1")},
		{Type: TypeNPServices, Items: items("movie", "2")},
		{Type: TypeNPPicks, Items: items("movie", "3")},
	}, func(it Item) float64 {
		if it.TMDBID == "3" {
			return 1
		}
		return 0
	}, nil)
	if rows[0].Type != TypeNPServices || rows[1].Type != TypeNPPicks {
		t.Fatalf("order %+v", rows)
	}
	m := RankMerge([][]Item{items("show", "1", "2"), items("show", "1", "3")}, 3)
	if len(m) != 3 || m[1].TMDBID != "2" || m[2].TMDBID != "3" {
		t.Fatalf("merge %+v", m)
	}
}
