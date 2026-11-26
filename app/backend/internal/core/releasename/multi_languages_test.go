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
		r := Parse(c.name, Hints{MultiLanguages: c.multi})
		if !slices.Equal(r.Languages, c.want) {
			t.Errorf("%s with %v: languages %v, want %v", c.name, c.multi, r.Languages, c.want)
		}
		// Only a MULTi name read with the Source's list has known languages.
		if known := r.Has("multi") && len(c.multi) > 0; r.Has("multi_known") != known {
			t.Errorf("%s with %v: multi_known %v, want %v", c.name, c.multi, r.Has("multi_known"), known)
		}
	}
}
