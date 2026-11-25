package releasename

import "testing"

// A BRRip/BDRip with no size written is an SD rip, as Sonarr and Radarr
// read it (Bluray-480p); a written size always wins.
func TestIssue543RipWithoutSizeIsSD(t *testing.T) {
	cases := []struct {
		name string
		res  int
	}{
		{"Birdman.2014.FRENCH.BRRip.x264-GHZ", 480},
		{"Movie.2020.BDRip.x264-GRP", 480},
		{"Movie (2020) BDRip-AVC", 480},
		{"Movie.2020.BDLight.x264-GRP", 480},
		{"Movie.2020.HDDVDRip.x264-GRP", 480},
		{"Movie.2020.BRRip.XviD-GRP", 480},
		{"Movie.2020.576p.BRRip.x264-GRP", 576},
		{"Movie.2020.720p.BRRip.x264-GRP", 720},
		{"Movie.2020.1080p.BDRip.x264-GRP", 1080},
		{"Movie.2020.BDRip.1080p.x264-GRP", 1080},
		{"Movie.2020.2160p.BDRip.x265-GRP", 2160},
		{"Movie.2020.UHDBDRip.x265-GRP", 2160},
		{"Movie.2020.BluRay.x264-GRP", 1080},
		{"Movie.2020.1080p.BluRay.x264-GRP", 1080},
	}
	for _, c := range cases {
		p := Parse(c.name, Hints{})
		if p.Source.Value != "bluray" || p.Resolution.Value != c.res {
			t.Errorf("%s: got %s %dp, want bluray %dp", c.name, p.Source.Value, p.Resolution.Value, c.res)
		}
	}
}
