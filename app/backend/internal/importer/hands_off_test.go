package importer

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// In hands-off mode a finished download is never imported: the file stays
// in the download folder, the Library folder stays empty, no File or
// import is recorded, and the Slot reads as downloaded and good enough.
// With the mode off again, a title that only has the stand-in folder is
// refused with a clear line instead of being written somewhere.
func TestHandsOffImportLeavesFiles(t *testing.T) {
	cases := []struct {
		name     string
		handsOff bool
		standIn  bool
		dryRun   bool
		wantErr  error
		state    mediafiles.SlotState
		placed   bool
	}{
		{"normal import", false, false, false, nil, mediafiles.SlotInLibrary, true},
		{"hands-off", true, false, false, nil, mediafiles.SlotInLibrary, false},
		{"hands-off dry run", true, false, true, nil, mediafiles.SlotDownloading, false},
		{"hands-off with stand-in", true, true, false, nil, mediafiles.SlotInLibrary, false},
		{"stand-in after hands-off ends", false, true, false, ErrStandInFolder, mediafiles.SlotDownloading, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := setup(t)
			ctx := context.Background()
			if err := parts.SetHandsOff(ctx, w.s, nil, tc.handsOff, domain.System); err != nil {
				t.Fatal(err)
			}
			if tc.standIn {
				lf, err := mediafiles.StandInFolder(ctx, w.s, "movie", domain.System)
				if err != nil {
					t.Fatal(err)
				}
				if err := w.s.Write(ctx, func(tx *sql.Tx) error {
					_, err := tx.Exec("UPDATE versions SET library_folder_id = ? WHERE id = ?", lf.ID, w.vID)
					return err
				}); err != nil {
					t.Fatal(err)
				}
			}
			imp, err := (&Importer{Store: w.s}).Import(ctx, w.dlID, tc.dryRun, domain.System)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err %v, want %v", err, tc.wantErr)
			}
			var sl mediafiles.Slot
			if err := mediafiles.LoadSlot(ctx, w.s.Read(), w.slotID, &sl); err != nil {
				t.Fatal(err)
			}
			if sl.State != tc.state {
				t.Fatalf("slot %s, want %s", sl.State, tc.state)
			}
			entries, _ := os.ReadDir(w.lib)
			if placed := len(entries) > 0; placed != tc.placed {
				t.Fatalf("library has %d entries", len(entries))
			}
			if _, err := os.Stat(filepath.Join(w.dl, relName+".mkv")); err != nil {
				t.Fatalf("the download's file moved: %v", err)
			}
			if !tc.handsOff || tc.dryRun {
				return
			}
			if imp == nil || imp.Mode != "hands_off" || sl.FileID != nil || !sl.GoodEnough || sl.ActiveDownloadID != nil ||
				sl.LastReason == nil || *sl.LastReason != mediafiles.HandsOffDownloadedWhy {
				t.Fatalf("import %+v slot %+v", imp, sl)
			}
			var files, imports int
			_ = w.s.Read().QueryRow("SELECT (SELECT count(*) FROM files), (SELECT count(*) FROM imports)").Scan(&files, &imports)
			if files != 0 || imports != 0 {
				t.Fatalf("%d files, %d imports recorded", files, imports)
			}
			// A second finish of the same download changes nothing.
			if _, err := (&Importer{Store: w.s}).Import(ctx, w.dlID, false, domain.System); err != nil {
				t.Fatal(err)
			}
			var n int
			_ = w.s.Read().QueryRow("SELECT count(*) FROM slots WHERE state = 'in_library'").Scan(&n)
			if n != 1 {
				t.Fatalf("%d slots in the library", n)
			}
		})
	}
}
