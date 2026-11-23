package database

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/files"
)

// mountAs is a MountType hook: every folder reports this mount type.
func mountAs(fstype string) func(string) string { return func(string) string { return fstype } }

// unraidShare is the Options of a config folder on an Unraid user share.
func unraidShare() Options {
	return Options{NetworkShare: shareAs("fuse"), MountType: mountAs("fuse.shfs")}
}

func journalOf(t *testing.T, db *sql.DB) string {
	t.Helper()
	var m string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&m); err != nil {
		t.Fatal(err)
	}
	return strings.ToLower(m)
}

func syncOf(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow("PRAGMA synchronous").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func put512(t *testing.T, s *Store, v string) {
	t.Helper()
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		if _, err := tx.Exec("CREATE TABLE IF NOT EXISTS t512(v TEXT)"); err != nil {
			return err
		}
		_, err := tx.Exec("INSERT INTO t512(v) VALUES (?)", v)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func count512(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow("SELECT count(*) FROM t512").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

// An Unraid user share starts without any setting, with a rollback journal
// and full sync on both files.
func TestUnraidShareStartsWithRollbackJournal(t *testing.T) {
	t.Setenv(files.EnvAllowFuseFolder, "")
	dir := t.TempDir()
	s, err := Open(dir, unraidShare())
	if err != nil {
		t.Fatalf("Unraid user share refused: %v", err)
	}
	defer s.Close()
	if !s.RollbackJournal() {
		t.Fatal("RollbackJournal() = false on an Unraid share")
	}
	for _, w := range []*writer{s.main, s.events} {
		if m := journalOf(t, w.db); m != "delete" {
			t.Errorf("journal_mode = %s, want delete", m)
		}
		if n := syncOf(t, w.db); n != 2 {
			t.Errorf("synchronous = %d, want 2 (FULL)", n)
		}
	}
	put512(t, s, "a")
	if n := count512(t, s.Read()); n != 1 {
		t.Fatalf("read back %d rows", n)
	}
	for _, f := range []string{MainFile, EventsFile} {
		if exists(filepath.Join(dir, f+"-wal")) || exists(filepath.Join(dir, f+"-shm")) {
			t.Errorf("%s has WAL files on an Unraid share", f)
		}
	}
}

// Only fuse.shfs is adapted to: mergerfs, an unknown FUSE kind and a share
// that only claims the shfs mount type stay refused; the override still
// lifts mergerfs (in WAL, as before).
func TestOnlyUnraidShareIsAdapted(t *testing.T) {
	t.Setenv(files.EnvAllowFuseFolder, "")
	cases := []Options{
		{NetworkShare: shareAs("fuse"), MountType: mountAs("fuse.mergerfs")},
		{NetworkShare: shareAs("fuse"), MountType: mountAs("fuse.rclone")},
		{NetworkShare: shareAs("fuse"), MountType: mountAs("")},
		{NetworkShare: shareAs("nfs"), MountType: mountAs("fuse.shfs")},
		{NetworkShare: shareAs("smb"), MountType: mountAs("fuse.shfs")},
	}
	for _, o := range cases {
		if _, err := Open(t.TempDir(), o); !errors.Is(err, ErrNetworkShare) {
			t.Errorf("%+v: want refused, got %v", o, err)
		}
	}
	t.Setenv(files.EnvAllowFuseFolder, "1")
	s, err := Open(t.TempDir(), Options{NetworkShare: shareAs("fuse"), MountType: mountAs("fuse.mergerfs")})
	if err != nil {
		t.Fatalf("override set, mergerfs refused: %v", err)
	}
	defer s.Close()
	if s.RollbackJournal() || journalOf(t, s.main.db) != "wal" {
		t.Fatal("mergerfs with the override must stay in WAL")
	}
	if _, err := Open(t.TempDir(), Options{NetworkShare: shareAs("nfs"), MountType: mountAs("nfs4")}); !errors.Is(err, ErrNetworkShare) {
		t.Fatalf("override lifted NFS: %v", err)
	}
}

// The override set on an Unraid share changes nothing: it is adapted.
func TestUnraidShareWithOverrideStillAdapts(t *testing.T) {
	t.Setenv(files.EnvAllowFuseFolder, "1")
	s, err := Open(t.TempDir(), unraidShare())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if journalOf(t, s.main.db) != "delete" {
		t.Fatal("Unraid share with the override is not in rollback mode")
	}
}

// copyFile copies src to dst, if src is there.
func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	in, err := os.Open(src)
	if err != nil {
		return
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
	_ = out.Close()
}

// A database made in WAL on a pool, then moved onto an Unraid share with
// rows still only in its -wal file (as after a crash or a copy of a running
// container), keeps every row and ends as one file in DELETE mode. Moving it
// back off the share returns it to WAL.
func TestJournalSwitchesKeepData(t *testing.T) {
	t.Setenv(files.EnvAllowFuseFolder, "")
	pool := t.TempDir()
	s, err := Open(pool, Options{NetworkShare: noShare})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		put512(t, s, "row")
	}
	// Copy while open: the newest rows are only in the -wal file.
	share := t.TempDir()
	for _, f := range []string{MainFile, EventsFile} {
		for _, suf := range []string{"", "-wal"} {
			copyFile(t, filepath.Join(pool, f+suf), filepath.Join(share, f+suf))
		}
	}
	if !exists(filepath.Join(share, MainFile+"-wal")) {
		t.Fatal("test setup: no -wal file copied")
	}
	_ = s.Close()

	s, err = Open(share, unraidShare())
	if err != nil {
		t.Fatalf("open on the share: %v", err)
	}
	if m := journalOf(t, s.main.db); m != "delete" {
		t.Fatalf("journal_mode = %s after the move, want delete", m)
	}
	if n := count512(t, s.Read()); n != 50 {
		t.Fatalf("rows after WAL to DELETE: %d, want 50", n)
	}
	put512(t, s, "on share")
	_ = s.Close()
	for _, f := range []string{MainFile, EventsFile} {
		for _, suf := range []string{"-wal", "-shm", "-journal"} {
			if exists(filepath.Join(share, f+suf)) {
				t.Errorf("%s%s left on the share", f, suf)
			}
		}
	}

	back := t.TempDir()
	for _, f := range []string{MainFile, EventsFile} {
		copyFile(t, filepath.Join(share, f), filepath.Join(back, f))
	}
	s, err = Open(back, Options{NetworkShare: noShare})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if m := journalOf(t, s.main.db); m != "wal" {
		t.Fatalf("journal_mode = %s off the share, want wal", m)
	}
	if n := count512(t, s.Read()); n != 51 {
		t.Fatalf("rows after DELETE to WAL: %d, want 51", n)
	}
}

// A reader never sets the journal mode: OpenReadOnly (doctor, secrets)
// next to a database in rollback mode leaves it in rollback mode.
func TestReadOnlyOpenKeepsRollbackJournal(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, unraidShare())
	if err != nil {
		t.Fatal(err)
	}
	put512(t, s, "a")
	_ = s.Close()
	ro, err := OpenReadOnly(dir)
	if err != nil {
		t.Fatal(err)
	}
	if n := count512(t, ro); n != 1 {
		t.Fatalf("rows %d", n)
	}
	if m := journalOf(t, ro); m != "delete" {
		t.Fatalf("read-only open changed the journal to %s", m)
	}
	_ = ro.Close()
	if exists(filepath.Join(dir, MainFile+"-wal")) {
		t.Fatal("read-only open made a -wal file")
	}
}

// In rollback mode reads and writes still run side by side through the
// pools (readers wait on the busy timeout instead of failing).
func TestRollbackJournalReadsDuringWrites(t *testing.T) {
	s, err := Open(t.TempDir(), unraidShare())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	put512(t, s, "seed")
	done := make(chan error, 1)
	go func() {
		for i := 0; i < 100; i++ {
			if err := s.Write(context.Background(), func(tx *sql.Tx) error {
				_, err := tx.Exec("INSERT INTO t512(v) VALUES ('w')")
				return err
			}); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	for i := 0; i < 100; i++ {
		var n int
		if err := s.Read().QueryRow("SELECT count(*) FROM t512").Scan(&n); err != nil {
			t.Fatalf("read during writes: %v", err)
		}
	}
	if err := <-done; err != nil {
		t.Fatalf("write during reads: %v", err)
	}
	if n := count512(t, s.Read()); n != 101 {
		t.Fatalf("rows %d, want 101", n)
	}
}

// A snapshot (what backups use) taken on an Unraid share is one
// self-contained file: it opens alone and holds every row.
func TestSnapshotOnUnraidShareIsOneFile(t *testing.T) {
	s, err := Open(t.TempDir(), unraidShare())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	put512(t, s, "a")
	put512(t, s, "b")
	out := t.TempDir()
	mainCopy, evCopy, err := s.Snapshot(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{mainCopy, evCopy} {
		for _, suf := range []string{"-wal", "-shm", "-journal"} {
			if exists(p + suf) {
				t.Errorf("snapshot left %s%s", filepath.Base(p), suf)
			}
		}
	}
	db, err := sql.Open("sqlite", fileURI(mainCopy)+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if n := count512(t, db); n != 2 {
		t.Fatalf("snapshot rows %d, want 2", n)
	}
}
