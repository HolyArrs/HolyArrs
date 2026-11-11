package importer

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// Another app (Sonarr sharing the Downloader category) imported the pack and
// removed it before HolyArrs got to it: the episodes are let go, not parked
// for a person, and the Release is not turned down.
func TestShowImportFilesTakenByAnotherApp(t *testing.T) {
	w := showSetup(t, []int{1, 2}, ep(1), ep(2))
	if err := os.RemoveAll(filepath.Join(w.dl, packName)); err != nil {
		t.Fatal(err)
	}
	var again, rescans []int64
	im := &Importer{Store: w.s, OnRejected: func(_ context.Context, id int64) { again = append(again, id) },
		OnFilesGone: func(_ context.Context, titleID int64) { rescans = append(rescans, titleID) }}
	imp, err := im.Import(context.Background(), w.dlID, false, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if imp.State != "failed" {
		t.Fatalf("import state %s", imp.State)
	}
	for _, r := range imp.Result {
		if r.Outcome != "gone" || r.Error != FilesGoneReason {
			t.Fatalf("result %+v", r)
		}
	}
	for _, n := range []int{1, 2} {
		sl := w.slot(t, n)
		if sl.State != mediafiles.SlotWanted || sl.ActiveDownloadID != nil || sl.LastReason == nil || !strings.Contains(*sl.LastReason, FilesGoneReason) {
			t.Fatalf("E%d slot %+v", n, sl)
		}
	}
	if r := w.release(t); r.State == "turned_down" {
		t.Fatal("the Release was turned down")
	}
	if len(again) != 0 {
		t.Fatalf("searched again %v", again)
	}
	if len(rescans) != 1 {
		t.Fatalf("rescans queued %v", rescans)
	}
}

// A download folder HolyArrs cannot see at all (a wrong path map) is not
// taken for files another app removed: the person still looks.
func TestShowImportUnseenFolderStillNeedsADecision(t *testing.T) {
	w := showSetup(t, []int{1}, ep(1))
	if err := execSQL(w.s, "UPDATE downloads SET save_path = ? WHERE id = ?", filepath.Join(w.dl, "not-mounted", "deeper"), w.dlID); err != nil {
		t.Fatal(err)
	}
	gone := 0
	im := &Importer{Store: w.s, OnFilesGone: func(context.Context, int64) { gone++ }}
	if _, err := im.Import(context.Background(), w.dlID, false, domain.System); err != nil {
		t.Fatal(err)
	}
	if sl := w.slot(t, 1); sl.State != mediafiles.SlotNeedsADecision || gone != 0 {
		t.Fatalf("slot %+v, rescans %d", sl, gone)
	}
}

// An empty download mount (Docker made /downloads, the Downloader's save
// folder below it is not there) is not taken for files another app removed:
// the person still looks, and nothing is grabbed again.
func TestShowImportEmptyDownloadMountStillNeedsADecision(t *testing.T) {
	w := showSetup(t, []int{1, 2}, ep(1), ep(2))
	if err := execSQL(w.s, "UPDATE downloads SET save_path = ? WHERE id = ?", filepath.Join(t.TempDir(), "tv"), w.dlID); err != nil {
		t.Fatal(err)
	}
	gone := 0
	im := &Importer{Store: w.s, OnFilesGone: func(context.Context, int64) { gone++ }}
	if _, err := im.Import(context.Background(), w.dlID, false, domain.System); err != nil {
		t.Fatal(err)
	}
	for _, n := range []int{1, 2} {
		if sl := w.slot(t, n); sl.State != mediafiles.SlotNeedsADecision || sl.ActiveDownloadID == nil || gone != 0 {
			t.Fatalf("E%d slot %+v, rescans %d", n, sl, gone)
		}
	}
}

// A Usenet job folder is the download's own: when another app removed it
// and the category folder above is still there, the movie is let go.
func TestUsenetImportJobFolderTakenByAnotherApp(t *testing.T) {
	w := usenetWorld(t)
	ctx := context.Background()
	if err := execSQL(w.s, "UPDATE slots SET active_download_id = ? WHERE id = ?", w.dlID, w.slotID); err != nil {
		t.Fatal(err)
	}
	if err := execSQL(w.s, "UPDATE downloads SET save_path = ? WHERE id = ?", filepath.Join(w.dl, "Job"), w.dlID); err != nil {
		t.Fatal(err)
	}
	imp, err := (&Importer{Store: w.s, MayMove: mayMove}).Import(ctx, w.dlID, false, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	var sl mediafiles.Slot
	if err := database.Get(ctx, w.s.Read(), "slots", w.slotID, &sl); err != nil {
		t.Fatal(err)
	}
	if imp.State != "failed" || sl.State != mediafiles.SlotWanted || sl.ActiveDownloadID != nil {
		t.Fatalf("import %+v slot %+v", imp, sl)
	}
}

func TestMovieImportFilesTakenByAnotherApp(t *testing.T) {
	w := setup(t)
	ctx := context.Background()
	if err := execSQL(w.s, "UPDATE slots SET active_download_id = ? WHERE id = ?", w.dlID, w.slotID); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(w.dl, relName+".mkv")); err != nil {
		t.Fatal(err)
	}
	var rescans []int64
	im := &Importer{Store: w.s, OnFilesGone: func(_ context.Context, titleID int64) { rescans = append(rescans, titleID) }}
	imp, err := im.Import(ctx, w.dlID, false, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if imp.State != "failed" || len(imp.Result) != 1 || imp.Result[0].Outcome != "gone" || imp.Result[0].Error != FilesGoneReason {
		t.Fatalf("import %+v", imp)
	}
	var sl mediafiles.Slot
	if err := database.Get(ctx, w.s.Read(), "slots", w.slotID, &sl); err != nil {
		t.Fatal(err)
	}
	if sl.State != mediafiles.SlotWanted || sl.ActiveDownloadID != nil || sl.LastReason == nil || *sl.LastReason != FilesGoneReason {
		t.Fatalf("slot %+v", sl)
	}
	if len(rescans) != 1 {
		t.Fatalf("rescans queued %v", rescans)
	}
}

func execSQL(s *database.Store, q string, args ...any) error {
	return s.Write(context.Background(), func(tx *sql.Tx) error { _, err := tx.Exec(q, args...); return err })
}
