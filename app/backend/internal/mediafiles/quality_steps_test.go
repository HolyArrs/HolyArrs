package mediafiles

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// Pre-release copies rank at the bottom of the ladder, and sizes between
// the bands fall to the band below.
func TestStepForNewSourcesAndSizes(t *testing.T) {
	for name, want := range map[string]string{
		"Movie.2020.TC.XviD-GRP":               "telesync",
		"Movie.2020.DVDSCR.XviD-GRP":           "telesync",
		"Movie.2020.R5.XviD-GRP":               "telesync",
		"Movie.2020.WORKPRINT.XviD-GRP":        "cam",
		"Movie.2020.1440p.WEB-DL.x265-GRP":     "1080p_web",
		"Movie.2020.4320p.WEB.HEVC-GRP":        "2160p_web",
		"Movie.2020.576p.BluRay.x264-GRP":      "sd_dvd",
		"Movie.2020.UHDBD.2160p.HEVC-GRP":      "2160p_bluray",
		"Movie.2020.1080p.WebHD.x264-GRP":      "1080p_web",
		"Movie.2020.Hybrid-Remux-2160p.HEVC-G": "2160p_remux",
	} {
		if got := StepFor(releasename.Parse(name, releasename.Hints{})); got != want {
			t.Errorf("%s: step %q, want %q", name, got, want)
		}
	}
	for _, name := range []string{"Movie.2020.TC.XviD-GRP", "Movie.2020.DVDSCR.XviD-GRP", "Movie.2020.R5.XviD-GRP"} {
		if ok, _ := traitMatches("telesync", releasename.Parse(name, releasename.Hints{})); !ok {
			t.Errorf("%s: not caught by the telesync trait, so the default never-list would let it through", name)
		}
	}
	if ok, _ := traitMatches("cam", releasename.Parse("Movie.2020.WORKPRINT.XviD-GRP", releasename.Hints{})); !ok {
		t.Error("workprint not caught by the cam trait")
	}
}
