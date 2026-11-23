package mediafiles

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// A name with a size but no source gets the TV step for that size, as
// Sonarr and Radarr do. No size and no source stays unknown.
func TestStepForUnknownSource(t *testing.T) {
	for name, want := range map[string]string{
		"Movie.2020.1080p.x264-GRP":       "1080p_tv",
		"Movie.2020.720p.x265-GRP":        "720p_tv",
		"Movie.2020.2160p.HEVC-GRP":       "2160p_web",
		"Movie.2020.480p.x264-GRP":        "sd_tv",
		"Show.S01E02.1080p.x265-GRP":      "1080p_tv",
		"Movie.2020.x264-GRP":             "",
		"Movie.2020.1080p.WEB-DL.x264-GR": "1080p_web",
	} {
		if got := StepFor(releasename.Parse(name, releasename.Hints{})); got != want {
			t.Errorf("%s: step %q, want %q", name, got, want)
		}
	}
}
