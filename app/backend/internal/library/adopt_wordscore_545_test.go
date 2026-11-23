package library

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Repeated words count once per appearance.
func TestWordScoreRepeatedWords(t *testing.T) {
	cases := []struct {
		folder, cand string
		want         float64
	}{
		{"Lisa Knapp", "Lisa Knapp", 1},
		{"Lisa Knapp", "Lisa Lisa", 0.5},
		{"Lisa Knapp", "LiSA", 0.5},
		{"Duran Smith", "Duran Duran", 0.5},
		{"Duran Duran", "Duran Duran", 1},
		{"Duran Duran", "Duran", 0.5},
		{"Talk Talk", "Talk Talk", 1},
		{"New York New York", "New York", 0.5},
	}
	for _, c := range cases {
		if got := wordScore(c.folder, c.cand); got != c.want {
			t.Errorf("wordScore(%q, %q) = %v, want %v", c.folder, c.cand, got, c.want)
		}
	}
}

// The reporter's folder: only Lisa Knapp fits it fully, and the runner-up
// is far enough behind that the close-call check does not hold it.
func TestArtistScoreRepeatedWords(t *testing.T) {
	cases := []struct {
		folder string
		a      metadata.Artist
		want   float64
	}{
		{"Lisa Knapp", metadata.Artist{Name: "Lisa Knapp", SortName: "Knapp, Lisa"}, 1},
		{"Lisa Knapp", metadata.Artist{Name: "Lisa Lisa", SortName: "Lisa Lisa"}, 0.5},
		{"Lisa Knapp", metadata.Artist{Name: "LiSA", SortName: "LiSA"}, 0.5},
		{"Duran Smith", metadata.Artist{Name: "Duran Duran", SortName: "Duran Duran"}, 0.5},
		{"Duran Duran", metadata.Artist{Name: "Duran Duran", SortName: "Duran Duran"}, 1},
	}
	for _, c := range cases {
		if got := artistScore(c.folder, c.a); got != c.want {
			t.Errorf("artistScore(%q, %q) = %v, want %v", c.folder, c.a.Name, got, c.want)
		}
	}
}
