package mediafiles

import (
	"slices"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// Plays on my devices: each format a ticked device can't play directly
// costs points once; nothing is ever refused for it.
func TestDeviceMisses(t *testing.T) {
	cases := []struct {
		name    string
		devices []string
		want    []string // missed features
	}{
		{"The.Last.of.Us.S01E01.1080p.WEB.H264-CAKES", []string{"web_browser"}, nil},
		{"The.Last.of.Us.S01E01.2160p.MAX.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX", []string{"apple_tv"}, nil},
		{"The.Last.of.Us.S01E01.2160p.MAX.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX", []string{"web_browser"}, []string{"hevc", "hdr", "eac3"}},
		{"Oppenheimer.2023.1080p.BluRay.DTS-HD.MA.5.1.x264-SPHD", []string{"fire_tv"}, []string{"dts"}},
		{"Dune.Part.Two.2024.2160p.UHD.BluRay.REMUX.DV.HDR10.HEVC.TrueHD.7.1.Atmos-FGT", []string{"roku"}, []string{"truehd"}},
		{"Severance.S02E01.2160p.ATVP.WEB-DL.DDP5.1.DV.H.265-NTb", []string{"chromecast"}, nil},
		{"Severance.S02E01.2160p.ATVP.WEB-DL.DDP5.1.DV.H.265-NTb", []string{"web_browser"}, []string{"hevc", "hdr", "eac3"}},
		{"Andor.S02E01.1080p.DSNP.WEB-DL.DDP5.1.AV1-DUAL", []string{"apple_tv", "fire_tv"}, []string{"av1"}},
		{"Andor.S02E01.1080p.DSNP.WEB-DL.DDP5.1.AV1-DUAL", []string{"fire_tv"}, nil},
		{"Oppenheimer.2023.1080p.BluRay.DTS-HD.MA.5.1.x264-SPHD", nil, nil},
	}
	for _, c := range cases {
		p := releasename.Parse(c.name, releasename.Hints{})
		var got []string
		for _, m := range deviceMisses(p, c.devices) {
			got = append(got, m.Feature)
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("%q on %v: misses %v, want %v", c.name, c.devices, got, c.want)
		}
	}
}

func TestDevicePointsNeverRefuse(t *testing.T) {
	rules := GoodEveryday()
	rules.Devices = []string{"fire_tv"}
	p := releasename.Parse("Oppenheimer.2023.1080p.BluRay.DTS-HD.MA.5.1.x264-SPHD", releasename.Hints{})
	plain, _ := scoreTraits(p, GoodEveryday())
	got, _ := scoreTraits(p, rules)
	if got != plain-DevicePenalty {
		t.Errorf("points %d, want %d", got, plain-DevicePenalty)
	}
	if err := CheckPickRules("x", []string{"movie"}, rules); err != nil {
		t.Errorf("fire_tv refused: %v", err)
	}
	rules.Devices = []string{"toaster"}
	if err := CheckPickRules("x", []string{"movie"}, rules); err == nil {
		t.Error("unknown device accepted")
	}
}
