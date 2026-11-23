package importer

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
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

func TestShowImportOverStaleMissingRow(t *testing.T) {
	w := showSetup(t, []int{1, 2}, ep(1), ep(2))
	ctx := context.Background()
	im := &Importer{Store: w.s}
	if imp, err := im.Import(ctx, w.dlID, false, domain.System); err != nil || imp.State != "done" {
		t.Fatalf("first import %v %+v", err, imp)
	}
	sl1 := w.slot(t, 1)
	if sl1.FileID == nil {
		t.Fatal("E01 has no file after first import")
	}
	staleID := *sl1.FileID
	origRel := w.file(t, staleID).RelativePath
	if err := os.Remove(filepath.Join(w.lib, filepath.FromSlash(origRel))); err != nil {
		t.Fatal(err)
	}
	otherRel := w.folder + "/Season 1/Copper Valley - S01E01 - 720p.mkv"
	write(t, filepath.Join(w.lib, filepath.FromSlash(otherRel)), 2048)
	otherName := "Copper.Valley.S01E01.720p.WEB.H264-HOLYARRS"
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		other := &mediafiles.File{SlotIDs: []int64{w.slots[1]}, VersionID: w.vID, RelativePath: otherRel, Size: 2048,
			Quality: mediafiles.Quality{Resolution: 720, Source: "web"}, Languages: []string{}, MediaInfo: map[string]any{},
			ReleaseName: &otherName, Origin: "imported", PlacedBy: "hardlink", Role: "main", State: "present"}
		otherID, err := database.Insert(tx, "files", other, domain.System)
		if err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE files SET state = 'missing' WHERE id = ?", staleID); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE slots SET file_id = ?, state = 'downloading', active_download_id = ? WHERE id = ?", otherID, w.dlID, w.slots[1]); err != nil {
			return err
		}
		_, err = tx.Exec("UPDATE downloads SET slot_ids = json_array(?), state = 'finished' WHERE id = ?", w.slots[1], w.dlID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	imp, err := im.Import(ctx, w.dlID, false, domain.System)
	if err != nil || imp.State != "done" {
		t.Fatalf("import over stale missing row: %v %+v", err, imp)
	}
	sl := w.slot(t, 1)
	if sl.State != mediafiles.SlotInLibrary && sl.State != mediafiles.SlotBetterWanted || sl.FileID == nil {
		t.Fatalf("E01 %s file %v", sl.State, sl.FileID)
	}
	if got := w.file(t, *sl.FileID).RelativePath; got != origRel {
		t.Fatalf("relative_path = %q, want %q", got, origRel)
	}
	var n int
	_ = w.s.Read().QueryRow("SELECT count(*) FROM files WHERE version_id = ? AND relative_path = ?", w.vID, origRel).Scan(&n)
	if n != 1 {
		t.Fatalf("%d rows at %q", n, origRel)
	}
}

func TestMovieImportOverStaleMissingRow(t *testing.T) {
	w := setup(t)
	ctx := context.Background()
	im := &Importer{Store: w.s}
	if imp, err := im.Import(ctx, w.dlID, false, domain.System); err != nil || imp.State != "done" {
		t.Fatalf("first import %v %+v", err, imp)
	}
	var sl mediafiles.Slot
	if err := database.Get(ctx, w.s.Read(), "slots", w.slotID, &sl); err != nil || sl.FileID == nil {
		t.Fatalf("slot after first import: %+v %v", sl, err)
	}
	staleID := *sl.FileID
	var stale mediafiles.File
	if err := database.Get(ctx, w.s.Read(), "files", staleID, &stale); err != nil {
		t.Fatal(err)
	}
	origRel := stale.RelativePath
	if err := os.Remove(filepath.Join(w.lib, filepath.FromSlash(origRel))); err != nil {
		t.Fatal(err)
	}
	otherRel := "Dune - Part Two (2024)/Dune - Part Two (2024) 720p WEB.mkv"
	write(t, filepath.Join(w.lib, filepath.FromSlash(otherRel)), 2048)
	otherName := "Dune.Part.Two.2024.720p.WEB.H264-HOLYARRS"
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		other := &mediafiles.File{SlotIDs: []int64{w.slotID}, VersionID: w.vID, RelativePath: otherRel, Size: 2048,
			Quality: mediafiles.Quality{Resolution: 720, Source: "web"}, Languages: []string{}, MediaInfo: map[string]any{},
			ReleaseName: &otherName, Origin: "imported", PlacedBy: "hardlink", Role: "main", State: "present"}
		otherID, err := database.Insert(tx, "files", other, domain.System)
		if err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE files SET state = 'missing' WHERE id = ?", staleID); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE slots SET file_id = ?, state = 'downloading', active_download_id = ? WHERE id = ?", otherID, w.dlID, w.slotID); err != nil {
			return err
		}
		_, err = tx.Exec("UPDATE downloads SET state = 'finished' WHERE id = ?", w.dlID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	imp, err := im.Import(ctx, w.dlID, false, domain.System)
	if err != nil || imp.State != "done" {
		t.Fatalf("import over stale missing row: %v %+v", err, imp)
	}
	if err := database.Get(ctx, w.s.Read(), "slots", w.slotID, &sl); err != nil || sl.FileID == nil {
		t.Fatalf("slot after second import: %+v %v", sl, err)
	}
	var got mediafiles.File
	if err := database.Get(ctx, w.s.Read(), "files", *sl.FileID, &got); err != nil {
		t.Fatal(err)
	}
	if got.RelativePath != origRel {
		t.Fatalf("relative_path = %q, want %q", got.RelativePath, origRel)
	}
	var n int
	_ = w.s.Read().QueryRow("SELECT count(*) FROM files WHERE version_id = ? AND relative_path = ?", w.vID, origRel).Scan(&n)
	if n != 1 {
		t.Fatalf("%d rows at %q", n, origRel)
	}
}
