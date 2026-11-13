package downloads

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// A grab Job for a choice that was replaced (another Release chosen, or the
// title deleted and its Slot id reused) ends at once and leaves the Slot alone.
func TestGrabForReplacedChoiceEndsAtOnce(t *testing.T) {
	w, m := setup(t)
	run := &jobs.Run{Job: domain.Job{Subject: &domain.Subject{Type: "slot", ID: w.slotID}, Params: map[string]any{"release_id": float64(w.relID + 99)}, Attempt: 1, MaxAttempts: 5}}
	if err := m.HandleGrab(context.Background(), run); err != nil {
		t.Fatalf("a grab for a replaced choice should end, not retry: %v", err)
	}
	if sl := w.slot(t); sl.State != mediafiles.SlotChosen || sl.ChosenReleaseID == nil || *sl.ChosenReleaseID != w.relID {
		t.Fatalf("the current choice was touched: %s %v", sl.State, sl.ChosenReleaseID)
	}
	if len(w.fake.added) != 0 {
		t.Fatalf("a replaced choice was sent")
	}
}

// Each choice gets its own grab Job: an old one still waiting for the same Slot
// must not swallow a new choice.
func TestGrabKeyNamesTheRelease(t *testing.T) {
	if GrabKey(7, 1) == GrabKey(7, 2) {
		t.Fatal("two choices for one Slot share a grab Job")
	}
}

// A chosen Slot with no grab Job and no send on record waits forever; the sweep
// puts it back to wanted with a reason.
func TestLostChoiceGoesBackToWanted(t *testing.T) {
	w, m := setup(t)
	ctx := context.Background()
	m.Now = func() time.Time { return time.Now().Add(time.Hour) }
	m.settleLostChoices(ctx)
	sl := w.slot(t)
	if sl.State != mediafiles.SlotWanted || sl.LastReason == nil || !strings.Contains(*sl.LastReason, "not sent") {
		t.Fatalf("slot %s (%v)", sl.State, sl.LastReason)
	}
}

func TestChoiceWithLiveGrabJobStays(t *testing.T) {
	w, m := setup(t)
	ctx := context.Background()
	m.Now = func() time.Time { return time.Now().Add(time.Hour) }
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		now := database.FormatTime(time.Now())
		_, err := tx.Exec(`INSERT INTO jobs(kind, state, subject, params, dedupe_key, attempt, max_attempts, run_after, asked_by, created_at, created_by, updated_at, updated_by)
			VALUES('grab', 'queued', json_object('type', 'slot', 'id', ?), json_object('release_id', ?), ?, 1, 5, ?, 'system', ?, 'system', ?, 'system')`,
			w.slotID, w.relID, GrabKey(w.slotID, w.relID), now, now, now)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	m.settleLostChoices(ctx)
	if sl := w.slot(t); sl.State != mediafiles.SlotChosen {
		t.Fatalf("a choice with a waiting grab Job was freed: %s", sl.State)
	}
}

func TestFreshChoiceStays(t *testing.T) {
	w, m := setup(t)
	m.Now = time.Now
	m.settleLostChoices(context.Background())
	if sl := w.slot(t); sl.State != mediafiles.SlotChosen {
		t.Fatalf("a fresh choice was freed: %s", sl.State)
	}
}
