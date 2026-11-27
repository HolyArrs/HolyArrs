package library

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// a movie's file replaced outside the app (old gone, new one beside
// it) is adopted on rescan; the missing row no longer blocks the slot.
func TestMissingMovieFileDoesNotBlockNewCopy(t *testing.T) {
	svc, s, id, rel := movieWithFoundCopy(t, "")
	ctx := context.Background()
	v := versionsOf(t, s, id)[0]
	var lf mediafiles.LibraryFolder
	if err := database.Get(ctx, s.Read(), "library_folders", v.LibraryFolderID, &lf); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Rescan(ctx, domain.System); err != nil {
		t.Fatal(err)
	}
	old := slotsOf(t, s, id)[0]
	if old.FileID == nil {
		t.Fatal("first copy was not linked")
	}
	newRel := filepath.ToSlash(filepath.Join(filepath.Dir(rel), "Dune.Part.Two.2024.2160p.WEB-DL.mkv"))
	if err := os.Remove(filepath.Join(lf.Path, filepath.FromSlash(rel))); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(newRel)), "new copy")
	for i := 0; i < 2; i++ { // the missing mark and the adoption may land in separate passes
		if _, err := svc.Rescan(ctx, domain.System); err != nil {
			t.Fatal(err)
		}
	}
	sl := slotsOf(t, s, id)[0]
	if sl.FileID == nil || *sl.FileID == *old.FileID {
		t.Fatalf("slot still on the missing file: %+v", sl)
	}
	var f mediafiles.File
	if err := database.Get(ctx, s.Read(), "files", *sl.FileID, &f); err != nil || f.RelativePath != newRel || f.State != "present" {
		t.Fatalf("linked file %+v %v", f, err)
	}
	var gone mediafiles.File
	if err := database.Get(ctx, s.Read(), "files", *old.FileID, &gone); err != nil || gone.State != "missing" || len(gone.SlotIDs) != 0 {
		t.Fatalf("missing row %+v %v", gone, err)
	}
}
