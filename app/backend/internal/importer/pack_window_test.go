package importer

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// packWindowDownload makes the lab pack one fetched a few episodes at a time:
// E01 and E02 switched on, E03 and E04 skipped. progress2 is E02's progress.
func packWindowDownload(t *testing.T, w showWorld, progress2 float64) {
	t.Helper()
	var seen []domain.SeenFile
	var pack []domain.PackFile
	for n := 1; n <= 4; n++ {
		f := domain.SeenFile{Name: packName + "/" + ep(n), Size: 2048, Progress: 1, Priority: 1}
		if n == 2 {
			f.Progress = progress2
		}
		if n > 2 {
			f.Progress, f.Priority = 0.3, 0 // skipped, a piece at the edge came anyway
		}
		seen = append(seen, f)
		pack = append(pack, domain.PackFile{I: n - 1, Name: f.Name, Slots: []int64{w.slots[n]}, Items: []int64{}, Eps: []int{n}, On: n <= 2})
	}
	a, _ := json.Marshal(seen)
	b, _ := json.Marshal(pack)
	if err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE downloads SET files_seen = ?, pack_files = ? WHERE id = ?", string(a), string(b), w.dlID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPackWindowImportPlacesOnlyFinishedFilesSwitchedOn(t *testing.T) {
	w := showSetup(t, []int{1, 2}, ep(1), ep(2), ep(3), ep(4))
	packWindowDownload(t, w, 1)
	var again []int64
	im := &Importer{Store: w.s, OnRejected: func(_ context.Context, id int64) { again = append(again, id) }}
	imp, err := im.Import(context.Background(), w.dlID, false, domain.System)
	if err != nil || imp.State != "done" {
		t.Fatalf("import %+v %v", imp, err)
	}
	for _, n := range []int{3, 4} {
		if sl := w.slot(t, n); sl.State != mediafiles.SlotWanted || sl.FileID != nil {
			t.Fatalf("skipped E%d: %+v", n, sl)
		}
	}
	if len(again) != 0 || w.release(t).State == "turned_down" {
		t.Fatalf("searched again %v, release %s", again, w.release(t).State)
	}
}

func TestPackWindowImportLeavesAnUnfinishedEpisodeDownloading(t *testing.T) {
	w := showSetup(t, []int{1, 2}, ep(1), ep(2), ep(3), ep(4))
	packWindowDownload(t, w, 0.4)
	im := &Importer{Store: w.s}
	imp, err := im.Import(context.Background(), w.dlID, false, domain.System)
	if err != nil || imp.State != "done" {
		t.Fatalf("import %+v %v", imp, err)
	}
	if sl := w.slot(t, 1); sl.FileID == nil {
		t.Fatalf("E01 not placed: %+v", sl)
	}
	if sl := w.slot(t, 2); sl.State != mediafiles.SlotDownloading || sl.FileID != nil {
		t.Fatalf("E02 at 0.4: %+v", sl)
	}
	var state string
	_ = w.s.Read().QueryRow("SELECT state FROM downloads WHERE id = ?", w.dlID).Scan(&state)
	if state != "downloading" || w.release(t).State == "turned_down" {
		t.Fatalf("download %s release %s", state, w.release(t).State)
	}
}

func TestPackWindowImportWaitsWhenNothingIsFinished(t *testing.T) {
	w := showSetup(t, []int{2}, ep(1), ep(2), ep(3), ep(4))
	packWindowDownload(t, w, 0.4)
	_, err := (&Importer{Store: w.s}).Import(context.Background(), w.dlID, false, domain.System)
	if !errors.Is(err, ErrPackWaiting) {
		t.Fatalf("err %v", err)
	}
	if sl := w.slot(t, 2); sl.State != mediafiles.SlotDownloading {
		t.Fatalf("E02 %+v", sl)
	}
}
