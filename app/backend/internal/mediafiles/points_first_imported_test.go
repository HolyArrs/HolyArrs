package mediafiles

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// TestPointsFirstHoldsForImportedRules: when imported rules are switched
// to points first, a higher step with fewer points does not replace the
// current file, and more points still do.
func TestPointsFirstHoldsForImportedRules(t *testing.T) {
	b := GoodEveryday()
	b.Synced = &SyncedProfile{From: "import", App: "radarr"}
	b.Order = "points_then_quality"
	b.GoodEnoughAt = b.Steps[len(b.Steps)-1].Quality
	b.UpgradeMinPoints = 1
	lo, hi := stepIndex(b, "1080p_web"), stepIndex(b, "1080p_bluray")
	if lo > hi {
		lo, hi = hi, lo
	}
	cur := &Current{Step: b.Steps[lo].Quality, Points: 100}
	p := releasename.Parse("Film.2020.1080p.WEB-DL-X", releasename.Hints{})
	var v Verdict
	v.better(ReleaseFacts{Parse: p}, b, cur, hi, 10, 0)
	if !v.failed() {
		t.Fatalf("a higher step with fewer points replaced the file: %+v", v.Reasons)
	}
	v = Verdict{}
	v.better(ReleaseFacts{Parse: p}, b, cur, hi, 150, 0)
	if v.failed() {
		t.Fatalf("more points did not count: %+v", v.Reasons)
	}
	b.Order = "quality_then_points"
	v = Verdict{}
	v.better(ReleaseFacts{Parse: p}, b, cur, hi, 10, 0)
	if v.failed() {
		t.Fatalf("quality first refused a higher step: %+v", v.Reasons)
	}
}
