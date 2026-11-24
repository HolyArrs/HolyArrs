package library

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// A look-only Library folder shows its titles, but HolyArrs never
// searches for them, renames, moves, tidies or deletes their files.
func TestLookOnlyFolderNeverChangesFiles(t *testing.T) {
	svc, s, lf, id, fs := deleteWorld(t)
	ctx := context.Background()
	on := true
	got, err := UpdateFolder(ctx, s, lf.ID, FolderChange{LookOnly: &on}, domain.System)
	if err != nil || !got.LookOnly {
		t.Fatalf("look-only not set: %+v %v", got, err)
	}

	var ti mediafiles.Title
	if err := database.Get(ctx, s.Read(), "titles", id, &ti); err != nil {
		t.Fatal(err)
	}
	if ti.KeepUpToDate {
		t.Fatal("a title only in a look-only folder is still kept up to date")
	}
	if !mediafiles.TitleLookOnly(ctx, s.Read(), id) {
		t.Fatal("TitleLookOnly false")
	}
	keep := true
	if _, err := svc.Edit(ctx, id, TitleChange{KeepUpToDate: &keep}, false, domain.System); !errors.Is(err, mediafiles.ErrLookOnly) {
		t.Fatalf("keep up to date turned on: %v", err)
	}

	plan, err := PlanRename(ctx, s.Read(), []int64{id})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) != 0 {
		t.Fatalf("rename planned in a look-only folder: %+v", plan)
	}
	if _, err := PlanTidy(ctx, s.Read(), lf.ID); !errors.Is(err, mediafiles.ErrLookOnly) {
		t.Fatalf("tidy: %v", err)
	}
	if _, err := UndoTidy(ctx, s, lf.ID, domain.System); !errors.Is(err, mediafiles.ErrLookOnly) {
		t.Fatalf("undo tidy: %v", err)
	}

	other, err := AddFolder(ctx, s, FolderInput{Path: t.TempDir(), Kind: "movie", Label: "Writable"}, nil, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	moves, err := PlanMove(ctx, s.Read(), s.Read(), id, other.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range moves {
		if m.Blocked == "" {
			t.Fatalf("move out of a look-only folder not blocked: %+v", m)
		}
	}
	if _, err := PlanMove(ctx, s.Read(), s.Read(), id, lf.ID); !errors.Is(err, mediafiles.ErrLookOnly) {
		t.Fatalf("move into a look-only folder: %v", err)
	}
	same := lf.ID
	if _, err := svc.Edit(ctx, id, TitleChange{LibraryFolderID: &same}, false, domain.System); err != nil {
		t.Fatalf("an edit naming the folder it is in: %v", err)
	}
	if _, err := svc.Edit(ctx, id, TitleChange{LibraryFolderID: &other.ID}, true, domain.System); err != nil {
		t.Logf("move out (dry run): %v", err)
	}

	// Removing the title, even with delete files, keeps every file on disk.
	rp, err := svc.Remove(ctx, id, RemoveChoice{DeleteFiles: true}, true, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if len(rp.Held) != 0 {
		t.Fatalf("files held from a look-only folder: %+v", rp.Held)
	}
	for _, f := range fs {
		if !onDisk(lf, f.RelativePath) {
			t.Fatalf("%s left the disk", f.RelativePath)
		}
	}
}

// A look-only folder is never picked as the folder for new titles, and a
// title cannot be moved into it.
func TestLookOnlyFolderNotADefault(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	lf, err := AddFolder(ctx, s, FolderInput{Path: t.TempDir(), Kind: "movie", LookOnly: true}, nil, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if !lf.LookOnly || lf.Health != "ok" {
		t.Fatalf("folder %+v", lf)
	}
	if UsableDefault(ctx, s.Read(), "library_folders", "movie", lf.ID) {
		t.Fatal("a look-only folder is a usable default")
	}
	if !mediafiles.FolderLookOnly(ctx, s.Read(), lf.ID) {
		t.Fatal("FolderLookOnly false")
	}
}

// A .strm link another app made is found like a video.
func TestListVideosFindsStrm(t *testing.T) {
	lib := t.TempDir()
	writeFile(t, filepath.Join(lib, "Heat (1995)", "Heat (1995).strm"), "http://example.invalid/heat")
	got := ListVideosN(lib, "Heat (1995)", 10)
	if len(got) != 1 || got[0].Path != "Heat (1995)/Heat (1995).strm" {
		t.Fatalf("videos %+v", got)
	}
}

// A look-only folder is often a read-only share: adding a title there
// must not ask for write access.
func TestCheckUseLookOnlyReadOnly(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0o755)
	if f, err := os.CreateTemp(dir, "probe"); err == nil {
		f.Close()
		os.Remove(f.Name())
		t.Skip("the folder still takes writes here (Windows or root)")
	}
	if h := CheckUse(mediafiles.LibraryFolder{Path: dir}, nil); h.Usable() {
		t.Fatalf("a read-only normal folder is usable: %+v", h)
	}
	if h := CheckUse(mediafiles.LibraryFolder{Path: dir, LookOnly: true}, nil); !h.Usable() {
		t.Fatalf("a read-only look-only folder is not usable: %+v", h)
	}
	if h := CheckUse(mediafiles.LibraryFolder{Path: filepath.Join(dir, "gone"), LookOnly: true}, nil); h.Usable() {
		t.Fatal("a missing look-only folder is usable")
	}
}
