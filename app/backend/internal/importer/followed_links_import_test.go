package importer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// linkOut moves a downloaded file into mount and leaves a link to it in its
// place, as a streaming drive (InfiniDysk, AltMount) lays out a finished job.
func linkOut(t *testing.T, file, mount string) string {
	t.Helper()
	target := filepath.Join(mount, filepath.Base(file))
	if err := os.Rename(file, target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, file); err != nil {
		t.Skip("symlinks not available:", err)
	}
	return target
}

func followLinks(t *testing.T) {
	t.Helper()
	old := FollowLinks
	FollowLinks = true
	t.Cleanup(func() { FollowLinks = old })
}

// A movie whose downloaded file is a link imports as a link to the same
// file, and Import again finds it in place.
func TestImportMovieFromALink(t *testing.T) {
	w := setup(t)
	target := linkOut(t, filepath.Join(w.dl, relName+".mkv"), t.TempDir())
	followLinks(t)
	im := &Importer{Store: w.s}
	ctx := context.Background()
	imp, err := im.Import(ctx, w.dlID, false, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if len(imp.Result) != 1 || imp.Result[0].Outcome != "placed" {
		t.Fatalf("import %s %+v", imp.State, imp.Result)
	}
	placed := imp.Plan[0].TargetPath
	if fi, err := os.Lstat(placed); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("want a link in the Library at %s: %v %v", placed, fi, err)
	}
	if got := mustReal(t, placed); got != mustReal(t, target) {
		t.Fatalf("the Library link points to %q, want %q", got, target)
	}
	if again := place(imp.Plan[0], Ops{}, time.Time{}); again.Outcome != "placed" {
		t.Fatalf("Import again: %+v", again)
	}
}

// A movie whose downloaded file is a link to nothing (the drive is not
// mounted) stops for a decision with that reason, not "no file matched", and
// places nothing.
func TestImportMovieFromADeadLink(t *testing.T) {
	w := setup(t)
	target := linkOut(t, filepath.Join(w.dl, relName+".mkv"), t.TempDir())
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	followLinks(t)
	im := &Importer{Store: w.s}
	ctx := context.Background()
	if _, err := im.Import(ctx, w.dlID, false, domain.System); err != nil {
		t.Fatal(err)
	}
	var state, why string
	if err := w.s.Read().QueryRowContext(ctx, "SELECT state, last_reason FROM slots WHERE id = ?", w.slotID).Scan(&state, &why); err != nil {
		t.Fatal(err)
	}
	if state != string(mediafiles.SlotNeedsADecision) || why != deadLinkWhy {
		t.Fatalf("slot %s: %q", state, why)
	}
	if entries, _ := os.ReadDir(w.lib); len(entries) != 0 {
		t.Fatal("something was placed for a dead link")
	}
	if !CanImportAgain(why) {
		t.Fatal("once the drive is mounted, Import again must be offered")
	}
}

// A season pack whose episodes are all links fills every episode with a
// link to its file.
func TestImportSeasonPackOfLinks(t *testing.T) {
	w := showSetup(t, []int{1, 2}, ep(1), ep(2))
	mount := t.TempDir()
	for n := 1; n <= 2; n++ {
		linkOut(t, filepath.Join(w.dl, packName, ep(n)), mount)
	}
	followLinks(t)
	im := &Importer{Store: w.s}
	imp, err := im.Import(context.Background(), w.dlID, false, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if imp.State != "done" {
		t.Fatalf("import %s: %+v", imp.State, imp.Result)
	}
	for n := 1; n <= 2; n++ {
		sl := w.slot(t, n)
		if sl.FileID == nil {
			t.Fatalf("E%d not filled: %+v", n, sl)
		}
		f := w.file(t, *sl.FileID)
		p := filepath.Join(w.lib, filepath.FromSlash(f.RelativePath))
		if fi, err := os.Lstat(p); err != nil || fi.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("E%d: want a link at %s: %v %v", n, p, fi, err)
		}
		if got := mustReal(t, p); got != mustReal(t, filepath.Join(mount, ep(n))) {
			t.Fatalf("E%d points to %q", n, got)
		}
	}
}
