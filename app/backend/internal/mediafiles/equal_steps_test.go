package mediafiles

import (
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// An old app's merged group: inside it points decide, both when
// ranking and when upgrading; a quality above the group still wins on
// quality alone.
func TestEqualStepsRankAndUpgrade(t *testing.T) {
	b := GoodEveryday()
	for i := range b.Steps {
		b.Steps[i].Allowed = true
	}
	b.Traits = []TraitPoints{{Trait: "hevc", Points: 500}}
	b.GoodEnoughAt, b.UpgradeMinPoints, b.Order = "1080p_remux", 1, "quality_then_points"
	far := 10000
	b.GoodEnoughPoints = &far
	b.Synced = &SyncedProfile{From: "import", App: "radarr"}
	b.EqualSteps = [][]string{{"720p_web", "720p_bluray", "1080p_web", "1080p_bluray"}}

	gains := func(cur, rel string) bool {
		c := CurrentOf(cur, 0, time.Time{}, b)
		return Gains(ReleaseFacts{Name: rel, Parse: releasename.Parse(rel, releasename.Hints{})}, b, &c)
	}
	if !gains("Film.1999.1080p.BluRay.H264-X", "Film.1999.1080p.WEB-DL.H265-Y") {
		t.Error("in the group, more points on a lower step did not upgrade")
	}
	if gains("Film.1999.1080p.BluRay.H265-X", "Film.1999.1080p.WEB-DL.H264-Y") {
		t.Error("in the group, fewer points upgraded")
	}
	if !gains("Film.1999.720p.WEB-DL.H265-X", "Film.1999.1080p.BluRay.Remux.AVC-Y") {
		t.Error("a quality above the group did not upgrade")
	}
	if gains("Film.1999.1080p.BluRay.Remux.AVC-X", "Film.1999.720p.WEB-DL.H265-Y") {
		t.Error("the group beat a quality above it on points")
	}

	idx := func(s string) int { return stepIndex(b, s) }
	rels := make([]ReleaseFacts, 3)
	vs := []Verdict{
		{Passed: true, Step: &StepRef{Index: idx("1080p_bluray"), Name: "1080p_bluray"}, Points: 0},
		{Passed: true, Step: &StepRef{Index: idx("720p_web"), Name: "720p_web"}, Points: 500},
		{Passed: true, Step: &StepRef{Index: idx("1080p_remux"), Name: "1080p_remux"}, Points: 0},
	}
	Rank(rels, vs, b)
	if vs[2].Rank != 1 {
		t.Errorf("remux above the group not chosen: %+v", vs)
	}
	Rank(rels[:2], vs[:2], b)
	if vs[1].Rank != 1 {
		t.Errorf("in the group, points did not decide: %+v", vs[:2])
	}
}
