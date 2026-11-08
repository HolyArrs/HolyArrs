package alerts

import (
	"slices"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestSubtitleMoments(t *testing.T) {
	for _, k := range []string{"subtitle_saved", "subtitle_upgraded", "subtitle_failed", "subtitle_site_paused"} {
		if MomentOf(domain.Event{Kind: k}) != MomentSubtitle {
			t.Errorf("%s is not a subtitle moment", k)
		}
	}
	for _, k := range []string{"subtitle_none_good_enough", "subtitle_synced"} {
		if MomentOf(domain.Event{Kind: k}) != "" {
			t.Errorf("%s speaks", k)
		}
	}
	if !slices.Contains(Moments, MomentSubtitle) {
		t.Error("not listed")
	}
}
