package mediafiles

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// A BRRip/BDRip with no size lands on the SD step, so a profile without SD
// turns it away; a written size keeps its HD step.
func TestIssue543RipWithoutSizeStep(t *testing.T) {
	cases := map[string]string{
		"Birdman.2014.FRENCH.BRRip.x264-GHZ": "sd_dvd",
		"Movie.2020.BDRip.x264-GRP":          "sd_dvd",
		"Movie.2020.720p.BRRip.x264-GRP":     "720p_bluray",
		"Movie.2020.1080p.BDRip.x264-GRP":    "1080p_bluray",
		"Movie.2020.BluRay.x264-GRP":         "1080p_bluray",
	}
	for name, want := range cases {
		if got := StepFor(releasename.Parse(name, releasename.Hints{})); got != want {
			t.Errorf("%s: step %s, want %s", name, got, want)
		}
	}
}
