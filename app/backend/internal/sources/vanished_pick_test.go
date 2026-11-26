package sources

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// gonePick seeds a Slot that follows a download, with a second Release to pick by hand.
func gonePick(t *testing.T, slotState string) (fixture, int64) {
	t.Helper()
	f := seed(t, "http://127.0.0.1:1/api")
	old := addRelease(t, f, tv720, "chosen")
	id := addRelease(t, f, br1080, "judged")
	var dlID int64
	if err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		d := &domain.Downloader{Name: "q", Adapter: "qbittorrent", Protocol: "torrent", URL: "http://q", Category: "c", PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok"}
		if _, err := database.Insert(tx, "downloaders", d, domain.System); err != nil {
			return err
		}
		dl := &domain.Download{DownloaderID: d.ID, ReleaseID: &old, SlotIDs: []int64{f.slotID}, ExternalID: "e", State: "downloading", FilesSeen: []domain.SeenFile{}, StartedBy: domain.System}
		_, err := database.Insert(tx, "downloads", dl, domain.System)
		dlID = dl.ID
		return err
	}); err != nil {
		t.Fatal(err)
	}
	exec(t, f, "UPDATE slots SET state = ?, active_download_id = ?, chosen_release_id = ? WHERE id = ?", slotState, dlID, old, f.slotID)
	return f, id
}

// A download removed in the Downloader is settled when a Release is picked by
// hand, so the pick goes through instead of "it is downloading".
func TestChoose_DownloadGoneFromDownloader(t *testing.T) {
	ctx := context.Background()
	f, id := gonePick(t, "downloading")
	asked := 0
	s := &Searcher{Store: f.s, SettleGone: func(_ context.Context, slotID int64) bool {
		asked++
		exec(t, f, "UPDATE downloads SET state = 'vanished'")
		exec(t, f, "UPDATE slots SET state = 'needs_a_decision', last_reason = 'removed outside HolyArrs; not replaced (setting)' WHERE id = ?", slotID)
		return true
	}}
	if _, err := s.Choose(ctx, id, f.slotID, true, domain.System); err != nil || asked != 1 {
		t.Fatalf("choose: %v after %d asks", err, asked)
	}
	if sl := slotState(t, f); sl.State != "chosen" {
		t.Fatalf("slot %s", sl.State)
	}

	// A download the Downloader still has keeps the Slot busy.
	f2, id2 := gonePick(t, "downloading")
	s2 := &Searcher{Store: f2.s, SettleGone: func(context.Context, int64) bool { return false }}
	if _, err := s2.Choose(ctx, id2, f2.slotID, true, domain.System); !errors.Is(err, ErrBusy) {
		t.Fatalf("still downloading: %v", err)
	}
}

// The Slot a removed download left waiting on a decision takes a pick by hand.
func TestChoose_AfterDownloadVanished(t *testing.T) {
	f, id := gonePick(t, "needs_a_decision")
	exec(t, f, "UPDATE downloads SET state = 'vanished'")
	if _, err := (&Searcher{Store: f.s}).Choose(context.Background(), id, f.slotID, true, domain.System); err != nil {
		t.Fatalf("choose: %v", err)
	}
}
