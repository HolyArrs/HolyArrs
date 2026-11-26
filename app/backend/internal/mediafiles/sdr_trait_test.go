package mediafiles

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// sdr_4k matches a 2160p name with no HDR word, and nothing else (#610).
func TestSDR4KTrait(t *testing.T) {
	cases := map[string]bool{
		"The.Lantern.Keepers.2021.2160p.WEB-DL.DDP5.1.H.265-QUILL":    true,
		"The.Lantern.Keepers.2021.2160p.WEB-DL.SDR.H.265-QUILL":       true,
		"The.Lantern.Keepers.2021.2160p.WEB-DL.HDR.H.265-QUILL":       false,
		"The.Lantern.Keepers.2021.2160p.WEB-DL.DV.H.265-QUILL":        false,
		"The.Lantern.Keepers.2021.2160p.BluRay.HLG.x265-QUILL":        false,
		"The.Lantern.Keepers.2021.2160p.WEB-DL.HDR10Plus.H.265-QUILL": false,
		"The.Lantern.Keepers.2021.1080p.WEB-DL.DDP5.1.H.264-QUILL":    false,
	}
	for name, want := range cases {
		if got, _ := traitMatches("sdr_4k", releasename.Parse(name, releasename.Hints{})); got != want {
			t.Errorf("%s: sdr_4k = %v, want %v", name, got, want)
		}
	}
}
