package releasename

import (
	"slices"
	"testing"
)

// French release tags VF, VF2, VFI and VOF all mean French audio.
func TestFrenchVersionTags(t *testing.T) {
	for name, want := range map[string]struct {
		title string
		langs []string
	}{
		"Matrix.Reloaded.2003.MULTI.VFI.2160p.BluRay.4KLight.DV.HDR10.TrueHD.Atmos.7.1.x265-QTZ": {"Matrix Reloaded", []string{"fr"}},
		"Movie.Title.2019.VF.1080p.WEB-DL.x264-GRP":                                              {"Movie Title", []string{"fr"}},
		"Movie.Title.2019.MULTi.VF2.1080p.BluRay.x264-GRP":                                       {"Movie Title", []string{"fr"}},
		"Movie.Title.2019.VOF.1080p.WEB-DL.x264-GRP":                                             {"Movie Title", []string{"fr"}},
		"Show.Name.S01E02.VFI.1080p.WEB.H264-GRP":                                                {"Show Name", []string{"fr"}},
		"Show.Name.S01E02.MULTi.VFF.VF2.1080p.WEB.H264-GRP":                                      {"Show Name", []string{"fr"}},
		"Movie.Title.2019.1080p.WEBRip.x264.VOSTFR-GRP":                                          {"Movie Title", nil},
		"Vofka.2019.1080p.WEB-DL.x264-GRP":                                                       {"Vofka", nil},
	} {
		r := Parse(name, Hints{})
		if !slices.Equal(r.Languages, want.langs) {
			t.Errorf("%q: languages %v, want %v", name, r.Languages, want.langs)
		}
		if r.Title.Value != want.title {
			t.Errorf("%q: title %q, want %q", name, r.Title.Value, want.title)
		}
	}
}
