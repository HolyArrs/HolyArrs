package shows

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// A show folder named like a release (season ranges, quality, a group tag,
// "Complete") is read down to the show's name before the search.
func TestShowFolderNameReleaseDetails(t *testing.T) {
	for in, want := range map[string]struct {
		name string
		year int
	}{
		"Breaking Bad S01-S05 1080p BluRay x264-GROUP": {"Breaking Bad", 0},
		"[Group] Breaking Bad Complete Series 1080p":   {"Breaking Bad", 0},
		"Breaking Bad (2008) S01-S05 1080p":            {"Breaking Bad", 2008},
		"Breaking Bad (2008) [1080p]":                  {"Breaking Bad", 2008},
		"Breaking Bad 2008 Complete 720p":              {"Breaking Bad", 2008},
		"Breaking Bad Season 1-5":                      {"Breaking Bad", 0},
		"Breaking Bad (2008)":                          {"Breaking Bad", 2008},
		"Breaking Bad":                                 {"Breaking Bad", 0},
		"9-1-1":                                        {"9-1-1", 0},
	} {
		if n, y := showFolderName(in); n != want.name || y != want.year {
			t.Errorf("%q -> %q %d, want %q %d", in, n, y, want.name, want.year)
		}
	}
}

// A show answer is scored by its best name, its other names included.
func TestShowScoreOtherNames(t *testing.T) {
	sh := metadata.Show{Name: "Shingeki no Kyojin", Year: 2013, OtherNames: []string{"Attack on Titan"}}
	if got := showScore("Attack on Titan", 2013, sh); got != 1 {
		t.Fatalf("score %v, want 1", got)
	}
	if got := showScore("Something Else", 2013, metadata.Show{Name: "Other Show", Year: 2013}); got >= 0.5 {
		t.Fatalf("unrelated score %v", got)
	}
}
