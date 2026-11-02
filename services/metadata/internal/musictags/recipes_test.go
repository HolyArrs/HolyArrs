package musictags

import (
	"strings"
	"testing"
)

func TestRecipesCoverBrowseKeys(t *testing.T) {
	rs, err := Recipes()
	if err != nil {
		t.Fatal(err)
	}
	count := map[string]int{}
	seen := map[string]bool{}
	for _, r := range rs {
		if seen[r.Key] {
			t.Fatalf("duplicate key %s", r.Key)
		}
		seen[r.Key] = true
		if !strings.HasPrefix(r.Key, r.Kind+":") {
			t.Fatalf("%s: key does not start with its kind %s", r.Key, r.Kind)
		}
		if r.Name == "" {
			t.Fatalf("%s: no name", r.Key)
		}
		if r.Kind != KindDecade && len(r.Tags) == 0 && len(r.ChartGenres) == 0 && r.Key != "mood:throwback" {
			t.Fatalf("%s: no tags", r.Key)
		}
		if r.Kind == KindSub && r.Parent == "" {
			t.Fatalf("%s: substyle without a parent genre", r.Key)
		}
		count[r.Kind]++
	}
	want := map[string]int{KindGenre: 22, KindMood: 12, KindSeason: 12, KindDecade: 7, KindActivity: 7}
	for k, n := range want {
		if count[k] != n {
			t.Errorf("%s keys = %d, want %d", k, count[k], n)
		}
	}
	if count[KindSub] < 40 {
		t.Errorf("substyles = %d, want at least 40", count[KindSub])
	}
	for _, r := range rs {
		if r.Key == "season:pride" && !r.AlbumTagsOnly {
			t.Error("pride must use album tags only")
		}
		if r.Kind == KindSub && !seen[r.Parent] {
			t.Errorf("%s: parent %s is not a key", r.Key, r.Parent)
		}
	}
}

func TestRecipeTagsHaveNoBrandWords(t *testing.T) {
	rs, _ := Recipes()
	for _, r := range rs {
		for _, w := range []string{"spotify", "apple", "deezer", "listenbrainz", "musicbrainz", "youtube"} {
			if strings.Contains(strings.ToLower(r.Name), w) {
				t.Errorf("%s: brand word in name", r.Key)
			}
		}
	}
}
