package mediafiles

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// A 2160p file with no quality words in its name, moved over from Sonarr
// with its recorded quality, is a 2160p step, never SD from TV.
func TestStepFor2160pTVFile(t *testing.T) {
	name := "The Book of Boba Fett (2021) - 1x01 - Chapter 1 Stranger in a Strange Land.mkv"
	cases := []struct{ rec, res, want string }{
		{"HDTV-2160p", "2160p", "2160p_web"},
		{"SDTV", "2160p", "2160p_web"},
		{"", "2160p", "2160p_web"},
		{"HDTV-1080p", "1080p", "1080p_tv"},
		{"HDTV-720p", "720p", "720p_tv"},
		{"SDTV", "480p", "sd_tv"},
	}
	for _, c := range cases {
		mi := map[string]any{RecordedQualityKey: c.rec}
		if c.res != "" {
			mi["resolution"] = c.res
		}
		got := StepFor(releasename.Parse(JudgeName(name, mi), releasename.Hints{}))
		if got != c.want {
			t.Errorf("recorded %q, resolution %q: step %q, want %q", c.rec, c.res, got, c.want)
		}
	}
	for _, n := range []string{"Show.S01E01.2160p.HDTV.x265-GRP", "Show.S01E01.UHDTV.x265-GRP"} {
		if got := StepFor(releasename.Parse(n, releasename.Hints{})); got != "2160p_web" {
			t.Errorf("%s: step %q, want 2160p_web", n, got)
		}
	}
	// With the 4K preset, such a file reaches good enough.
	mi := map[string]any{RecordedQualityKey: "HDTV-2160p", "resolution": "2160p"}
	if !OwnedReachesGoodEnough(name, mi, FourK()) {
		t.Error("2160p TV file does not reach good enough on the 4K preset")
	}
}
