package suggest

import (
	"reflect"
	"strconv"
	"testing"
)

func TestHeroCandidates_SourcesOrderAndOwned(t *testing.T) {
	trend := heroItems("movie", 3, "Drama")
	trend[1].Owned = true
	svc := []Item{trend[0], {TMDBID: "s1", Kind: "show", Genres: []string{"Crime"}}}
	head := []Row{
		{Type: TypeBest, Items: heroItems("movie", 2, "Drama")},
		{Type: TypeTopServices, Items: svc},
		{Type: TypeTrending, Items: trend},
	}
	got := keys(HeroCandidates(head, nil, 10))
	want := []string{"movie:movie0", "movie:movie2", "show:s1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	head = append(head, Row{Type: TypeTrending, Items: []Item{{TMDBID: "x", Kind: "movie"}}})
	if got := keys(HeroCandidates(head, nil, 10)); len(got) != 3 {
		t.Fatalf("unknown genres kept: %v", got)
	}
	if n := len(HeroCandidates(head, nil, 2)); n != 2 {
		t.Fatalf("pool cap: got %d", n)
	}
}

func TestHeroExcluded_UnlessHouseholdLeans(t *testing.T) {
	for _, g := range []string{"Animation", "Kids", "Family", "Documentary", "Reality"} {
		if !HeroExcluded([]string{"Comedy", g}, nil) {
			t.Errorf("%s not excluded", g)
		}
		if HeroExcluded([]string{"Comedy", g}, []string{g}) {
			t.Errorf("%s excluded though the household leans on it", g)
		}
	}
	if HeroExcluded([]string{"Drama"}, nil) {
		t.Error("Drama excluded")
	}
	head := []Row{{Type: TypeTrending, Items: append(heroItems("movie", 2, "Animation"), heroItems("show", 1, "Drama")...)}}
	if got := keys(HeroCandidates(head, nil, 10)); !reflect.DeepEqual(got, []string{"show:show0"}) {
		t.Fatalf("got %v", got)
	}
}

func TestHeroRank_TasteBoost(t *testing.T) {
	cands := heroItems("movie", 5, "Drama")
	cands[1].Genres = []string{"Horror"}
	if got := keys(HeroRank(cands, []string{"Horror"})); got[0] != "movie:movie1" {
		t.Fatalf("taste match should move up: %v", got)
	}
	if got := keys(HeroRank(cands, nil)); !reflect.DeepEqual(got, keys(cands)) {
		t.Fatalf("no taste keeps order: %v", got)
	}
	long := heroItems("movie", 10, "Drama")
	long[9].Genres = []string{"Horror"}
	if got := HeroRank(long, []string{"Horror"}); got[0].TMDBID != "movie0" {
		t.Fatalf("boost too strong: %v", keys(got))
	}
	if HeroTaste(cands[0], []string{"Drama", "Horror"}) != 0.5 {
		t.Fatal("HeroTaste share")
	}
}

func TestHeroShuffle_Daily(t *testing.T) {
	items := heroItems("movie", 15)
	a := keys(HeroShuffle(items, 1, "2026-10-20"))
	if !reflect.DeepEqual(a, keys(HeroShuffle(items, 1, "2026-10-20"))) {
		t.Fatal("not stable within a day")
	}
	if reflect.DeepEqual(a, keys(HeroShuffle(items, 1, "2026-10-21"))) && reflect.DeepEqual(a, keys(HeroShuffle(items, 1, "2026-10-22"))) {
		t.Fatal("does not change by day")
	}
	if reflect.DeepEqual(a, keys(HeroShuffle(items, 2, "2026-10-20"))) && reflect.DeepEqual(a, keys(HeroShuffle(items, 3, "2026-10-20"))) {
		t.Fatal("does not change by member")
	}
	if !reflect.DeepEqual(a[HeroShuffleTop:], keys(items)[HeroShuffleTop:]) {
		t.Fatal("shuffled beyond the top")
	}
}

func TestPickHero_Tabs(t *testing.T) {
	got := PickHero(heroItems("movie", 15), false, 1, "2026-10-20")
	if len(got) != HeroCount {
		t.Fatalf("got %d", len(got))
	}
	top := map[string]bool{}
	for _, k := range keys(heroItems("movie", HeroShuffleTop)) {
		top[k] = true
	}
	for _, it := range got {
		if !top[it.Key()] {
			t.Errorf("%s is not from the top candidates", it.Key())
		}
	}
}

func TestPickHero_AllDiffersFromTabs(t *testing.T) {
	var ranked []Item
	m, s := heroItems("movie", 16), heroItems("show", 16)
	for i := range m {
		ranked = append(ranked, m[i], s[i])
	}
	tab := map[string]bool{}
	for _, k := range []string{"movie", "show"} {
		for _, d := range []string{"2026-10-20", "2026-10-21"} {
			for _, it := range PickHero(heroItems(k, 16), false, 1, d) {
				tab[it.Key()] = true
			}
		}
	}
	got := PickHero(ranked, true, 1, "2026-10-20")
	if len(got) != HeroCount {
		t.Fatalf("got %d", len(got))
	}
	for i, it := range got {
		if tab[it.Key()] {
			t.Errorf("All repeats tab hero %s", it.Key())
		}
		if want := []string{"movie", "show"}[i%2]; it.Kind != want {
			t.Errorf("slot %d is %s, want %s", i, it.Kind, want)
		}
	}
	if n := len(PickHero(heroItems("movie", 3), true, 1, "d")); n != 3 {
		t.Fatalf("fallback: got %d", n)
	}
}

func heroItems(kind string, n int, genres ...string) []Item {
	var out []Item
	for i := 0; i < n; i++ {
		out = append(out, Item{TMDBID: kind + strconv.Itoa(i), Kind: kind, Genres: genres})
	}
	return out
}

func keys(items []Item) []string {
	var out []string
	for _, it := range items {
		out = append(out, it.Key())
	}
	return out
}
