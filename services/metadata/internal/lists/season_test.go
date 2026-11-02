package lists

import (
	"path/filepath"
	"testing"
	"time"
)

func TestPlan900FundsFocusSleepAndOneSeasonSlot(t *testing.T) {
	p := Plan(900)
	if c := PlanCost(p); c != 810 {
		t.Fatalf("Plan(900) cost = %d, want 810 (the 90%% budget)", c)
	}
	n := map[Kind]int{}
	keys := map[string]Source{}
	for _, s := range p {
		n[s.Kind]++
		keys[s.Key] = s
	}
	// New releases are read for the US only; the calls go to more charts.
	if n[KindSongs] != 17 || n[KindAlbums] != 16 || n[KindArtists] != 16 || n[KindNew] != 1 || keys["new:us"].Key == "" {
		t.Fatalf("charts = %v", n)
	}
	for _, k := range []string{"list:focus:0", "list:sleep:0", "list:season:0", "category:season"} {
		if _, ok := keys[k]; !ok {
			t.Fatalf("%s missing from Plan(900)", k)
		}
	}
	if keys["category:pop"].Every != 31*day {
		t.Fatalf("category refresh = %v, want 31 days", keys["category:pop"].Every)
	}
	if c := PlanCost(Plan(6000)); c > 5400 {
		t.Fatalf("Plan(6000) cost = %d", c)
	}
}

func TestActiveSeason(t *testing.T) {
	cases := map[string]string{"2026-10-24": "halloween", "2026-11-20": "christmas", "2026-07-04": "summer", "2026-03-01": "", "2026-11-05": ""}
	for d, want := range cases {
		at, _ := time.Parse("2006-01-02", d)
		if got := ActiveSeason(at); got != want {
			t.Errorf("%s: %q, want %q", d, got, want)
		}
	}
}

func TestSeasonSlotDue(t *testing.T) {
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	s, err := Open(filepath.Join(t.TempDir(), "l.sqlite"), Options{Key: "k", Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	src := Source{Key: "list:season:0", Kind: KindList, Every: weekly, Tile: Tile{Key: "season"}}
	if !s.due(src) {
		t.Fatal("never read: due")
	}
	s.store(src, []byte(`[1]`))
	s.setSeason(src.Key, "halloween")
	now = now.Add(24 * time.Hour)
	if s.due(src) {
		t.Fatal("read yesterday for this season: not due")
	}
	now = time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	if !s.due(src) {
		t.Fatal("Christmas is on and the stored list is Halloween's: due")
	}
	now = time.Date(2027, 3, 1, 12, 0, 0, 0, time.UTC)
	if s.due(src) {
		t.Fatal("no paid season on: never due")
	}
}
