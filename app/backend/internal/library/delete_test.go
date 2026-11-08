package library

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
)

func deleteWorld(t *testing.T) (*Service, *database.Store, mediafiles.LibraryFolder, int64, [3]mediafiles.File) {
	t.Helper()
	svc, s, lf, id := renameWorld(t)
	imp := putFile(t, s, lf, id, "Dune - Part Two (2024)/Dune - Part Two (2024) 1080p BluRay.mkv", "Dune.Part.Two.2024.1080p.BluRay-HOLYARRS", "imported")
	ado := putFile(t, s, lf, id, "Dune - Part Two (2024)/old copy.mkv", "Dune.Part.Two.2024.720p.WEB-DL-X", "adopted")
	fnd := putFile(t, s, lf, id, "Dune - Part Two (2024)/someone else.mkv", "x", "found")
	return svc, s, lf, id, [3]mediafiles.File{imp, ado, fnd}
}

func onDisk(lf mediafiles.LibraryFolder, rel string) bool {
	_, err := os.Stat(filepath.Join(lf.Path, filepath.FromSlash(rel)))
	return err == nil
}

// 5.1.3: by default a delete keeps every file on disk.
func TestDeleteKeepsFilesByDefault(t *testing.T) {
	svc, s, lf, id, fs := deleteWorld(t)
	ctx := context.Background()
	plan, err := svc.Remove(ctx, id, RemoveChoice{}, true, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Held) != 0 || len(plan.Kept) != 3 {
		t.Fatalf("plan %+v", plan)
	}
	for _, f := range fs {
		if !onDisk(lf, f.RelativePath) {
			t.Fatalf("%s left the disk", f.RelativePath)
		}
	}
	var ti mediafiles.Title
	_ = database.Get(ctx, s.Read(), "titles", id, &ti)
	if ti.State != "removed" || ti.KeepUpToDate {
		t.Fatalf("title %s keep %v", ti.State, ti.KeepUpToDate)
	}
	for _, sl := range slotsOf(t, s, id) {
		if sl.State != mediafiles.SlotRemoved {
			t.Fatalf("slot %s", sl.State)
		}
	}
	// It can be added again later.
	if _, err := AddMovie(ctx, s, dune, AddOptions{}, nil, now, domain.System); err != nil {
		t.Fatalf("add again: %v", err)
	}
}

// Deleting files moves only imported and adopted files to the holding
// folder; a found file stays where it is, and so does its folder.
func TestDeleteFilesGoToHoldingNeverFoundOnes(t *testing.T) {
	svc, s, lf, id, fs := deleteWorld(t)
	ctx := context.Background()
	before := s.WriteCount()
	preview, err := svc.Remove(ctx, id, RemoveChoice{DeleteFiles: true}, false, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if s.WriteCount() != before || !onDisk(lf, fs[0].RelativePath) {
		t.Fatal("the dry run changed something")
	}
	plan, err := svc.Remove(ctx, id, RemoveChoice{DeleteFiles: true}, true, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Held) != 2 || len(plan.Kept) != 1 || plan.Kept[0].FileID != fs[2].ID || !strings.Contains(plan.Kept[0].Why, "found") {
		t.Fatalf("plan %+v", plan)
	}
	for i, h := range plan.Held {
		if h.To != preview.Held[i].To {
			t.Fatalf("preview %q, result %q", preview.Held[i].To, h.To)
		}
		if onDisk(lf, h.Path) || !onDisk(lf, h.To) || !strings.HasPrefix(h.To, importer.HoldingDir+"/2026-10-13/") {
			t.Fatalf("held %+v", h)
		}
		if f := fileRow(t, s, h.FileID); f.State != "gone" || f.RelativePath != h.To {
			t.Fatalf("row %+v", f)
		}
	}
	if !onDisk(lf, fs[2].RelativePath) {
		t.Fatal("found file touched")
	}
}

func TestDeleteWithNeverAdd(t *testing.T) {
	svc, s, _, id, _ := deleteWorld(t)
	ctx := context.Background()
	if _, err := svc.Remove(ctx, id, RemoveChoice{NeverAdd: true}, true, domain.System); err != nil {
		t.Fatal(err)
	}
	if _, err := AddMovie(ctx, s, dune, AddOptions{}, nil, now, domain.System); !errors.Is(err, ErrNeverAdd) {
		t.Fatalf("err %v", err)
	}
	if _, err := svc.Remove(ctx, id, RemoveChoice{}, true, domain.System); !errors.Is(err, ErrAlreadyRemoved) {
		t.Fatalf("second delete %v", err)
	}
}

// The clean-up empties holding days older than the setting and nothing
// else: today's files, a folder that is not a day, and files elsewhere stay.
func TestHoldingCleanUp(t *testing.T) {
	svc, s, lf, _, fs := deleteWorld(t)
	ctx := context.Background()
	hold := filepath.Join(lf.Path, importer.HoldingDir)
	writeFile(t, filepath.Join(hold, "2026-10-01", "Old (2020)", "old.mkv"), "x")
	writeFile(t, filepath.Join(hold, "2026-10-06", "Edge (2020)", "edge.mkv"), "x")
	writeFile(t, filepath.Join(hold, "2026-10-12", "New (2020)", "new.mkv"), "x")
	writeFile(t, filepath.Join(hold, "notes", "keep.txt"), "x")
	// A replaced File kept in the holding folder by an import.
	rel := importer.HoldingDir + "/2026-10-01/Dune - Part Two (2024)/old copy.mkv"
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE files SET state = 'replaced', relative_path = ? WHERE id = ?", rel, fs[1].ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(rel)), "x")

	n, err := svc.CleanHolding(ctx, domain.System)
	if err != nil || n != 1 {
		t.Fatalf("cleaned %d %v", n, err)
	}
	for dir, want := range map[string]bool{"2026-10-01": false, "2026-10-06": true, "2026-10-12": true, "notes": true} {
		if _, err := os.Stat(filepath.Join(hold, dir)); (err == nil) != want {
			t.Errorf("%s there=%v, want %v", dir, err == nil, want)
		}
	}
	if f := fileRow(t, s, fs[1].ID); f.State != "gone" {
		t.Fatalf("replaced row %+v", f)
	}
	if !onDisk(lf, fs[0].RelativePath) || !onDisk(lf, fs[2].RelativePath) {
		t.Fatal("a Library file was touched")
	}
	// 0 days keeps everything.
	if err := importer.SaveFileRules(ctx, s, importer.FileRules{HoldingDays: 0, MinFreeMB: 100}, domain.System); err != nil {
		t.Fatal(err)
	}
	svc.Now = func() time.Time { return now.AddDate(1, 0, 0) }
	if n, _ := svc.CleanHolding(ctx, domain.System); n != 0 {
		t.Fatalf("0 days still cleaned %d", n)
	}
}
