package sources

import (
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// TestReuse_AfterFailedDownload: the search after a failed Download
// picks the next Release from the last search without asking the Source,
// searches again once the results are older than ReuseWindow, and searches
// again when nothing left in them passes.
func TestReuse_AfterFailedDownload(t *testing.T) {
	old := time.Now().Add(-2 * time.Hour)
	lf := &liveFeed{}
	lf.set(feedOf(item(web1080, old, 50, ""), item(br1080, old, 40, "")))
	f := seed(t, lf.url(t))
	reuse := map[string]any{reuseParam: true}
	slot := &domain.Subject{Type: "slot", ID: f.slotID}
	if j := runKind(t, f, &Searcher{Store: f.s}, jobs.KindSearchSlot, slot, map[string]any{"choose": false}); j.State != "done" {
		t.Fatalf("first search %s", j.State)
	}

	// Fresh results: the Source now answers nothing, yet the next Release is picked.
	lf.set(feedOf())
	exec(t, f, "UPDATE releases SET state = 'turned_down' WHERE name = ?", br1080)
	exec(t, f, "UPDATE slots SET state = 'failed' WHERE id = ?", f.slotID)
	j := runKind(t, f, &Searcher{Store: f.s}, jobs.KindSearchSlot, slot, reuse)
	if j.State != "done" || slotState(t, f).State != mediafiles.SlotChosen || relByName(t, f, web1080).State != "chosen" {
		t.Fatalf("reused Release not chosen: job %s, slot %s", j.State, slotState(t, f).State)
	}

	// Nothing left in the results: a full search finds a new Release.
	lf.set(feedOf(item(tv720, old, 30, "")))
	exec(t, f, "UPDATE releases SET state = 'turned_down' WHERE name = ?", web1080)
	exec(t, f, "UPDATE slots SET state = 'failed', chosen_release_id = NULL WHERE id = ?", f.slotID)
	j = runKind(t, f, &Searcher{Store: f.s}, jobs.KindSearchSlot, slot, reuse)
	if j.State != "done" || slotState(t, f).State != mediafiles.SlotChosen || relByName(t, f, tv720).State != "chosen" {
		t.Fatalf("full search did not follow: job %s, slot %s", j.State, slotState(t, f).State)
	}

	// Old results are not reused: the Source answers nothing and the Slot goes to a person.
	lf.set(feedOf())
	exec(t, f, "UPDATE releases SET state = 'judged', created_at = ? WHERE name = ?", database.FormatTime(time.Now().Add(-ReuseWindow-time.Minute)), tv720)
	exec(t, f, "UPDATE slots SET state = 'failed', chosen_release_id = NULL WHERE id = ?", f.slotID)
	j = runKind(t, f, &Searcher{Store: f.s}, jobs.KindSearchSlot, slot, reuse)
	if sl := slotState(t, f); j.State != "done" || sl.State != mediafiles.SlotNeedsADecision {
		t.Fatalf("old results reused: job %s, slot %s", j.State, sl.State)
	}
}
