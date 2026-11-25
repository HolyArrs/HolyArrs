package importer

import (
	"context"
	"database/sql"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"os"
	"path/filepath"
	"testing"
)

// a movie kept in a flat Library folder (no folder of its own) is
// imported as a file directly in it, named like any other; a file of that
// name already there is never replaced, and no other file is touched.
func TestImportIntoFlatFolder(t *testing.T) {
	dl, lib := t.TempDir(), t.TempDir()
	write(t, filepath.Join(dl, relName+".mkv"), 1000)
	write(t, filepath.Join(lib, "Alien (1979).mkv"), 5)
	write(t, filepath.Join(lib, "poster.jpg"), 3)
	tg := target(lib)
	tg.FolderName = ""
	cands, _ := Scan(dl)
	steps := Plan(cands, tg)
	want := filepath.Join(lib, "Dune - Part Two (2024) 1080p WEB.mkv")
	if len(steps) != 1 || steps[0].TargetPath != want || steps[0].Method != "hardlink" {
		t.Fatalf("steps %+v", steps)
	}
	write(t, want, 7) // another file already has the name
	if res := Execute(steps, Ops{}); res[0].Outcome != "refused" {
		t.Fatalf("result %+v", res)
	}
	if fi, _ := os.Stat(want); fi.Size() != 7 {
		t.Fatal("existing file replaced")
	}
	_ = os.Remove(want)
	if res := Execute(steps, Ops{}); res[0].Outcome != "placed" {
		t.Fatalf("result %+v", res)
	}
	ents, _ := os.ReadDir(lib)
	if len(ents) != 3 {
		t.Fatalf("library holds %v", ents)
	}
	for _, e := range ents {
		if e.IsDir() {
			t.Fatalf("a folder was made: %s", e.Name())
		}
	}
	if fi, _ := os.Stat(filepath.Join(lib, "Alien (1979).mkv")); fi.Size() != 5 {
		t.Fatal("another movie changed")
	}
}

// a real download for a movie kept flat lands as a file directly in the
// flat Library folder: no movie folder is made, other movies stay as they are.
func TestImportRunIntoFlatFolder(t *testing.T) {
	w := setup(t)
	ctx := context.Background()
	write(t, filepath.Join(w.lib, "Alien (1979).mkv"), 5)
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE library_folders SET flat = 1"); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE versions SET folder_name = '' WHERE id = ?", w.vID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	im := &Importer{Store: w.s}
	imp, err := im.Import(ctx, w.dlID, false, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if imp.State != "done" {
		t.Fatalf("import %+v", imp)
	}
	files, _ := database.Select[mediafiles.File](ctx, w.s.Read(), "files", "")
	if len(files) != 1 || files[0].RelativePath != "Dune - Part Two (2024) 1080p WEB.mkv" {
		t.Fatalf("files %+v", files)
	}
	ents, _ := os.ReadDir(w.lib)
	for _, e := range ents {
		if e.IsDir() {
			t.Fatalf("a folder was made: %s", e.Name())
		}
	}
	if fi, _ := os.Stat(filepath.Join(w.lib, "Alien (1979).mkv")); fi == nil || fi.Size() != 5 {
		t.Fatal("another movie changed")
	}
}
