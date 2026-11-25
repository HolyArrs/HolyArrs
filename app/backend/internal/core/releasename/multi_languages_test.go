package releasename

import (
	"slices"
	"testing"
)

// A Source's MULTi list adds its languages to a MULTi name only; an empty
// list and a Multi-Subs name keep what the name says.
func TestSourceMultiLanguages(t *testing.T) {
	fr := []string{"fr", "en"}
	for _, c := range []struct {
		name  string
		multi []string
		want  []string
	}{
		{"Movie.Title.2019.MULTi.1080p.BluRay.x264-GRP", fr, []string{"fr", "en"}},
		{"Movie.Title.2019.MULTi.VFF.1080p.BluRay.x264-GRP", fr, []string{"fr", "en"}},
		{"Movie.Title.2019.MULTi.1080p.BluRay.x264-GRP", nil, nil},
		{"Movie.Title.2019.1080p.BluRay.Multi.Subs-GRP", nil, nil},
		{"Movie.Title.2019.1080p.BluRay.x264-GRP", fr, nil},
		{"Movie.Title.2019.German.1080p.BluRay.x264-GRP", fr, []string{"de"}},
	} {
		got := Parse(c.name, Hints{MultiLanguages: c.multi}).Languages
		if !slices.Equal(got, c.want) {
			t.Errorf("%s with %v: languages %v, want %v", c.name, c.multi, got, c.want)
		}
	}
}
