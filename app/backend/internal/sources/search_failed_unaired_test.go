package sources

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// A person's search for something whose air time is still ahead stays a
// not-out search after a failed try moved its Slot to failed: a
// release posted before the air time is a warning, not a refusal.
func TestHandle_FailedUnairedByHandStaysNotOut(t *testing.T) {
	lf := &liveFeed{}
	lf.set(feedOf(item(web1080, time.Now().Add(-48*time.Hour), 10, "")))
	f := seed(t, lf.url(t))
	exec(t, f, fmt.Sprintf("UPDATE items SET air_date = '%s'", time.Now().Add(6*time.Hour).UTC().Format(time.RFC3339)))
	exec(t, f, "UPDATE slots SET state = 'failed'")
	sr := &Searcher{Store: f.s}
	runKind(t, f, sr, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slotID}, map[string]any{"by_hand": true})
	rel := relByName(t, f, web1080)
	why := fmt.Sprint(rel.Verdict["reasons"])
	if slotState(t, f).State != mediafiles.SlotChosen || !strings.Contains(why, "gate_skipped") {
		t.Fatalf("slot %s, reasons %s", slotState(t, f).State, why)
	}
}
