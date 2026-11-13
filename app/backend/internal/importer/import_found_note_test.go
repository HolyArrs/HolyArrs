package importer

import (
	"context"
	"database/sql"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// foundAgain turns the file a first import placed into a note a library scan
// left (no episode, origin found) and sends the same download again for the
// Slot, the way a re-grab of a torrent still in the Downloader takes over its
// old row.
func foundAgain(t *testing.T, ctx context.Context, s interface {
	Write(context.Context, func(*sql.Tx) error) error
}, slotID, dlID int64) {
	t.Helper()
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		for _, q := range []string{
			"UPDATE files SET slot_ids = '[]', origin = 'found' WHERE id = (SELECT file_id FROM slots WHERE id = ?1)",
			"UPDATE slots SET state = 'downloading', file_id = NULL, active_download_id = ?2 WHERE id = ?1",
			"UPDATE downloads SET slot_ids = json_array(?1), state = 'finished' WHERE id = ?2",
		} {
			if _, err := tx.Exec(q, slotID, dlID); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// A found note at the place an import puts its file gives way, as it does
// for an import by hand: the import used to fail on the note and leave the
// episode on Importing for good.
func TestShowImportOverAFoundNote(t *testing.T) {
	w := showSetup(t, []int{1, 2}, ep(1), ep(2))
	ctx := context.Background()
	im := &Importer{Store: w.s}
	if imp, err := im.Import(ctx, w.dlID, false, domain.System); err != nil || imp.State != "done" {
		t.Fatalf("first import %v", err)
	}
	foundAgain(t, ctx, w.s, w.slots[1], w.dlID)
	imp, err := im.Import(ctx, w.dlID, false, domain.System)
	if err != nil || imp.State != "done" {
		t.Fatalf("import over the note: %v %+v", err, imp)
	}
	sl := w.slot(t, 1)
	if sl.State != mediafiles.SlotInLibrary && sl.State != mediafiles.SlotBetterWanted || sl.FileID == nil {
		t.Fatalf("E01 %s file %v", sl.State, sl.FileID)
	}
	var n int
	_ = w.s.Read().QueryRow("SELECT count(*) FROM files WHERE version_id = ? AND relative_path = ?", w.vID, w.file(t, *sl.FileID).RelativePath).Scan(&n)
	if n != 1 {
		t.Fatalf("%d rows at the path", n)
	}
}

func TestMovieImportOverAFoundNote(t *testing.T) {
	w := setup(t)
	ctx := context.Background()
	im := &Importer{Store: w.s}
	if imp, err := im.Import(ctx, w.dlID, false, domain.System); err != nil || imp.State != "done" {
		t.Fatalf("first import %v", err)
	}
	foundAgain(t, ctx, w.s, w.slotID, w.dlID)
	imp, err := im.Import(ctx, w.dlID, false, domain.System)
	if err != nil || imp.State != "done" {
		t.Fatalf("import over the note: %v %+v", err, imp)
	}
}

// An import that stopped while placing (its episode still Importing) is
// queued again by the stranded sweep, and finishes.
func TestStrandedPlacingImportQueuedAgain(t *testing.T) {
	w := showSetup(t, []int{1, 2}, ep(1), ep(2))
	ctx := context.Background()
	im := &Importer{Store: w.s}
	if _, err := im.Import(ctx, w.dlID, false, domain.System); err != nil {
		t.Fatal(err)
	}
	// What the failed import left: the episode Importing, its newest import placing.
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		for _, q := range []string{
			"UPDATE files SET slot_ids = '[]', origin = 'found' WHERE id = (SELECT file_id FROM slots WHERE id = ?1)",
			"UPDATE slots SET state = 'importing', file_id = NULL, active_download_id = ?2 WHERE id = ?1",
			"UPDATE downloads SET slot_ids = json_array(?1) WHERE id = ?2",
			`INSERT INTO imports (created_at, created_by, updated_at, updated_by, download_id, from_path, kind, plan, mode, result, state)
				SELECT created_at, created_by, updated_at, updated_by, download_id, from_path, kind, plan, mode, '[]', 'placing' FROM imports WHERE download_id = ?2`,
		} {
			if _, err := tx.Exec(q, w.slots[1], w.dlID); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var queued []int64
	if n, err := im.RequeueStranded(ctx, func(_ context.Context, id int64) { queued = append(queued, id) }); err != nil || n != 1 || queued[0] != w.dlID {
		t.Fatalf("requeue %d %v %v", n, err, queued)
	}
	if imp, err := im.Import(ctx, w.dlID, false, domain.System); err != nil || imp.State != "done" {
		t.Fatalf("resumed import %v", err)
	}
	if sl := w.slot(t, 1); sl.State != mediafiles.SlotInLibrary && sl.State != mediafiles.SlotBetterWanted {
		t.Fatalf("E01 %s", sl.State)
	}
	queued = nil
	if n, _ := im.RequeueStranded(ctx, func(_ context.Context, id int64) { queued = append(queued, id) }); n != 0 {
		t.Fatalf("queued again after the import: %v", queued)
	}
}
