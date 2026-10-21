package api

import (
	"strconv"
	"testing"

	"github.com/CoderLuii/holyarrs/internal/explore"
)

func heroItems(kind string, n int) []explore.Item {
	var out []explore.Item
	for i := 0; i < n; i++ {
		out = append(out, explore.Item{TMDBID: kind + strconv.Itoa(i), Kind: kind})
	}
	return out
}

func TestPickHeroAllDiffersFromTabs(t *testing.T) {
	items := append(heroItems("movie", 10), heroItems("show", 10)...)
	head := []explore.Row{{Type: explore.TypeTrending, Items: items}}
	tab := map[string]bool{}
	for _, k := range []string{"movie", "show"} {
		for _, it := range pickHero([]explore.Row{{Type: explore.TypeTrending, Items: heroItems(k, 10)}}, false) {
			tab[it.Key()] = true
		}
	}
	got := pickHero(head, true)
	if len(got) != heroCount {
		t.Fatalf("got %d hero titles, want %d", len(got), heroCount)
	}
	for i, it := range got {
		if tab[it.Key()] {
			t.Errorf("All hero repeats tab hero %s", it.Key())
		}
		if want := []string{"movie", "show"}[i%2]; it.Kind != want {
			t.Errorf("slot %d is %s, want %s", i, it.Kind, want)
		}
	}
}

func TestPickHeroAllFallsBack(t *testing.T) {
	got := pickHero([]explore.Row{{Type: explore.TypeTrending, Items: heroItems("movie", 3)}}, true)
	if len(got) != 3 {
		t.Fatalf("got %d, want the 3 there are", len(got))
	}
}
