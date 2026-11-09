package musicscore

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"
)

type mixFixture struct {
	Expect  []string `json:"expect"`
	Artists []struct {
		Name    string         `json:"name"`
		MBID    string         `json:"mbid"`
		A       float64        `json:"a"`
		Genres  map[string]int `json:"genres"`
		Similar []struct {
			MBID string  `json:"mbid"`
			S    float64 `json:"s"`
		} `json:"similar"`
	} `json:"artists"`
}

func TestClusters_IndieFixture(t *testing.T) {
	set, S, names, want := mixSet(t, "household_indie.json")
	mixes := Clusters(set, S)
	var got []string
	for _, m := range mixes {
		got = append(got, MixTitle(m, names[m.Lead]))
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mixes %q, want %q", got, want)
	}
	again := Clusters(set, S)
	if !reflect.DeepEqual(mixes, again) {
		t.Fatal("clustering is not deterministic")
	}
	d1, d2 := MixSeeds(mixes[0], 1, "2026-10-23"), MixSeeds(mixes[0], 1, "2026-10-24")
	if len(d1) != 3 || reflect.DeepEqual(d1, MixSeeds(mixes[0], 1, "2026-10-23")) == false {
		t.Fatalf("seeds %v", d1)
	}
	top := map[string]bool{}
	for _, id := range mixes[0].Members[:6] {
		top[id] = true
	}
	for _, id := range append(d1, d2...) {
		if !top[id] {
			t.Fatalf("seed %s not in the top 6", id)
		}
	}
}

func TestClusters_TinySet(t *testing.T) {
	set, S, _, _ := mixSet(t, "household_tiny.json")
	if m := Clusters(set, S); len(m) != 0 {
		t.Fatalf("4 unrelated artists gave %d mixes", len(m))
	}
}

func TestClusters_NoVotesJoinsNeighbour(t *testing.T) {
	set, S, _, _ := mixSet(t, "household_indie.json")
	bare := MixArtist{ID: "zz", A: 0.1}
	near := set[0].ID
	mixes := Clusters(append(set, bare), func(a, b string) float64 {
		if a == "zz" && b == near || b == "zz" && a == near {
			return 0.9
		}
		return S(a, b)
	})
	found := false
	for _, m := range mixes {
		for _, id := range m.Members {
			if id == "zz" {
				found = true
				if _, ok := m.Centroid["zz"]; ok {
					t.Fatal("bare artist entered the centroid")
				}
			}
		}
	}
	if !found {
		t.Fatal("bare artist joined no mix")
	}
}

func TestMixScores_ExactValues(t *testing.T) {
	near := func(a, b float64) bool { return a-b < 1e-9 && b-a < 1e-9 }
	if !near(ScoreMix(1, 0.5, 0.4, 0.2), 0.45+0.125+0.08+0.02) {
		t.Fatal("mix score")
	}
	if MixKeeps(0.34, 1, 1) || !MixKeeps(0.35, 0.3, 0) || !MixKeeps(0.5, 0, 0.5) || MixKeeps(0.5, 0.29, 0.49) {
		t.Fatal("mix quality bar")
	}
	if !near(ScoreDeepCut(0.8, DeepRankFrac(3, 5), 1), 0.4+0.15+0.2) || DeepRankFrac(1, 1) != 0 {
		t.Fatal("deep cut score")
	}
	if !near(ScoreAnniversary(0.6, 0.5, 1), 0.3+0.15+0.2) {
		t.Fatal("anniversary score")
	}
	if !near(ScoreTogether(Rel("collaboration"), 0.5, 1), 0.4+0.15+0.2) || Rel("is person") != 0 || Rel("founder") != 1 {
		t.Fatal("together score")
	}
}

func TestAnniversary_Window(t *testing.T) {
	day := func(s string) time.Time { d, _ := time.Parse("2006-01-02", s); return d.Add(12 * time.Hour) }
	if n, _, ok := Anniversary("2001-10-30", day("2026-10-23")); !ok || n != 25 {
		t.Fatalf("2001-10-30 on 2026-10-23: %d %v", n, ok)
	}
	if _, _, ok := Anniversary("2001-10-30", day("2026-12-15")); ok {
		t.Fatal("2001-10-30 shows on 2026-12-15")
	}
	if _, _, ok := Anniversary("2001-10-16", day("2026-10-23")); !ok {
		t.Fatal("−7 days must count")
	}
	if _, _, ok := Anniversary("2001-10-15", day("2026-10-23")); ok {
		t.Fatal("−8 days must not count")
	}
}

// mixSet reads a fixture as a taste set, its similarity function, the names
// and the ids.
func mixSet(t *testing.T, name string) ([]MixArtist, func(a, b string) float64, map[string]string, []string) {
	t.Helper()
	raw, err := os.ReadFile("../testdata/explore_v7/" + name)
	if err != nil {
		t.Fatal(err)
	}
	var fx mixFixture
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatal(err)
	}
	pair := map[[2]string]float64{}
	names := map[string]string{}
	var set []MixArtist
	for _, a := range fx.Artists {
		names[a.MBID] = a.Name
		set = append(set, MixArtist{ID: a.MBID, A: a.A, Share: Shares(a.Genres)})
		for _, s := range a.Similar {
			k := [2]string{min(a.MBID, s.MBID), max(a.MBID, s.MBID)}
			pair[k] = max(pair[k], s.S)
		}
	}
	return set, func(x, y string) float64 { return pair[[2]string{min(x, y), max(x, y)}] }, names, fx.Expect
}
