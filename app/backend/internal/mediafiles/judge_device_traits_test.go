package mediafiles

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// Device traits: favour or avoid what players can't show directly.
func TestDeviceTraits(t *testing.T) {
	cases := []struct {
		name, trait string
		want        bool
	}{
		{"Copper.Valley.2021.1080p.WEB-DL.DDP5.1.H.264-GRP", "h264", true},
		{"Copper.Valley.2021.1080p.BluRay.x264-GRP", "h264", true},
		{"Copper.Valley.2021.1080p.WEB-DL.DDP5.1.x265-GRP", "h264", false},
		{"Copper.Valley.2021.1080p.BluRay.DTS-HD.MA.5.1.x264-GRP", "dts_audio", true},
		{"Copper.Valley.2021.2160p.BluRay.DTS-X.7.1.x265-GRP", "dts_audio", true},
		{"Copper.Valley.2021.1080p.BluRay.DTS.5.1.x264-GRP", "dts_audio", true},
		{"Copper.Valley.2021.1080p.WEB-DL.DDP5.1.H.264-GRP", "dts_audio", false},
		{"Copper.Valley.2021.2160p.BluRay.TrueHD.7.1.Atmos.x265-GRP", "truehd_audio", true},
		{"Copper.Valley.2021.1080p.WEB-DL.AAC2.0.H.264-GRP", "truehd_audio", false},
		{"Copper.Valley.2021.2160p.WEB-DL.DV.DDP5.1.H.265-GRP", "dv_no_fallback", true},
		{"Copper.Valley.2021.2160p.WEB-DL.DV.HDR10.DDP5.1.H.265-GRP", "dv_no_fallback", false},
		{"Copper.Valley.2021.2160p.WEB-DL.DV.HDR10+.DDP5.1.H.265-GRP", "dv_no_fallback", false},
		{"Copper.Valley.2021.2160p.WEB-DL.HDR10.DDP5.1.H.265-GRP", "dv_no_fallback", false},
	}
	for _, c := range cases {
		got, _ := traitMatches(c.trait, releasename.Parse(c.name, releasename.Hints{}))
		if got != c.want {
			t.Errorf("%s on %q = %v, want %v", c.trait, c.name, got, c.want)
		}
	}
	for _, tr := range []string{"h264", "dts_audio", "truehd_audio", "dv_no_fallback"} {
		if !videoTraits[tr] {
			t.Errorf("%s missing from the video traits Pick rules accept", tr)
		}
	}
}
