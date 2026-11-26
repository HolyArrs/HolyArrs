package importer

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// Review cases for the put-back guard.

func adv638FailFiles(t *testing.T, w world) {
	t.Helper()
	if err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("CREATE TRIGGER fail_new_file BEFORE INSERT ON files BEGIN SELECT RAISE(ABORT, 'injected'); END")
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func adv638Old(t *testing.T, w world) (string, []byte) {
	t.Helper()
	old := slot308(t, w)
	var f mediafiles.File
	_ = database.Get(context.Background(), w.s.Read(), "files", *old.FileID, &f)
	p := filepath.Join(w.lib, filepath.FromSlash(f.RelativePath))
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return p, b
}

// The taken-target error is recognisable as "exists" on every system.
func TestAdv638MoveFileTakenIsErrExist(t *testing.T) {
	dir := t.TempDir()
	from, to := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	_ = os.WriteFile(from, []byte("old"), 0o644)
	_ = os.WriteFile(to, []byte("new"), 0o644)
	err := moveFile(from, to)
	if !errors.Is(err, os.ErrExist) {
		t.Fatalf("want ErrExist, got %v", err)
	}
}

// Long Windows paths still move, and still never replace.
func TestAdv638MoveFileLongPath(t *testing.T) {
	dir := t.TempDir()
	deep := dir
	for len(deep) < 300 {
		deep = filepath.Join(deep, strings.Repeat("e", 40))
	}
	from, to := filepath.Join(deep, "a.mkv"), filepath.Join(deep, "x", "a.mkv")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(from, []byte("old"), 0o644)
	if err := moveFile(from, to); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(from, []byte("again"), 0o644)
	if err := moveFile(from, to); err == nil {
		t.Fatal("long path replaced a file")
	}
	if b, _ := os.ReadFile(to); string(b) != "old" {
		t.Fatalf("target changed %q", b)
	}
}

// Cross-disk: the rename fails with a non-exist error, the copy is used,
// and the copy itself never replaces.
func TestAdv638MoveFileCrossDiskFallback(t *testing.T) {
	prev := renameFile
	renameFile = func(string, string) error { return &os.LinkError{Op: "rename", Err: errors.New("cross-device link")} }
	t.Cleanup(func() { renameFile = prev })
	dir := t.TempDir()
	from, to := filepath.Join(dir, "a"), filepath.Join(dir, "d", "b")
	_ = os.WriteFile(from, []byte("old"), 0o644)
	if err := moveFile(from, to); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(to); string(b) != "old" {
		t.Fatal("copy wrong")
	}
	_ = os.WriteFile(from, []byte("two"), 0o644)
	if err := moveFile(from, to); err == nil {
		t.Fatal("cross-disk fallback replaced a file")
	}
	if b, _ := os.ReadFile(to); string(b) != "old" {
		t.Fatal("cross-disk fallback changed target")
	}
}

// renameChecked: a missing source is an error and leaves the target alone.
func TestAdv638RenameCheckedMissingSource(t *testing.T) {
	dir := t.TempDir()
	if err := renameChecked(filepath.Join(dir, "nope"), filepath.Join(dir, "b")); err == nil {
		t.Fatal("missing source moved")
	}
	if _, err := os.Stat(filepath.Join(dir, "b")); !os.IsNotExist(err) {
		t.Fatal("target appeared")
	}
}

// renameChecked on a hard-linked source: the other link keeps the data.
func TestAdv638RenameCheckedHardlinkedSource(t *testing.T) {
	dir := t.TempDir()
	src, twin, dst := filepath.Join(dir, "a"), filepath.Join(dir, "t"), filepath.Join(dir, "b")
	_ = os.WriteFile(src, []byte("data"), 0o644)
	if err := os.Link(src, twin); err != nil {
		t.Skip("no hard links")
	}
	if err := renameChecked(src, dst); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{twin, dst} {
		if b, _ := os.ReadFile(p); string(b) != "data" {
			t.Fatalf("%s lost", p)
		}
	}
}

// By hand Copy and Replace whose record fails: source kept, old back,
// no stray new file at the target.
func TestAdv638ByHandCopyReplaceRecordFails(t *testing.T) {
	w := setup(t)
	ctx := context.Background()
	im := &Importer{Store: w.s}
	if _, err := im.Import(ctx, w.dlID, false, domain.System); err != nil {
		t.Fatal(err)
	}
	oldAbs, oldBytes := adv638Old(t, w)
	loose := t.TempDir()
	src := filepath.Join(loose, relName+".mkv")
	newBytes := bytes.Repeat([]byte("copy mode "), 400)
	_ = os.WriteFile(src, newBytes, 0o644)
	im.SavePath = loose
	adv638FailFiles(t, w)
	if _, err := im.ImportByHand(ctx, []ByHandItem{{Path: src, SlotID: w.slotID, Mode: "copy", Replace: true}}, domain.System); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(src); !bytes.Equal(b, newBytes) {
		t.Fatal("copy mode lost the source")
	}
	if b, _ := os.ReadFile(oldAbs); !bytes.Equal(b, oldBytes) {
		t.Fatal("old copy not back")
	}
}

// By hand Move and Replace that succeeds still removes the source and
// leaves the new file in place (no regression from moving the remove).
func TestAdv638ByHandMoveReplaceSucceeds(t *testing.T) {
	w := setup(t)
	ctx := context.Background()
	im := &Importer{Store: w.s}
	if _, err := im.Import(ctx, w.dlID, false, domain.System); err != nil {
		t.Fatal(err)
	}
	oldAbs, _ := adv638Old(t, w)
	loose := t.TempDir()
	src := filepath.Join(loose, relName+".mkv")
	newBytes := bytes.Repeat([]byte("moved ok "), 400)
	_ = os.WriteFile(src, newBytes, 0o644)
	im.SavePath = loose
	res, err := im.ImportByHand(ctx, []ByHandItem{{Path: src, SlotID: w.slotID, Mode: "move", Replace: true}}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if res[0].Outcome != "placed" {
		t.Fatalf("%+v", res[0])
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatal("move left the source")
	}
	if b, _ := os.ReadFile(oldAbs); !bytes.Equal(b, newBytes) {
		t.Fatal("new file not at its place")
	}
}
