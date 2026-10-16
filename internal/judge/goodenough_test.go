package judge

import (
	"testing"

	"github.com/CoderLuii/holyarrs/internal/domain"
)

func TestReachesGoodEnough(t *testing.T) {
	r := domain.GoodEveryday()
	cases := map[string]bool{
		"Dune.Part.Two.2024.1080p.WEB-DL.DDP5.1.H.264-GRP": true,
		"Dune.Part.Two.2024.720p.WEB-DL.DDP5.1.H.264-GRP":  false,
		"Dune.Part.Two.2024.1080p.BluRay.x264-GRP":         true,
		"no quality at all":                                false,
	}
	for name, want := range cases {
		if got := ReachesGoodEnough(name, r); got != want {
			t.Errorf("%s: %v, want %v", name, got, want)
		}
	}
	r.GoodEnoughAt = "2160p_web"
	if ReachesGoodEnough("Dune.Part.Two.2024.1080p.WEB-DL.DDP5.1.H.264-GRP", r) {
		t.Error("1080p is not good enough at 2160p_web")
	}
}
