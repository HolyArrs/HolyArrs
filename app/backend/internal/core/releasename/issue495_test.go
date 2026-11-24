package releasename

import (
	"slices"
	"testing"
)

// an episode range at the very end of a name ("S01E01-02") is two
// episodes; the "02" is not a release group.
func TestEpisodeRangeAtEnd(t *testing.T) {
	for name, want := range map[string][]int{
		"Show.S01E01-02":                 {1, 2},
		"Show S01E01-03":                 {1, 2, 3},
		"Show.S01E01E02-03":              {1, 2, 3},
		"Show.S01E01-02.1080p":           {1, 2},
		"Show.S01E01.1080p.WEB-DL-GROUP": {1},
		"Show.S01E01-GROUP":              {1},
		"The.Office.S03E01-E02.720p":     {1, 2},
		"Show.S01E01-E02":                {1, 2},
	} {
		p := Parse(name, Hints{})
		if p.Show == nil || !slices.Equal(p.Show.Episodes, want) {
			t.Errorf("%s: %+v, want %v", name, p.Show, want)
		}
	}
	if g := Parse("Show.S01E01-GROUP", Hints{}).Group.Value; g != "GROUP" {
		t.Errorf("group %q, want GROUP", g)
	}
	if g := Parse("The.Office.S03E01-E02.720p", Hints{}).Group.Value; g != "" {
		t.Errorf("S03E01-E02 group %q, want none", g)
	}
}
