package releasename

import "testing"

// Upscale tags AI upscalers and regraders put in their names.
func TestUpscaleTags(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"The.Lantern.Keepers.2021.2160p.WEB-DL.AIUS.HEVC-QUILL", true},
		{"The.Lantern.Keepers.2021.2160p.AI.HEVC-QUILL", true},
		{"The.Lantern.Keepers.2021.2160p.AI.Upscaled.HEVC-QUILL", true},
		{"The.Lantern.Keepers.2021.2160p.AI-Enhanced.x265-QUILL", true},
		{"The.Lantern.Keepers.2021.2160p.UpRez.HDR.x265-QUILL", true},
		{"The.Lantern.Keepers.2021.2160p.UPSUHD.x265-QUILL", true},
		{"The.Lantern.Keepers.2021.2160p.Regraded.HDR.x265-QUILL", true},
		{"The.Lantern.Keepers.2021.2160p.BluRay.x265.The.Upscaler", true},
		{"The.Lantern.Keepers.2021.1080p.WEB-DL.x264.UPSCALED-GRAVELY", true},
		// not upscales
		{"AI.Rising.2018.2160p.WEB-DL.HEVC-QUILL", false},
		{"A.I.Artificial.Intelligence.2001.2160p.BluRay.x265-QUILL", false},
		{"The.Lantern.Keepers.2021.2160p.AI.x264-QUILL", false},
		{"The.Lantern.Keepers.2021.2160p.UHD.BluRay.HDR.HEVC-QUILL", false},
	}
	for _, c := range cases {
		r := Parse(c.name, Hints{})
		if got := r.Has("upscaled"); got != c.want {
			t.Errorf("%s: upscaled = %v, want %v", c.name, got, c.want)
		}
	}
}
