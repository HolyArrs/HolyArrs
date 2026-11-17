package downloads

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
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
	wantNotSentEvent(t, w, lostChoiceReason)
}

// A chosen Release whose file the Source no longer has never reaches the
// Downloader: History says it failed and why, nothing reads as sent, the
// Release is turned down and the Slot is wanted again at once.
func TestGrabWithAMissingReleaseFileSaysSo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }))
	t.Cleanup(srv.Close)
	fakeDNS(t, map[string]string{"fake-indexer": hostPort(t, srv)}, true)
	w, m := setup(t)
	ctx := context.Background()
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE releases SET link_ref = 'http://fake-indexer/gone.torrent', info_hash = NULL WHERE id = ?", w.relID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	searched := 0
	m.OnFailed = func(context.Context, int64) { searched++ }
	run := &jobs.Run{Job: domain.Job{Subject: &domain.Subject{Type: "slot", ID: w.slotID}, Params: map[string]any{"release_id": float64(w.relID)}, Attempt: 1, MaxAttempts: 5}}
	if err := m.HandleGrab(ctx, run); err != nil {
		t.Fatalf("a Release that can never be sent should end the grab: %v", err)
	}
	if len(w.fake.added) != 0 {
		t.Fatalf("qBittorrent got %+v", w.fake.added)
	}
	if sl := w.slot(t); sl.State != mediafiles.SlotWanted || searched != 1 {
		t.Fatalf("slot %s, searched again %d times", sl.State, searched)
	}
	var rel domain.Release
	if err := database.Get(ctx, w.s.Read(), "releases", w.relID, &rel); err != nil || rel.State != "turned_down" {
		t.Fatalf("release %s %v", rel.State, err)
	}
	wantNotSentEvent(t, w, "the Source has no such Release file")
}

// wantNotSentEvent checks History holds one failed line for the Slot with the
// reason, and no line that says the Release was sent.
func wantNotSentEvent(t *testing.T, w *world, reason string) {
	t.Helper()
	evs, _, err := w.s.Events(context.Background(), database.EventFilter{SlotIDs: []int64{w.slotID}})
	if err != nil {
		t.Fatal(err)
	}
	failed := 0
	for _, e := range evs {
		switch e.Kind {
		case "download_sent":
			t.Fatalf("History says sent: %s", e.Summary)
		case "download_failed":
			if r, _ := e.Data["reason"].(string); r != reason || e.TitleID == nil || e.Data["release"] == nil {
				t.Fatalf("failed line %+v", e)
			}
			failed++
		}
	}
	if failed != 1 {
		t.Fatalf("%d failed lines in %+v", failed, evs)
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
