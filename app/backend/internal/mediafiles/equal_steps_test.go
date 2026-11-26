package mediafiles

import (
	"slices"
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

// Judging's side of the import check: with 1080p WEB and BluRay as one step, a WEB
// replaces a BluRay below its smallest size, and one big enough stays unless
// the rules' own order counts the WEB better (an old app's: more points);
// without the group the WEB is lower. The import check ranks the same way (importer TestLieOfRanksEqualSteps).
func TestEqualStepsReplaceUnderMin(t *testing.T) {
	const bluRay = "The.Lantern.Keepers.2021.1080p.BluRay.x264-YTS"
	const web = "The.Lantern.Keepers.2021.1080p.AMZN.WEB-DL.DDP5.1.H.264-GRP"
	group := GoodEveryday()
	group.ReplaceUnderMin = true
	group.EqualSteps = [][]string{{"1080p_web", "1080p_bluray"}}
	oldApp := group
	oldApp.Synced = &SyncedProfile{From: "import", App: "radarr"}
	noPoints := oldApp
	noPoints.Traits = nil
	plain := GoodEveryday()
	plain.ReplaceUnderMin = true
	cases := []struct {
		name    string
		rules   PickRulesBody
		curSize int64
		want    string
	}{
		{"group, small bluray", group, 300 * mb, "better.under_min"},
		{"group, bluray big enough", group, 4 * gb, "better.good_enough_already"},
		{"old app group, small bluray", oldApp, 300 * mb, "better.under_min"},
		{"old app group, bluray big enough, more points", oldApp, 4 * gb, "better.points"},
		{"old app group, bluray big enough, no more points", noPoints, 4 * gb, "better.not_better"},
		{"no group, small bluray", plain, 300 * mb, "better.lower_step"},
	}
	for _, c := range cases {
		cur := CurrentOf(bluRay, c.curSize, time.Now().Add(-48*time.Hour), c.rules)
		rel := release(row{Name: web})
		v := Judge(rel, setFor(t, rel, &cur), c.rules)
		if got := append(codes(v, "pass"), codes(v, "fail")...); !slices.Contains(got, c.want) {
			t.Errorf("%s: want %s, got %v", c.name, c.want, got)
		}
	}
}
