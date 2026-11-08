package main

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/files"
)

// mainPlus is this build's main migrations plus one more.
func mainPlus(t *testing.T, extra string) fstest.MapFS {
	t.Helper()
	src := os.DirFS(filepath.Join("..", "..", "internal", "core", "database", "migrations", "main"))
	out := fstest.MapFS{}
	es, err := fs.ReadDir(src, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range es {
		b, _ := fs.ReadFile(src, e.Name())
		out[e.Name()] = &fstest.MapFile{Data: b}
	}
	out["9999_future.sql"] = &fstest.MapFile{Data: []byte(extra)}
	return out
}

func restoreRun(t *testing.T, args ...string) (int, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := restoreCmd(args, &out, &errOut)
	return code, out.String() + errOut.String()
}

// oldThenNew opens dir with this build, then with a "newer build" that has
// one more migration.
func oldThenNew(t *testing.T, extra string) (string, error) {
	t.Helper()
	dir := t.TempDir()
	s, err := database.Open(dir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	s, err = database.Open(dir, database.Options{MainMigrations: mainPlus(t, extra)})
	if err == nil {
		_ = s.Close()
	}
	return dir, err
}

func openVersion(t *testing.T, dir string) (int, error) {
	t.Helper()
	s, err := database.Open(dir, database.Options{})
	if err != nil {
		return 0, err
	}
	defer s.Close()
	return s.Version()
}

// Scenarios 9 and 11: after a downgrade this build refuses the newer data
// and says how to go back; restore --pre-update puts the data from before
// the update back, keeps the newer data aside, and this build starts.
func TestRestorePreUpdateAfterDowngrade(t *testing.T) {
	dir, err := oldThenNew(t, "CREATE TABLE future(x);")
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.Open(dir, database.Options{})
	var nd *database.NewerDataError
	if !errors.As(err, &nd) || !needsPerson(err) {
		t.Fatalf("got %v", err)
	}
	if r := publicReason(err); !strings.Contains(r, "holyarrs restore --pre-update") || !strings.Contains(r, "docker exec <container> /holyarrs restore --pre-update") {
		t.Fatalf("page %q", r)
	}
	code, out := restoreRun(t, "-config", dir, "--list")
	if code != 0 || !strings.Contains(out, "pre-update") {
		t.Fatalf("list %d %q", code, out)
	}
	code, out = restoreRun(t, "-config", dir, "--pre-update")
	if code != 0 {
		t.Fatalf("restore %d %q", code, out)
	}
	v, err := openVersion(t, dir)
	if err != nil || v == 9999 {
		t.Fatalf("after restore: version %d, %v", v, err)
	}
	// Undoable: the newer data is kept aside.
	kept, _ := filepath.Glob(filepath.Join(dir, "backups", "before-restore-*", database.MainFile))
	if len(kept) != 1 || !strings.Contains(out, "before-restore-") {
		t.Fatalf("kept %v, out %q", kept, out)
	}
	s, err := database.Open(filepath.Dir(kept[0]), database.Options{MainMigrations: mainPlus(t, "CREATE TABLE future(x);")})
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := s.Version(); v != 9999 {
		t.Fatalf("kept data version %d", v)
	}
	_ = s.Close()
}

// Scenario 7: an update whose SQL fails is undone and refused with a clear
// page; restore --pre-update still works.
func TestRestoreAfterFailedMigration(t *testing.T) {
	dir, err := oldThenNew(t, "CREATE TABLE future(x); INSERT INTO nope VALUES (1);")
	if !errors.Is(err, database.ErrMigration) || !needsPerson(err) || !strings.Contains(publicReason(err), "restore --pre-update") {
		t.Fatalf("got %v", err)
	}
	before, err := openVersion(t, dir)
	if err != nil {
		t.Fatal(err)
	}
	if code, out := restoreRun(t, "-config", dir, "--pre-update"); code != 0 {
		t.Fatalf("restore %d %q", code, out)
	}
	if v, err := openVersion(t, dir); err != nil || v != before {
		t.Fatalf("after restore %d %v, want %d", v, err, before)
	}
}

// Scenario 10: no restore while HolyArrs holds the config folder.
func TestRestoreRefusedWhileRunning(t *testing.T) {
	dir, err := oldThenNew(t, "CREATE TABLE future(x);")
	if err != nil {
		t.Fatal(err)
	}
	unlock, err := files.LockDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	code, out := restoreRun(t, "-config", dir, "--pre-update")
	if code == 0 || !strings.Contains(out, "running") {
		t.Fatalf("restore %d %q", code, out)
	}
	if _, err := os.Stat(filepath.Join(dir, "restore-staged")); !os.IsNotExist(err) {
		t.Fatal("something was staged")
	}
}

func TestRestoreUsage(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{{"-config", dir}, {"-config", dir, "--list", "--pre-update"}, {"-config", dir, "--pre-update", "x"}} {
		if code, _ := restoreRun(t, args...); code != 2 {
			t.Fatalf("%v: %d", args, code)
		}
	}
	if code, out := restoreRun(t, "-config", dir, "--pre-update"); code != 1 || !strings.Contains(out, "no pre-update backup") {
		t.Fatalf("%d %q", code, out)
	}
	if code, out := restoreRun(t, "-config", dir, "holyarrs-backup-manual-x.zip"); code != 1 || !strings.Contains(out, "no such backup") {
		t.Fatalf("%d %q", code, out)
	}
}
