package importer

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// A RAR set lists only its volumes, its info files and a sample: the size a
// Release holds is measured after unpacking, not from the sample alone.
func TestClaimMismatchCountsUnpackedVideo(t *testing.T) {
	fr := fakeRules{on: true, floors: true}
	claimed := int64(2 << 30)
	cands := []Candidate{
		{Path: "South.Park.S29E03-CAKES/south.park.s29e03-cakes.rar", Size: 100 << 20},
		{Path: "South.Park.S29E03-CAKES/south.park.s29e03-cakes.r00", Size: 100 << 20},
		{Path: "South.Park.S29E03-CAKES/south.park.s29e03-cakes.nfo", Size: 4 << 10},
		{Path: "South.Park.S29E03-CAKES/Sample/south.park.s29e03-cakes-sample.mkv", Size: 39 << 20},
		// What unpackRars adds to the candidates.
		{Path: "South.Park.S29E03-CAKES/south.park.s29e03-cakes.mkv", Size: 1900 << 20},
	}
	if why := fr.claimMismatch(claimed, cands); why != "" {
		t.Fatalf("unpacked RAR set refused: %s", why)
	}
	// Without the unpacked file the sample alone still reads as a fake.
	if why := fr.claimMismatch(claimed, cands[:4]); !strings.Contains(why, "likely a fake") {
		t.Fatalf("sample alone: %q", why)
	}
}

// strandedWorld is a show download whose import was refused, with a second
// download (the replacement grab) that the Slot now waits for.
func strandedWorld(t *testing.T) (world, int64) {
	t.Helper()
	w := setup(t)
	var newID int64
	if err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE titles SET kind = 'show'"); err != nil {
			return err
		}
		var dlr int64
		if err := tx.QueryRow("SELECT downloader_id FROM downloads WHERE id = ?", w.dlID).Scan(&dlr); err != nil {
			return err
		}
		repl := &domain.Download{DownloaderID: dlr, SlotIDs: []int64{w.slotID}, ExternalID: "def", Progress: 0.5, State: "downloading",
			FilesSeen: []domain.SeenFile{}, SavePath: w.dl, StartedBy: domain.System}
		if _, err := database.Insert(tx, "downloads", repl, domain.System); err != nil {
			return err
		}
		newID = repl.ID
		if _, err := tx.Exec(`INSERT INTO imports (download_id, state, plan, from_path, kind, mode, created_at, created_by, updated_at, updated_by)
			VALUES (?, 'rejected', '[]', ?, 'episode', 'auto', 'x', '{}', 'x', '{}')`, w.dlID, w.dl); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE slots SET state = 'downloading', active_download_id = ? WHERE id = ?", newID, w.slotID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return w, newID
}

// The refused download is not queued again once a replacement holds the Slot.
func TestStrandedSkipsRefusedDownloadReplaced(t *testing.T) {
	w, _ := strandedWorld(t)
	var queued []int64
	n, err := (&Importer{Store: w.s}).RequeueStranded(context.Background(), func(_ context.Context, id int64) { queued = append(queued, id) })
	if err != nil || n != 0 || len(queued) != 0 {
		t.Fatalf("queued %v (%d, %v); the Slot waits for the replacement", queued, n, err)
	}
}

// Importing the refused download again leaves the Slot to the replacement.
func TestRefusedDownloadDoesNotTakeTheSlotBack(t *testing.T) {
	w, newID := strandedWorld(t)
	_, err := (&Importer{Store: w.s}).Import(context.Background(), w.dlID, false, domain.System)
	if err == nil || !strings.Contains(err.Error(), "waits for it") {
		t.Fatalf("err = %v, want the nothing-waits refusal", err)
	}
	var state string
	var active sql.NullInt64
	if err := w.s.Read().QueryRow("SELECT state, active_download_id FROM slots WHERE id = ?", w.slotID).Scan(&state, &active); err != nil {
		t.Fatal(err)
	}
	if state != "downloading" || !active.Valid || active.Int64 != newID {
		t.Fatalf("slot %s active %v, want downloading for %d", state, active, newID)
	}
}

// A season pack fetched a few episodes at a time is measured from every file
// the client lists, so the first finished episodes are not a fake.
func TestClaimSetPackWindowCountsWholePack(t *testing.T) {
	fr := fakeRules{on: true, floors: true}
	var seen []domain.SeenFile
	for n := 1; n <= 20; n++ {
		seen = append(seen, domain.SeenFile{Name: fmt.Sprintf("Show.S01/Show.S01E%02d.mkv", n), Size: 1 << 30})
	}
	claimed := int64(20 << 30)
	// The first episode of the window is done; one of twenty is under a tenth.
	ready := []Candidate{{Path: seen[0].Name, Size: seen[0].Size}}
	d := domain.Download{FilesSeen: seen, PackFiles: []domain.PackFile{{Name: seen[0].Name, On: true}}}
	if why := fr.claimMismatch(claimed, claimSet(d, ready)); why != "" {
		t.Fatalf("pack window refused: %s", why)
	}
	// A normal download is measured from its files to import.
	d.PackFiles = nil
	if why := fr.claimMismatch(claimed, claimSet(d, ready)); !strings.Contains(why, "likely a fake") {
		t.Fatalf("normal download: %q", why)
	}
}
