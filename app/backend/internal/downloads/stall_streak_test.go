package downloads

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// Each stall in a row of the same Release is counted; from the second one
// History says how long it now waits, and the Release is never turned down.
// A finished Download of it clears the count.
func TestStallsInARowAreCountedAndClearedOnSuccess(t *testing.T) {
	ctx := context.Background()
	w, m := setup(t)
	letChange(t, m)
	id := grabbed(t, w, m)
	rel := w.release(t)
	reason := "no progress for 24 hours"
	stall := func() {
		t.Helper()
		d := w.download(t, id)
		next := d
		next.State, next.StateReason = "failed", &reason
		if !m.failed(ctx, w.downloader(t, d.DownloaderID), d, next, m.settings(ctx), domain.System) {
			t.Fatal("not failed")
		}
	}
	count := func() int {
		got, err := mediafiles.StallStreaks(ctx, w.s.Read(), []string{rel.Name})
		if err != nil {
			t.Fatal(err)
		}
		return got[rel.Name].Count
	}
	again := func() {
		t.Helper()
		if err := w.s.Write(ctx, func(tx *sql.Tx) error {
			if _, err := tx.Exec("UPDATE downloads SET state = 'downloading', state_reason = NULL WHERE id = ?", id); err != nil {
				return err
			}
			_, err := tx.Exec("UPDATE slots SET state = 'downloading', active_download_id = ? WHERE id = ?", id, w.slotID)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	summary := func() (string, string) {
		var s, data string
		_ = w.s.ReadEvents().QueryRow("SELECT summary, data FROM events WHERE kind = 'download_failed' AND download_id = ? ORDER BY id DESC LIMIT 1", id).Scan(&s, &data)
		return s, data
	}

	stall()
	if s, _ := summary(); count() != 1 || !strings.HasPrefix(s, "Download stalled: ") {
		t.Fatalf("first stall: count %d, %q", count(), s)
	}
	again()
	stall()
	s, data := summary()
	if count() != 2 || !strings.HasPrefix(s, "Download stalled 2 times in a row: "+reason) || !strings.Contains(s, "waits 2 days") || !strings.Contains(data, `"stall_count":2`) {
		t.Fatalf("second stall: count %d, %q %s", count(), s, data)
	}
	if r := w.release(t); r.State == "turned_down" {
		t.Fatal("a stall turned the Release down")
	}
	again()
	d := w.download(t, id)
	d.State = "finished"
	m.stateMoved(ctx, "downloading", &d)
	if count() != 0 {
		t.Fatalf("finished: count %d", count())
	}
}
