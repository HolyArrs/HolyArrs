package source

import (
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
)

// Stage 4.0.3: after a failed download the search picks the next Release;
// the turned-down one is never picked again; with nothing left the Slot
// needs a decision.
func TestSearchAfterFailedDownload(t *testing.T) {
	lf := &liveFeed{}
	lf.set(feedOf(item(web1080, time.Now().Add(-2*time.Hour), 50, ""), item(br1080, time.Now().Add(-2*time.Hour), 40, "")))
	f := seed(t, lf.url(t))
	// The first pick failed: it is turned down, the Slot is failed.
	j := runKind(t, f, &Searcher{Store: f.s}, KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slotID}, map[string]any{"choose": false})
	if j.State != "done" {
		t.Fatalf("show search %s", j.State)
	}
	exec(t, f, "UPDATE releases SET state = 'turned_down' WHERE name = ?", br1080)
	exec(t, f, "UPDATE slots SET state = 'failed' WHERE id = ?", f.slotID)
	j = runKind(t, f, &Searcher{Store: f.s}, KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slotID}, nil)
	if j.State != "done" || slotState(t, f).State != domain.SlotChosen || relByName(t, f, web1080).State != "chosen" {
		t.Fatalf("next Release not chosen: job %s, slot %s, %s", j.State, slotState(t, f).State, relByName(t, f, web1080).State)
	}
	// That one fails too: nothing is left.
	exec(t, f, "UPDATE releases SET state = 'turned_down' WHERE name = ?", web1080)
	exec(t, f, "UPDATE slots SET state = 'failed' WHERE id = ?", f.slotID)
	j = runKind(t, f, &Searcher{Store: f.s}, KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slotID}, nil)
	sl := slotState(t, f)
	if j.State != "done" || sl.State != domain.SlotNeedsADecision || sl.LastReason == nil {
		t.Fatalf("want needs a decision, got %s (%v)", sl.State, sl.LastReason)
	}
}
