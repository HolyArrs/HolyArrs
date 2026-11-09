package suggest

import (
	"encoding/json"
	"os"
	"testing"
)

// Movies share 5% and shows 15%, both under 50%, so the row stays unchanged.
func TestBestDecision_LowOverlapKeeps(t *testing.T) {
	b, err := os.ReadFile("testdata/best_overlap_20261019.json")
	if err != nil {
		t.Fatal(err)
	}
	var s overlapSnapshot
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	movie := TopOverlap(s.Movie.Trending, s.Movie.Best, 20)
	show := TopOverlap(s.Show.Trending, s.Show.Best, 20)
	if movie != 0.05 || show != 0.15 {
		t.Fatalf("overlap movie %.2f show %.2f, want 0.05 and 0.15", movie, show)
	}
	if got := BestDecision(movie, show); got != BestKeep {
		t.Fatalf("decision %q, want keep", got)
	}
}

func TestBestDecision_Rule(t *testing.T) {
	cases := []struct {
		movie, show float64
		want        string
	}{{0.49, 0.49, BestKeep}, {0.5, 0.1, BestReframe}, {0.1, 0.55, BestReframe}, {0, 0, BestKeep}}
	for _, c := range cases {
		if got := BestDecision(c.movie, c.show); got != c.want {
			t.Errorf("BestDecision(%v, %v) = %q, want %q", c.movie, c.show, got, c.want)
		}
	}
}

func TestTopOverlap_CountsOnlyTopN(t *testing.T) {
	if got := TopOverlap([]string{"1", "2", "3"}, []string{"3", "2", "9"}, 2); got != 0.5 {
		t.Fatalf("got %v, want 0.5 (only 2 is shared in the top 2)", got)
	}
	if got := TopOverlap(nil, nil, 20); got != 0 {
		t.Fatalf("empty lists: %v", got)
	}
}

type overlapSnapshot struct {
	Movie, Show struct {
		Trending []string `json:"trending_week"`
		Best     []string `json:"best_new_releases"`
	}
}
