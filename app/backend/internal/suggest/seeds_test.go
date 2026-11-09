package suggest

import (
	"math"
	"testing"
	"time"
)

// day0 is the fixed now used by the seed tests.
var day0 = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

// near reports whether two floats are equal within rounding.
func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

// TestSeeds_Weights checks own, other-member and Library weights and their decay.
func TestSeeds_Weights(t *testing.T) {
	lib := []LibTitle{
		{TMDBID: "1", Kind: "movie", Name: "New", Added: day0},
		{TMDBID: "2", Kind: "movie", Name: "Old", Added: day0.AddDate(-2, 0, 0)},
	}
	asks := []AskSeed{
		{TMDBID: "3", Kind: "show", Name: "Mine", Member: 7, At: day0},
		{TMDBID: "4", Kind: "movie", Name: "Theirs", Member: 8, At: day0},
	}
	seeds := Seeds(7, false, lib, asks, day0)
	w := map[string]float64{}
	for _, s := range seeds {
		w[s.Key()] = s.Weight
	}
	if !near(w["movie:1"], 1.0) || !near(w["show:3"], 2.0) || !near(w["movie:4"], 0.5) {
		t.Fatalf("weights %v", w)
	}
	if want := 0.5 * math.Exp(-730.0/120); !near(w["movie:2"], want) {
		t.Fatalf("old library title %v, want %v", w["movie:2"], want)
	}
	if seeds[0].Key() != "show:3" {
		t.Fatalf("seeds not strongest first: %v", seeds)
	}
}

// TestSeeds_KidStaysApart checks a Kid and an adult never seed each other.
func TestSeeds_KidStaysApart(t *testing.T) {
	asks := []AskSeed{{TMDBID: "1", Kind: "movie", Member: 1, At: day0}, {TMDBID: "2", Kind: "movie", Member: 2, At: day0, Kid: true}}
	adult := Seeds(1, false, nil, asks, day0)
	kid := Seeds(2, true, nil, asks, day0)
	if len(adult) != 1 || adult[0].TMDBID != "1" {
		t.Fatalf("an adult page took a Kid's ask: %v", adult)
	}
	if len(kid) != 1 || kid[0].TMDBID != "2" {
		t.Fatalf("a Kid page took an adult's ask: %v", kid)
	}
}

// TestProfileAffinity_Scores checks the genre, people and era parts of the score.
func TestProfileAffinity_Scores(t *testing.T) {
	seeds := []Seed{{TMDBID: "1", Kind: "movie", Weight: 2, Genres: []string{"Drama"}, People: []string{"Dee"}, Year: 2000}}
	p := NewProfile(seeds)
	full := p.Affinity(Item{Genres: []string{"Drama"}, People: []string{"Dee"}, Year: 2000})
	if !near(full, 1.0) {
		t.Fatalf("perfect match %v", full)
	}
	none := p.Affinity(Item{Genres: []string{"Comedy"}, Year: 1940})
	if !near(none, 0) {
		t.Fatalf("no match %v", none)
	}
	two := append(seeds, Seed{TMDBID: "3", Weight: 1, Genres: []string{"Comedy"}})
	neg := NewProfile(append(two, Seed{TMDBID: "2", Weight: -1, Genres: []string{"Drama"}}))
	if neg.Affinity(Item{Genres: []string{"Drama"}}) >= NewProfile(two).Affinity(Item{Genres: []string{"Drama"}}) {
		t.Fatal("a negative seed did not lower affinity")
	}
}

// TestTailCap_BySeedCount checks the tail size steps and the Kid limit.
func TestTailCap_BySeedCount(t *testing.T) {
	for _, c := range []struct {
		seeds int
		kid   bool
		want  int
	}{{0, false, 8}, {2, false, 8}, {3, false, 20}, {9, false, 20}, {10, false, 40}, {12, true, 8}} {
		if got := TailCap(c.seeds, c.kid); got != c.want {
			t.Errorf("TailCap(%d, %v) = %d, want %d", c.seeds, c.kid, got, c.want)
		}
	}
}
