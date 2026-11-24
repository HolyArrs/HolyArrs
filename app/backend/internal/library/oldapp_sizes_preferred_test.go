package library

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// TestOldSizeLimitsPreferred: the old apps' preferred size comes over per
// step, the biggest of the qualities sharing a step, kept inside the band.
func TestOldSizeLimitsPreferred(t *testing.T) {
	const mib = int64(1 << 20)
	cases := []struct {
		name string
		rows []OldQualitySize
		want mediafiles.SizeLimit
	}{
		{"set", []OldQualitySize{{Quality: "WEBDL-1080p", Min: 10, Max: 100, Preferred: 40}},
			mediafiles.SizeLimit{Min: 10 * mib, Max: 100 * mib, Preferred: 40 * mib, PerMinute: true}},
		{"none", []OldQualitySize{{Quality: "WEBDL-1080p", Min: 10, Max: 100}},
			mediafiles.SizeLimit{Min: 10 * mib, Max: 100 * mib, PerMinute: true}},
		{"shared step takes the biggest", []OldQualitySize{{Quality: "WEBDL-1080p", Min: 10, Max: 100, Preferred: 40}, {Quality: "WEBRip-1080p", Min: 5, Max: 90, Preferred: 60}},
			mediafiles.SizeLimit{Min: 5 * mib, Max: 100 * mib, Preferred: 60 * mib, PerMinute: true}},
		{"above the largest is held to it", []OldQualitySize{{Quality: "WEBDL-1080p", Min: 10, Max: 100, Preferred: 400}},
			mediafiles.SizeLimit{Min: 10 * mib, Max: 100 * mib, Preferred: 100 * mib, PerMinute: true}},
	}
	for _, c := range cases {
		got := OldSizeLimits(c.rows)["1080p_web"]
		if got != c.want {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
}
