package database

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

var (
	v1 = fstest.MapFS{"0001_a.sql": {Data: []byte("CREATE TABLE a(x);")}}
	v2 = fstest.MapFS{
		"0001_a.sql": {Data: []byte("CREATE TABLE a(x);")},
		"0002_b.sql": {Data: []byte("CREATE TABLE b(y);")},
	}
)

func openAt(t *testing.T, dir string, ms fstest.MapFS) (*Store, error) {
	t.Helper()
	s, err := Open(dir, Options{MainMigrations: ms})
	if err == nil {
		t.Cleanup(func() { _ = s.Close() })
	}
	return s, err
}

func TestNewerDataIsRefused(t *testing.T) {
	dir := t.TempDir()
	s, err := openAt(t, dir, v2)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	_, err = openAt(t, dir, v1)
	var nd *NewerDataError
	if !errors.As(err, &nd) || nd.Have != 2 || nd.Know != 1 {
		t.Fatalf("got %v", err)
	}
	if !strings.Contains(err.Error(), "this data is from a newer HolyArrs (schema 2, this build knows 1)") {
		t.Fatalf("message %q", err)
	}
}

func TestPreUpdateBackupIsCheckedAndNamed(t *testing.T) {
	dir := t.TempDir()
	s, err := openAt(t, dir, v1)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	backups := filepath.Join(dir, "backups")
	_ = os.MkdirAll(backups, 0o750)
	stale := filepath.Join(backups, "main-pre-update-0001-to-0002-old.db.partial")
	_ = os.WriteFile(stale, []byte("half"), 0o600)
	if _, err := openAt(t, dir, v2); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatal("stale .partial left behind")
	}
	got, _ := filepath.Glob(filepath.Join(backups, "main-pre-update-0001-to-0002-*.db"))
	if len(got) != 1 {
		t.Fatalf("backups %v", got)
	}
	sum, err := os.ReadFile(got[0] + ".sha256")
	want, _ := fileSHA256(got[0])
	if err != nil || !strings.HasPrefix(string(sum), want) {
		t.Fatalf("sha256 file %q %v, want %s", sum, err, want)
	}
}

func TestUpdateRefusedWithoutSpace(t *testing.T) {
	dir := t.TempDir()
	s, err := openAt(t, dir, v1)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	old := freeSpace
	freeSpace = func(string) (uint64, error) { return 100 << 20, nil }
	defer func() { freeSpace = old }()
	_, err = openAt(t, dir, v2)
	if !errors.Is(err, ErrNoSpace) || !strings.Contains(err.Error(), "need ") || !strings.Contains(err.Error(), "have 100.0 MB") {
		t.Fatalf("got %v", err)
	}
}

func TestUpdateRefusedAfterThreeFailedBackups(t *testing.T) {
	dir := t.TempDir()
	s, err := openAt(t, dir, v1)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	old, tries := takeBackup, 0
	takeBackup = func(*writer, string, string, int, int) (string, error) {
		tries++
		return "", errors.New("disk said no")
	}
	defer func() { takeBackup = old }()
	_, err = openAt(t, dir, v2)
	if !errors.Is(err, ErrBackup) || tries != 3 {
		t.Fatalf("got %v after %d tries", err, tries)
	}
	takeBackup = old
	s, err = openAt(t, dir, v1)
	if err != nil {
		t.Fatalf("data changed after the refused update: %v", err)
	}
	if v, _ := s.Version(); v != 1 {
		t.Fatalf("version %d, want 1", v)
	}
}

func TestHumanSize(t *testing.T) {
	for n, want := range map[uint64]string{12: "12 B", 200 << 20: "200.0 MB", 3 << 29: "1.5 GB"} {
		if got := humanSize(n); got != want {
			t.Fatalf("%d: %s, want %s", n, got, want)
		}
	}
}

// tinyDisk makes every disk report one byte free.
func tinyDisk(t *testing.T) {
	old := freeSpace
	freeSpace = func(string) (uint64, error) { return 1, nil }
	t.Cleanup(func() { freeSpace = old })
}

func preUpdateCopies(t *testing.T, dir string) []string {
	t.Helper()
	got, _ := filepath.Glob(filepath.Join(dir, "backups", "*-pre-update-*.db"))
	return got
}

// Scenario 1: a fresh install makes no backup and never checks space.
func TestFreshInstallSkipsBackupAndSpace(t *testing.T) {
	tinyDisk(t)
	dir := t.TempDir()
	called := false
	s, err := Open(dir, Options{MainMigrations: v2, OnUpdating: func() { called = true }})
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	if n := preUpdateCopies(t, dir); len(n) != 0 || called {
		t.Fatalf("copies %v, updating %v", n, called)
	}
}

// Scenario 2: a restart with nothing to migrate makes no backup and starts
// on a nearly full disk.
func TestRestartWithoutMigrationSkipsBackupAndSpace(t *testing.T) {
	dir := t.TempDir()
	s, err := openAt(t, dir, v2)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	tinyDisk(t)
	called := false
	s, err = Open(dir, Options{MainMigrations: v2, OnUpdating: func() { called = true }})
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	if n := preUpdateCopies(t, dir); len(n) != 0 || called {
		t.Fatalf("copies %v, updating %v", n, called)
	}
}

// Scenario 5: a measured shortage refuses and leaves the data byte for
// byte; HOLYARRS_SKIP_SPACE_CHECK=yes lets the update run.
func TestShortDiskRefusesThenSkipEnvRuns(t *testing.T) {
	dir := t.TempDir()
	s, err := openAt(t, dir, v1)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	before, _ := os.ReadFile(filepath.Join(dir, MainFile))
	tinyDisk(t)
	if _, err := openAt(t, dir, v2); !errors.Is(err, ErrNoSpace) {
		t.Fatalf("got %v", err)
	}
	after, _ := os.ReadFile(filepath.Join(dir, MainFile))
	if string(before) != string(after) {
		t.Fatal("the database changed after a refused update")
	}
	t.Setenv(SkipSpaceCheckEnv, "yes")
	s, err = openAt(t, dir, v2)
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := s.Version(); v != 2 {
		t.Fatalf("version %d", v)
	}
}

// Scenario 6: free space that cannot be measured only warns.
func TestUnmeasurableSpaceWarnsAndUpdates(t *testing.T) {
	for _, hook := range []string{"freeSpace", "sameVolume"} {
		t.Run(hook, func(t *testing.T) {
			dir := t.TempDir()
			s, err := openAt(t, dir, v1)
			if err != nil {
				t.Fatal(err)
			}
			_ = s.Close()
			oldF, oldS := freeSpace, sameVolume
			defer func() { freeSpace, sameVolume = oldF, oldS }()
			if hook == "freeSpace" {
				freeSpace = func(string) (uint64, error) { return 0, errors.New("not supported") }
			} else {
				sameVolume = func(string, string) (bool, error) { return false, errors.New("not supported") }
			}
			s, err = openAt(t, dir, v2)
			if err != nil {
				t.Fatal(err)
			}
			if v, _ := s.Version(); v != 2 {
				t.Fatalf("version %d", v)
			}
		})
	}
}

// Scenario 7: a migration that fails half way is undone; the pre-update
// copy is there and the error is one a person is shown.
func TestFailedMigrationIsUndone(t *testing.T) {
	dir := t.TempDir()
	s, err := openAt(t, dir, v1)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	bad := fstest.MapFS{
		"0001_a.sql": v2["0001_a.sql"],
		"0002_b.sql": {Data: []byte("CREATE TABLE b(y); INSERT INTO nope VALUES (1);")},
	}
	updating := false
	_, err = Open(dir, Options{MainMigrations: bad, OnUpdating: func() { updating = true }})
	if !errors.Is(err, ErrMigration) || !updating {
		t.Fatalf("got %v (updating %v)", err, updating)
	}
	if n := preUpdateCopies(t, dir); len(n) != 1 {
		t.Fatalf("copies %v", n)
	}
	s, err = openAt(t, dir, v1)
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := s.Version(); v != 1 {
		t.Fatalf("version %d", v)
	}
	var n int
	if err := s.main.db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name='b'").Scan(&n); err != nil || n != 0 {
		t.Fatalf("table b left behind: %d %v", n, err)
	}
}

// Scenario 12: a backups folder that cannot be made refuses with a clear
// error and leaves the data alone.
func TestUnwritableBackupFolderRefuses(t *testing.T) {
	dir := t.TempDir()
	s, err := openAt(t, dir, v1)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	if err := os.WriteFile(filepath.Join(dir, "backups"), []byte("not a folder"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = openAt(t, dir, v2)
	if !errors.Is(err, ErrBackup) || !strings.Contains(err.Error(), "backups folder") || !strings.Contains(err.Error(), "not changed") {
		t.Fatalf("got %v", err)
	}
	_ = os.Remove(filepath.Join(dir, "backups"))
	s, err = openAt(t, dir, v1)
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := s.Version(); v != 1 {
		t.Fatalf("version %d", v)
	}
}
