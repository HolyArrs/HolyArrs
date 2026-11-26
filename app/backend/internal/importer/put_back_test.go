package importer

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// moveFile never replaces a file already at the target, so putting an
// old copy back can never destroy a new copy that took its spot.
func TestMoveFileNeverReplaces(t *testing.T) {
	dir := t.TempDir()
	from, to := filepath.Join(dir, "held.mkv"), filepath.Join(dir, "spot", "a.mkv")
	if err := os.WriteFile(from, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(to, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := moveFile(from, to); err == nil {
		t.Fatal("moveFile replaced a file")
	}
	if b, _ := os.ReadFile(to); string(b) != "new" {
		t.Fatalf("the file at the target changed: %q", b)
	}
	if b, _ := os.ReadFile(from); string(b) != "old" {
		t.Fatalf("the source changed: %q", b)
	}
}

// A Move and Replace by hand whose database record fails after the new
// file took the old one's name keeps both files: the new one goes back
// where it came from and the old one returns to its place. Before, the
// old one was renamed over the new one, and with Move the original was
// already gone, so the new file was lost.
func TestByHandMoveReplaceKeepsBothFilesWhenTheRecordFails(t *testing.T) {
	w := setup(t)
	ctx := context.Background()
	im := &Importer{Store: w.s}
	if _, err := im.Import(ctx, w.dlID, false, domain.System); err != nil {
		t.Fatal(err)
	}
	old := slot308(t, w)
	var oldFile mediafiles.File
	_ = database.Get(ctx, w.s.Read(), "files", *old.FileID, &oldFile)
	oldAbs := filepath.Join(w.lib, filepath.FromSlash(oldFile.RelativePath))
	oldBytes, err := os.ReadFile(oldAbs)
	if err != nil {
		t.Fatal(err)
	}
	// Same release name, so the new file is named like the old one.
	loose := t.TempDir()
	src := filepath.Join(loose, relName+".mkv")
	newBytes := bytes.Repeat([]byte("new copy "), 512)
	if err := os.WriteFile(src, newBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	im.SavePath = loose
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("CREATE TRIGGER fail_new_file BEFORE INSERT ON files BEGIN SELECT RAISE(ABORT, 'injected'); END")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	res, err := im.ImportByHand(ctx, []ByHandItem{{Path: src, SlotID: w.slotID, Mode: "move", Replace: true}}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if res[0].Outcome == "placed" && res[0].Error == "" {
		t.Fatalf("the failed record was reported as placed: %+v", res[0])
	}
	if b, err := os.ReadFile(src); err != nil || !bytes.Equal(b, newBytes) {
		t.Fatalf("the new file is lost: %v", err)
	}
	if b, err := os.ReadFile(oldAbs); err != nil || !bytes.Equal(b, oldBytes) {
		t.Fatalf("the old file is not back in its place: %v", err)
	}
}

// A name reached through a symlinked folder, or one that differs only in
// case in the same folder, may be the very same entry: a Move must not
// remove it. Two names in two real folders are two entries.
func TestOneEntry(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "Movies", "Heat (1995)")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(real, "heat.mkv")
	if err := os.WriteFile(file, []byte("h"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, filepath.Join(dir, "alias")); err != nil {
		t.Skip("no symlinks here")
	}
	other := filepath.Join(dir, "Other")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(file, filepath.Join(other, "heat.mkv")); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{filepath.Join(dir, "alias", "heat.mkv"), file, true},
		{filepath.Join(real, "Heat.mkv"), file, true},
		{filepath.Join(other, "heat.mkv"), file, false},
		{filepath.Join(dir, "gone", "heat.mkv"), file, true},
	} {
		if got := oneEntry(c.a, c.b); got != c.want {
			t.Errorf("oneEntry(%s, %s) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

// The fallback rename never replaces a file, even where it cannot rely on
// the system's own no-replace rename.
func TestRenameCheckedNeverReplaces(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	_ = os.WriteFile(src, []byte("old"), 0o644)
	_ = os.WriteFile(dst, []byte("new"), 0o644)
	if err := renameChecked(src, dst); err == nil {
		t.Fatal("replaced a file")
	}
	if b, _ := os.ReadFile(dst); string(b) != "new" {
		t.Fatalf("target changed: %q", b)
	}
	free := filepath.Join(dir, "c")
	if err := renameChecked(src, free); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatal("source left behind")
	}
	if b, _ := os.ReadFile(free); string(b) != "old" {
		t.Fatalf("moved file %q", b)
	}
}
