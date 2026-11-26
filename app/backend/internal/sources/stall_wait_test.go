package sources

import (
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// A Release whose Downloads keep stalling (a dead VPN) is held for a wait
// that grows with each stall in a row: 1 day, then 2, 4, at most 7. It is
// never turned down for it.
func TestStalledReleaseWaitsLongerEachTime(t *testing.T) {
	old := time.Now().Add(-2 * time.Hour)
	lf := &liveFeed{}
	lf.set(feedOf(item(web1080, old, 50, "")))
	f := seed(t, lf.url(t))
	slot := &domain.Subject{Type: "slot", ID: f.slotID}
	if j := runKind(t, f, &Searcher{Store: f.s}, jobs.KindSearchSlot, slot, map[string]any{"choose": false}); j.State != "done" {
		t.Fatalf("first search %s", j.State)
	}
	rel := relByName(t, f, web1080)
	// stalled sets the streak the way a failed Download writes it.
	stalled := func(n int, ago time.Duration) {
		exec(t, f, "DELETE FROM release_stalls")
		if n > 0 {
			exec(t, f, "INSERT INTO release_stalls (name, count, last_at) VALUES (?, ?, ?)", web1080, n, database.FormatTime(time.Now().Add(-ago)))
		}
	}
	search := func() (mediafiles.Slot, domain.Release) {
		t.Helper()
		exec(t, f, "UPDATE slots SET state = 'wanted', chosen_release_id = NULL, held_release_id = NULL WHERE id = ?", f.slotID)
		exec(t, f, "UPDATE releases SET state = 'judged', hold_until = NULL WHERE id = ?", rel.ID)
		if j := runKind(t, f, &Searcher{Store: f.s}, jobs.KindSearchSlot, slot, nil); j.State != "done" {
			t.Fatalf("search %s", j.State)
		}
		return slotState(t, f), relByName(t, f, web1080)
	}
	near := func(got *time.Time, want time.Duration) bool {
		return got != nil && got.Sub(time.Now().Add(want)).Abs() < 5*time.Minute
	}

	stalled(1, time.Hour)
	if sl, r := search(); sl.State != mediafiles.SlotHeld || r.State != "held" || !near(r.HoldUntil, 23*time.Hour) {
		t.Fatalf("one stall: slot %s, release %s until %v", sl.State, r.State, r.HoldUntil)
	}
	stalled(2, 0)
	if _, r := search(); r.State != "held" || !near(r.HoldUntil, 48*time.Hour) {
		t.Fatalf("two stalls: release %s until %v", r.State, r.HoldUntil)
	}
	stalled(5, 0)
	if _, r := search(); r.State != "held" || !near(r.HoldUntil, 7*24*time.Hour) {
		t.Fatalf("five stalls: release %s until %v (cap 7 days)", r.State, r.HoldUntil)
	}
	// The wait is over: sent again.
	stalled(1, 25*time.Hour)
	if sl, _ := search(); sl.State != mediafiles.SlotChosen {
		t.Fatalf("wait over: slot %s", sl.State)
	}
}
