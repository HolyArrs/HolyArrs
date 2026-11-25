package importer

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// oldDouble puts an old copy that holds episodes 1 and 2 in one file on
// both their slots, and returns its id and Library path.
func oldDouble(t *testing.T, w showWorld, name string) (int64, string) {
	t.Helper()
	rel := w.folder + "/Season 1/" + name + ".mkv"
	write(t, filepath.Join(w.lib, filepath.FromSlash(rel)), 100)
	var id int64
	if err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		f := &mediafiles.File{SlotIDs: []int64{w.slots[1], w.slots[2]}, VersionID: w.vID, RelativePath: rel, Size: 100, AddedAt: time.Now().Add(-time.Hour),
			Languages: []string{}, MediaInfo: map[string]any{}, ReleaseName: &name, Origin: "imported", PlacedBy: "hardlink", Role: "main", State: "present"}
		var err error
		if id, err = database.Insert(tx, "files", f, domain.System); err != nil {
			return err
		}
		_, err = tx.Exec("UPDATE slots SET file_id = ? WHERE id IN (?, ?)", id, w.slots[1], w.slots[2])
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return id, rel
}

// failFor makes every way of placing a file fail for sources whose name
// holds part, as a full disk or a dropped share would.
func failFor(part string) Ops {
	no := errors.New("injected failure")
	pick := func(real func(a, b string) error) func(a, b string) error {
		return func(a, b string) error {
			if strings.Contains(filepath.Base(a), part) {
				return no
			}
			return real(a, b)
		}
	}
	return Ops{Link: pick(os.Link), Copy: pick(copyFile), Move: pick(renameNoReplace)}
}

// An old copy that holds two episodes must stay when only one of the two
// new episode files is placed. Before, the plan alone decided, so the old
// copy went to the holding folder and the second episode had no file.
func TestPartlyPlacedUpgradeKeepsOldDoubleEpisode(t *testing.T) {
	w := showSetup(t, []int{1, 2}, ep(1), ep(2))
	ctx := context.Background()
	oldID, rel := oldDouble(t, w, "Copper.Valley.S01E01E02.720p.HDTV.x264-OLD")
	imp, err := (&Importer{Store: w.s, Ops: failFor("S01E02")}).Import(ctx, w.dlID, false, domain.System)
	if err != nil {
		t.Fatalf("import: %v %+v", err, imp)
	}
	if _, err := os.Stat(filepath.Join(w.lib, filepath.FromSlash(rel))); err != nil {
		t.Fatalf("the old copy left its spot: %v", err)
	}
	of := w.file(t, oldID)
	if of.State != "present" || of.RelativePath != rel {
		t.Fatalf("old file %+v", of)
	}
	if !slices.Equal(of.SlotIDs, []int64{w.slots[2]}) {
		t.Fatalf("old file should hold only episode 2, holds %v", of.SlotIDs)
	}
	if sl := w.slot(t, 1); sl.FileID == nil || *sl.FileID == oldID {
		t.Fatalf("episode 1 should have the new copy: %+v", sl)
	}
	if sl := w.slot(t, 2); sl.FileID == nil || *sl.FileID != oldID {
		t.Fatalf("episode 2 should keep the old copy: %+v", sl)
	}
	held, _ := filepath.Glob(filepath.Join(w.lib, HoldingDir, "*", "*", "*", "*"))
	if len(held) > 0 {
		t.Fatalf("nothing should be in the holding folder, found %v", held)
	}
}

// When both new episode files are placed, the old double copy is
// replaced and set aside as before.
func TestFullyPlacedUpgradeReplacesOldDoubleEpisode(t *testing.T) {
	w := showSetup(t, []int{1, 2}, ep(1), ep(2))
	ctx := context.Background()
	oldID, rel := oldDouble(t, w, "Copper.Valley.S01E01E02.720p.HDTV.x264-OLD")
	imp, err := (&Importer{Store: w.s}).Import(ctx, w.dlID, false, domain.System)
	if err != nil || imp.State != "done" {
		t.Fatalf("import: %v %+v", err, imp)
	}
	of := w.file(t, oldID)
	if of.State != "replaced" || !strings.HasPrefix(of.RelativePath, HoldingDir+"/") {
		t.Fatalf("old file %+v", of)
	}
	if _, err := os.Stat(filepath.Join(w.lib, filepath.FromSlash(rel))); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("old copy still in the season folder")
	}
	for _, n := range []int{1, 2} {
		if sl := w.slot(t, n); sl.FileID == nil || *sl.FileID == oldID {
			t.Fatalf("episode %d: %+v", n, sl)
		}
	}
}

// An import cut short after it set the old two-episode copy aside, then
// tried again once the second new file is gone, must bring the old copy
// back: it is still the only file for the second episode.
func TestResumedPartialUpgradeBringsTheOldCopyBack(t *testing.T) {
	w := showSetup(t, []int{1, 2}, ep(1), ep(2))
	ctx := context.Background()
	oldID, rel := oldDouble(t, w, "Copper.Valley.S01E01E02.720p.HDTV.x264-OLD")
	crashAt = func(stage string) error {
		if stage == "held_late" {
			return errors.New("killed")
		}
		return nil
	}
	_, err := (&Importer{Store: w.s}).Import(ctx, w.dlID, false, domain.System)
	crashAt = nil
	if err == nil {
		t.Fatal("no crash")
	}
	if _, err := os.Stat(filepath.Join(w.lib, filepath.FromSlash(rel))); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the crash came before the old copy was set aside")
	}
	if err := os.Remove(filepath.Join(w.dl, packName, ep(2))); err != nil {
		t.Fatal(err)
	}
	if _, err := (&Importer{Store: w.s}).Import(ctx, w.dlID, false, domain.System); err != nil {
		t.Fatal(err)
	}
	of := w.file(t, oldID)
	if of.State != "present" || !slices.Contains(of.SlotIDs, w.slots[2]) {
		t.Fatalf("old file %+v", of)
	}
	if _, err := os.Stat(filepath.Join(w.lib, filepath.FromSlash(of.RelativePath))); err != nil {
		t.Fatalf("episode 2 points at a missing file: %v", err)
	}
}

// When the retry refuses the steps that would replace the old copy (it was
// protected after the cut-short run), the copy still comes back from the
// holding folder instead of staying there with its record pointing at an
// empty spot.
func TestRefusedRetryBringsTheOldCopyBack(t *testing.T) {
	w := showSetup(t, []int{1, 2}, ep(1), ep(2))
	ctx := context.Background()
	oldID, rel := oldDouble(t, w, "Copper.Valley.S01E01E02.720p.HDTV.x264-OLD")
	crashAt = func(stage string) error {
		if stage == "held_late" {
			return errors.New("killed")
		}
		return nil
	}
	_, err := (&Importer{Store: w.s}).Import(ctx, w.dlID, false, domain.System)
	crashAt = nil
	if err == nil {
		t.Fatal("no crash")
	}
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE files SET protected = 1 WHERE id = ?", oldID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	_, _ = (&Importer{Store: w.s}).Import(ctx, w.dlID, false, domain.System)
	of := w.file(t, oldID)
	if _, err := os.Stat(filepath.Join(w.lib, filepath.FromSlash(of.RelativePath))); err != nil || of.State != "present" {
		t.Fatalf("the old copy stayed in the holding folder: %+v %v", of, err)
	}
	if of.RelativePath != rel {
		t.Fatalf("old copy at %s, want %s", of.RelativePath, rel)
	}
}

// When the old copy's place was taken by another file after the cut-short
// run, the retry stops before recording anything: the other file stays as
// it is, and no episode is recorded on a file that is not its own.
func TestRetryStopsWhenTheOldCopysPlaceIsTaken(t *testing.T) {
	w := showSetup(t, []int{1, 2}, ep(1), ep(2))
	ctx := context.Background()
	oldID, rel := oldDouble(t, w, "Copper.Valley.S01E01E02.720p.HDTV.x264-OLD")
	crashAt = func(stage string) error {
		if stage == "held_late" {
			return errors.New("killed")
		}
		return nil
	}
	_, err := (&Importer{Store: w.s}).Import(ctx, w.dlID, false, domain.System)
	crashAt = nil
	if err == nil {
		t.Fatal("no crash")
	}
	if err := os.Remove(filepath.Join(w.dl, packName, ep(2))); err != nil {
		t.Fatal(err)
	}
	spot := filepath.Join(w.lib, filepath.FromSlash(rel))
	if err := os.WriteFile(spot, []byte("someone else's file"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := (&Importer{Store: w.s}).Import(ctx, w.dlID, false, domain.System); err == nil || !strings.Contains(err.Error(), "is taken") {
		t.Fatalf("retry over a taken place: %v", err)
	}
	if b, _ := os.ReadFile(spot); string(b) != "someone else's file" {
		t.Fatal("the other file changed")
	}
	held, _ := filepath.Glob(filepath.Join(w.lib, HoldingDir, "*", filepath.FromSlash(rel)))
	if len(held) != 1 {
		t.Fatalf("the old copy is not waiting in the holding folder: %v", held)
	}
	if of := w.file(t, oldID); of.State != "present" || of.RelativePath != rel {
		t.Fatalf("old file record changed: %+v", of)
	}
}
