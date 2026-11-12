package downloads

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// A grab with no Downloader for the Release's protocol records no Download,
// never says sent, and puts the Slot back to wanted at once with the reason.
func TestGrabWithNoDownloaderForTheProtocolIsNeverSent(t *testing.T) {
	w, m := setup(t)
	ctx := context.Background()
	if !m.HasDownloader(ctx, "torrent") {
		t.Fatal("the lab qBittorrent should count")
	}
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE downloaders SET removed = 1`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if m.HasDownloader(ctx, "torrent") || m.HasDownloader(ctx, "usenet") {
		t.Fatal("a removed Downloader still counts")
	}
	_, err := m.Grab(ctx, w.slotID, w.relID, domain.System)
	var nd *NoDownloaderError
	if !errors.As(err, &nd) || err.Error() != "no torrent Downloader is set up" {
		t.Fatalf("Grab: %v", err)
	}
	run := &jobs.Run{Job: domain.Job{Subject: &domain.Subject{Type: "slot", ID: w.slotID}, Params: map[string]any{"release_id": float64(w.relID)}, Attempt: 1, MaxAttempts: 5}}
	if err := m.HandleGrab(ctx, run); err != nil {
		t.Fatalf("the Job should end, not retry: %v", err)
	}
	var sl mediafiles.Slot
	if err := database.Get(ctx, w.s.Read(), "slots", w.slotID, &sl); err != nil {
		t.Fatal(err)
	}
	if sl.State != mediafiles.SlotWanted || sl.ChosenReleaseID != nil || sl.LastReason == nil || *sl.LastReason != "not sent: no torrent Downloader is set up" {
		t.Fatalf("slot %s, chosen %v, reason %v", sl.State, sl.ChosenReleaseID, sl.LastReason)
	}
	var n int
	_ = w.s.Read().QueryRow("SELECT count(*) FROM downloads").Scan(&n)
	if n != 0 || len(w.fake.added) != 0 {
		t.Fatalf("something was sent: %d downloads", n)
	}
}
