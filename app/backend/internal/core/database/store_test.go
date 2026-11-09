package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"
)

func openTest(t *testing.T, opts Options) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(dir, opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, dir
}

func TestPragmas(t *testing.T) {
	s, _ := openTest(t, Options{})
	for _, db := range []*sql.DB{s.Read(), s.ReadEvents()} {
		checks := map[string]string{
			"journal_mode": "wal",
			"busy_timeout": "5000",
			"synchronous":  "1",
			"mmap_size":    "0",
			"cache_size":   "-8000",
			"foreign_keys": "1",
		}
		for p, want := range checks {
			var got string
			if err := db.QueryRow("PRAGMA " + p).Scan(&got); err != nil {
				t.Fatalf("%s: %v", p, err)
			}
			if strings.ToLower(got) != want {
				t.Errorf("PRAGMA %s = %s, want %s", p, got, want)
			}
		}
	}
}

// Low-memory mode gives the writers and the readers their own, smaller
// page cache; unset, both keep the 8000 KiB default.
func TestReaderCacheSize(t *testing.T) {
	cacheOf := func(s *Store) (writer, main, events string) {
		_ = s.Write(context.Background(), func(tx *sql.Tx) error { return tx.QueryRow("PRAGMA cache_size").Scan(&writer) })
		_ = s.Read().QueryRow("PRAGMA cache_size").Scan(&main)
		_ = s.ReadEvents().QueryRow("PRAGMA cache_size").Scan(&events)
		return
	}
	s, _ := openTest(t, Options{CacheKiB: 4000, ReaderCacheKiB: 2000})
	if w, m, e := cacheOf(s); w != "-4000" || m != "-2000" || e != "-2000" {
		t.Fatalf("low: writer %s, readers %s %s", w, m, e)
	}
	d, _ := openTest(t, Options{})
	if w, m, e := cacheOf(d); w != "-8000" || m != "-8000" || e != "-8000" {
		t.Fatalf("default: writer %s, readers %s %s", w, m, e)
	}
}

func TestReadPoolIsReadOnly(t *testing.T) {
	s, _ := openTest(t, Options{})
	if _, err := s.Read().Exec("CREATE TABLE nope(x)"); err == nil {
		t.Fatal("read pool accepted a write")
	}
}

func TestConcurrentWrites(t *testing.T) {
	s, _ := openTest(t, Options{})
	ctx := context.Background()
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("CREATE TABLE t(n INTEGER)")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 200)
	for i := range 200 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- s.Write(ctx, func(tx *sql.Tx) error {
				_, err := tx.Exec("INSERT INTO t(n) VALUES (?)", i)
				return err
			})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var n int
	if err := s.Read().QueryRow("SELECT count(*) FROM t").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 200 {
		t.Fatalf("got %d rows, want 200", n)
	}
}

func TestWriteRollsBackOnError(t *testing.T) {
	s, _ := openTest(t, Options{})
	ctx := context.Background()
	_ = s.Write(ctx, func(tx *sql.Tx) error { _, err := tx.Exec("CREATE TABLE t(n)"); return err })
	boom := errors.New("boom")
	err := s.Write(ctx, func(tx *sql.Tx) error {
		_, _ = tx.Exec("INSERT INTO t VALUES (1)")
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("got %v, want boom", err)
	}
	var n int
	_ = s.Read().QueryRow("SELECT count(*) FROM t").Scan(&n)
	if n != 0 {
		t.Fatal("write was not rolled back")
	}
}

func TestEventsWriter(t *testing.T) {
	s, dir := openTest(t, Options{})
	if _, err := os.Stat(filepath.Join(dir, EventsFile)); err != nil {
		t.Fatalf("events file missing: %v", err)
	}
	err := s.WriteEvents(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("CREATE TABLE e(x)")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRefusesNetworkShare(t *testing.T) {
	dir := t.TempDir()
	_, err := Open(dir, Options{NetworkShare: func(string) (bool, string, error) { return true, "nfs", nil }})
	if !errors.Is(err, ErrNetworkShare) {
		t.Fatalf("got %v, want ErrNetworkShare", err)
	}
	if !strings.Contains(err.Error(), "nfs") {
		t.Fatalf("message should name the share type: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, MainFile)); statErr == nil {
		t.Fatal("database file created on a network share")
	}
}

func TestMigrationsAndBackup(t *testing.T) {
	dir := t.TempDir()
	v1 := fstest.MapFS{"0001_a.sql": {Data: []byte("CREATE TABLE a(x);")}}
	s, err := Open(dir, Options{MainMigrations: v1})
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := s.Version(); v != 1 {
		t.Fatalf("version %d, want 1", v)
	}
	_ = s.Write(context.Background(), func(tx *sql.Tx) error { _, err := tx.Exec("INSERT INTO a VALUES (42)"); return err })
	_ = s.Close()
	if b, _ := filepath.Glob(filepath.Join(dir, "backups", "*.db")); len(b) != 0 {
		t.Fatalf("no backup expected for a new database, got %v", b)
	}

	v2 := fstest.MapFS{
		"0001_a.sql": {Data: []byte("CREATE TABLE a(x);")},
		"0002_b.sql": {Data: []byte("CREATE TABLE b(y);")},
	}
	s, err = Open(dir, Options{MainMigrations: v2})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if v, _ := s.Version(); v != 2 {
		t.Fatalf("version %d, want 2", v)
	}
	backups, _ := filepath.Glob(filepath.Join(dir, "backups", "main-pre-update-0001-to-0002-*.db"))
	if len(backups) != 1 {
		t.Fatalf("want one backup before 0002, got %v", backups)
	}
	b, err := sql.Open("sqlite", backups[0])
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	var x int
	if err := b.QueryRow("SELECT x FROM a").Scan(&x); err != nil || x != 42 {
		t.Fatalf("backup content wrong: %v %d", err, x)
	}
	if err := b.QueryRow("SELECT count(*) FROM b").Scan(&x); err == nil {
		t.Fatal("backup should be taken before 0002 ran")
	}
}

func TestBadMigrationNames(t *testing.T) {
	bad := fstest.MapFS{"one.sql": {Data: []byte("SELECT 1;")}}
	if _, err := Open(t.TempDir(), Options{MainMigrations: bad}); err == nil {
		t.Fatal("expected an error for a migration without a number")
	}
}

func TestWriteWaitP95(t *testing.T) {
	s, _ := openTest(t, Options{})
	if d, n := s.WriteWaitP95(); n != 0 && d < 0 {
		t.Fatal("bad start")
	}
	ctx := context.Background()
	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = s.Write(ctx, func(tx *sql.Tx) error { time.Sleep(time.Millisecond); return nil })
		}()
	}
	wg.Wait()
	d, n := s.WriteWaitP95()
	if n < 50 || d <= 0 {
		t.Fatalf("p95 %v over %d writes", d, n)
	}
}

func TestWriteWaitP95Under200ConcurrentWrites(t *testing.T) {
	s, _ := openTest(t, Options{})
	ctx := context.Background()
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("CREATE TABLE load_check (id INTEGER PRIMARY KEY, v TEXT)")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := range 200 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = s.Write(ctx, func(tx *sql.Tx) error {
				_, err := tx.Exec("INSERT INTO load_check (v) VALUES (?)", fmt.Sprint(i))
				return err
			})
		}()
	}
	wg.Wait()
	d, n := s.WriteWaitP95()
	// The race detector makes each write several times slower, and a
	// queued write waits for the whole burst ahead of it, so the pass line
	// scales with it; without it the line is the 100 ms of the design.
	limit := 100 * time.Millisecond
	if raceOn {
		limit *= 10
	}
	t.Logf("p95 writer-queue wait over %d writes: %v (pass line %v)", n, d, limit)
	if d >= limit {
		t.Fatalf("p95 %v, pass line is %v", d, limit)
	}
}

func TestSnapshot(t *testing.T) {
	s, _ := openTest(t, Options{})
	out := t.TempDir()
	m, e, err := s.Snapshot(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{m, e} {
		db, err := sql.Open("sqlite", p)
		if err != nil {
			t.Fatal(err)
		}
		var ok string
		if err := db.QueryRow("PRAGMA integrity_check").Scan(&ok); err != nil || ok != "ok" {
			t.Fatalf("%s: %q %v", p, ok, err)
		}
		db.Close()
	}
	if _, _, err := s.Snapshot(out); err == nil {
		t.Fatal("snapshot over an existing copy should fail")
	}
	if mv, ev := SchemaVersions(); mv < 5 || ev < 1 {
		t.Fatalf("versions %d %d", mv, ev)
	}
}

// A background reader that outlives the server (Explore's genre fill) gets an
// error after Close, never a nil pointer panic; Close twice is safe.
func TestReadAfterCloseErrors(t *testing.T) {
	s, err := Open(t.TempDir(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Read().QueryContext(context.Background(), "SELECT 1"); err == nil {
		t.Fatal("read after Close: want an error")
	}
	if err := s.Write(context.Background(), func(*sql.Tx) error { return nil }); err == nil {
		t.Fatal("write after Close: want an error")
	}
	if err := s.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}
