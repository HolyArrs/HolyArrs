package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/model"
	_ "modernc.org/sqlite"
)

func entry(key, title string) model.Entry {
	data, _ := json.Marshal(map[string]any{"id": key, "title": title, "summary": nil})
	return model.Entry{Key: key, Result: model.Result{Complete: true, Data: data}, FetchedAt: time.Now().UTC().Truncate(time.Microsecond), ExpiresAt: time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond)}
}

func openTest(t *testing.T, opts Options) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(dir, opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, dir
}

func mustPut(t *testing.T, s *Store, e model.Entry) {
	t.Helper()
	if err := s.Put(context.Background(), e); err != nil {
		t.Fatal(err)
	}
}

func TestPersistenceAndUnknowns(t *testing.T) {
	s, dir := openTest(t, Options{})
	e := entry("answer", "synthetic")
	mustPut(t, s, e)
	got, ok, err := s.Get(context.Background(), e.Key)
	if err != nil || !ok || !bytes.Equal(got.Result.Data, e.Result.Data) || !got.FetchedAt.Equal(e.FetchedAt) || !got.ExpiresAt.Equal(e.ExpiresAt) || !got.Result.Complete {
		t.Fatalf("round trip: %+v %v %v", got, ok, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if _, ok, err := reopened.Get(context.Background(), e.Key); err != nil || !ok {
		t.Fatalf("reopen: %v %v", ok, err)
	}
	if _, ok, err := reopened.Get(context.Background(), "missing"); err != nil || ok {
		t.Fatalf("missing: %v %v", ok, err)
	}
}

func TestInvalidReplacementPreservesAnswer(t *testing.T) {
	s, _ := openTest(t, Options{})
	original := entry("answer", "original")
	mustPut(t, s, original)
	for name, replacement := range map[string]model.Entry{
		"incomplete": {Key: original.Key, Result: model.Result{Data: json.RawMessage(`{"id":"answer"}`)}},
		"truncated":  {Key: original.Key, Result: model.Result{Complete: true, Data: json.RawMessage(`{"id":`)}},
		"array":      {Key: original.Key, Result: model.Result{Complete: true, Data: json.RawMessage(`[]`)}},
		"null":       {Key: original.Key, Result: model.Result{Complete: true, Data: json.RawMessage(`null`)}},
		"empty key":  {Result: original.Result, FetchedAt: original.FetchedAt, ExpiresAt: original.ExpiresAt},
	} {
		t.Run(name, func(t *testing.T) {
			if err := s.Put(context.Background(), replacement); err == nil {
				t.Fatal("invalid write accepted")
			}
			got, ok, err := s.Get(context.Background(), original.Key)
			if err != nil || !ok || !bytes.Equal(got.Result.Data, original.Result.Data) {
				t.Fatalf("previous answer changed: %v %v", ok, err)
			}
		})
	}
}

func TestEvictionAndOversizePreserveAuthorization(t *testing.T) {
	s, _ := openTest(t, Options{MaxEntries: 2, MaxBytes: 1200})
	ctx := context.Background()
	if err := s.IssueToken(ctx, "fixture-install-token"); err != nil {
		t.Fatal(err)
	}
	first := entry("first", "first")
	first.FetchedAt = first.FetchedAt.Add(-2 * time.Hour)
	second := entry("second", "second")
	second.FetchedAt = second.FetchedAt.Add(-time.Hour)
	mustPut(t, s, first)
	mustPut(t, s, second)
	mustPut(t, s, entry("third", "third"))
	if _, ok, err := s.Get(ctx, "first"); err != nil || ok {
		t.Fatalf("oldest not evicted: %v %v", ok, err)
	}
	if err := s.Put(ctx, entry("third", strings.Repeat("x", 2000))); err == nil {
		t.Fatal("oversize accepted")
	}
	if _, ok, err := s.Get(ctx, "third"); err != nil || !ok {
		t.Fatalf("oversize destroyed old answer: %v %v", ok, err)
	}
	if ok, err := s.Authorized(ctx, "fixture-install-token"); err != nil || !ok {
		t.Fatalf("cache evicted token: %v %v", ok, err)
	}
}

func TestByteCapacityEvictsWithinTransaction(t *testing.T) {
	s, _ := openTest(t, Options{MaxBytes: 650, MaxEntries: 100})
	first := entry("first", strings.Repeat("a", 250))
	first.FetchedAt = first.FetchedAt.Add(-time.Hour)
	mustPut(t, s, first)
	mustPut(t, s, entry("second", strings.Repeat("b", 250)))
	if _, ok, err := s.Get(context.Background(), "first"); err != nil || ok {
		t.Fatalf("byte capacity not enforced: %v %v", ok, err)
	}
}

func TestFailedWriteRollsBackEvictions(t *testing.T) {
	s, dir := openTest(t, Options{MaxEntries: 1})
	mustPut(t, s, entry("first", "saved"))
	db, err := sql.Open("sqlite", filepath.Join(dir, "cache.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TRIGGER fail_write BEFORE INSERT ON entries BEGIN SELECT RAISE(ABORT, 'fixture disk write error'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.Put(context.Background(), entry("second", "new")); err == nil {
		t.Fatal("injected failure accepted")
	}
	if _, ok, err := s.Get(context.Background(), "first"); err != nil || !ok {
		t.Fatalf("failed write lost old entry: %v %v", ok, err)
	}
}

func TestTokenHashesAndDurableRevocation(t *testing.T) {
	s, dir := openTest(t, Options{})
	ctx := context.Background()
	token := "fixture-only-unique-installation-token-123456789"
	if err := s.IssueToken(ctx, ""); err == nil {
		t.Fatal("empty token accepted")
	}
	if err := s.IssueToken(ctx, token); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.Authorized(ctx, token); err != nil || !ok {
		t.Fatalf("issued: %v %v", ok, err)
	}
	if ok, err := s.Authorized(ctx, token+"wrong"); err != nil || ok {
		t.Fatalf("wrong token: %v %v", ok, err)
	}
	if err := s.RevokeToken(ctx, token); err != nil {
		t.Fatal(err)
	}
	if err := s.IssueToken(ctx, token); err == nil {
		t.Fatal("revoked token reissued")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if ok, err := reopened.Authorized(ctx, token); err != nil || ok {
		t.Fatalf("revocation lost: %v %v", ok, err)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.sqlite*"))
	for _, path := range files {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(b, []byte(token)) {
			t.Fatalf("plaintext token in %s", filepath.Base(path))
		}
	}
}

func TestBackupRestoresCacheAndAuthorization(t *testing.T) {
	s, _ := openTest(t, Options{})
	ctx := context.Background()
	mustPut(t, s, entry("saved", "saved"))
	if err := s.IssueToken(ctx, "fixture-live-token"); err != nil {
		t.Fatal(err)
	}
	if err := s.RevokeToken(ctx, "fixture-revoked-token"); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "backup")
	if err := s.Backup(ctx, dest); err != nil {
		t.Fatal(err)
	}
	if err := s.Backup(ctx, dest); err == nil {
		t.Fatal("backup overwrote existing destination")
	}
	if err := s.Check(ctx); err != nil {
		t.Fatal(err)
	}
	restored, err := Open(dest, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if _, ok, err := restored.Get(ctx, "saved"); err != nil || !ok {
		t.Fatalf("cache restore: %v %v", ok, err)
	}
	if ok, err := restored.Authorized(ctx, "fixture-live-token"); err != nil || !ok {
		t.Fatalf("token restore: %v %v", ok, err)
	}
	if err := restored.IssueToken(ctx, "fixture-revoked-token"); err == nil {
		t.Fatal("revocation missing in backup")
	}
	if err := restored.Check(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestUnknownSchemaRejected(t *testing.T) {
	for _, filename := range []string{"cache.sqlite", "access.sqlite"} {
		t.Run(filename, func(t *testing.T) {
			s, dir := openTest(t, Options{})
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			db, err := sql.Open("sqlite", filepath.Join(dir, filename))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec("PRAGMA user_version=99"); err != nil {
				t.Fatal(err)
			}
			db.Close()
			if opened, err := Open(dir, Options{}); err == nil {
				opened.Close()
				t.Fatal("future schema accepted")
			}
		})
	}
}

func TestFreeSpaceCheckPreservesExistingFiles(t *testing.T) {
	s, dir := openTest(t, Options{})
	mustPut(t, s, entry("saved", "saved"))
	s.Close()
	if opened, err := Open(dir, Options{MinFreeBytes: ^uint64(0)}); err == nil {
		opened.Close()
		t.Fatal("impossible free-space requirement accepted")
	}
	reopened, err := Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if _, ok, err := reopened.Get(context.Background(), "saved"); err != nil || !ok {
		t.Fatalf("free space failure destroyed data: %v %v", ok, err)
	}
}

func TestCanceledOperationsAndConcurrentWrites(t *testing.T) {
	s, _ := openTest(t, Options{MaxEntries: 200})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.Put(ctx, entry("canceled", "canceled")); err == nil {
		t.Fatal("canceled put accepted")
	}
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := s.Put(context.Background(), entry(fmt.Sprint(i), "fixture")); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	if err := s.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertAccounting(t, s, 30)
}

func TestExclusiveStoreOwner(t *testing.T) {
	s, dir := openTest(t, Options{})
	if other, err := Open(dir, Options{}); err == nil {
		other.Close()
		t.Fatal("second store owner accepted")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	reopened.Close()
}

func TestSQLiteFullPreservesReplacementAndEvictions(t *testing.T) {
	s, _ := openTest(t, Options{MaxEntries: 1})
	original := entry("saved", "previous valid answer")
	mustPut(t, s, original)
	var pages int
	if err := s.cache.QueryRow("PRAGMA page_count").Scan(&pages); err != nil {
		t.Fatal(err)
	}
	if _, err := s.cache.Exec(fmt.Sprintf("PRAGMA max_page_count=%d", pages)); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"saved", "new"} {
		if err := s.Put(context.Background(), entry(key, strings.Repeat("x", 128<<10))); err == nil {
			t.Fatal("SQLite physical page cap did not fail write")
		}
		got, ok, err := s.Get(context.Background(), original.Key)
		if err != nil || !ok || !bytes.Equal(got.Result.Data, original.Result.Data) {
			t.Fatalf("SQLITE_FULL lost old answer: %v %v", ok, err)
		}
		assertAccounting(t, s, 1)
	}
}

func TestDiskReserveBlocksWriteBeforeEviction(t *testing.T) {
	s, _ := openTest(t, Options{MaxEntries: 1})
	mustPut(t, s, entry("saved", "saved"))
	s.opts.MinFreeBytes = ^uint64(0)
	if err := s.Put(context.Background(), entry("new", "new")); err == nil {
		t.Fatal("disk reserve ignored")
	}
	if _, ok, err := s.Get(context.Background(), "saved"); err != nil || !ok {
		t.Fatalf("disk reserve failure lost answer: %v %v", ok, err)
	}
}

func TestSQLiteLimitsConfigured(t *testing.T) {
	s, _ := openTest(t, Options{})
	for i, db := range []*sql.DB{s.cache, s.access} {
		var mode string
		if err := db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil || mode != "wal" {
			t.Fatalf("WAL: %s %v", mode, err)
		}
		want := map[string]int64{"cache_size": -2048, "mmap_size": 0, "wal_autocheckpoint": 64, "journal_size_limit": 262144, "synchronous": 2}
		if i == 0 {
			// The answer cache reads through a larger page cache and a map.
			want["cache_size"], want["mmap_size"] = -65536, 2147418112
		}
		for pragma, want := range want {
			var got int64
			if err := db.QueryRow("PRAGMA " + pragma).Scan(&got); err != nil || got != want {
				t.Fatalf("%s=%d want %d: %v", pragma, got, want, err)
			}
		}
		var cap int64
		if err := db.QueryRow("PRAGMA max_page_count").Scan(&cap); err != nil || cap > 1<<20 {
			t.Fatalf("physical page cap %d: %v", cap, err)
		}
		if got := db.Stats().MaxOpenConnections; got != 1 {
			t.Fatalf("max connections %d", got)
		}
	}
}

func TestFailedBackupRemovesPartialDestination(t *testing.T) {
	s, _ := openTest(t, Options{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	dest := filepath.Join(t.TempDir(), "backup")
	if err := s.Backup(ctx, dest); err == nil {
		t.Fatal("canceled backup accepted")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("partial destination remains: %v", err)
	}
}

func TestMismatchedCurrentSchemaRejected(t *testing.T) {
	s, dir := openTest(t, Options{})
	s.Close()
	db, err := sql.Open("sqlite", filepath.Join(dir, "cache.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("DROP TABLE entries"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if opened, err := Open(dir, Options{}); err == nil {
		opened.Close()
		t.Fatal("current version with missing schema accepted")
	}
}

func TestPinnedWALStopsWritesAndRecovers(t *testing.T) {
	s, dir := openTest(t, Options{})
	mustPut(t, s, entry("saved", "original"))
	reader, err := sql.Open("sqlite", filepath.Join(dir, "cache.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	tx, err := reader.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var data []byte
	if err := tx.QueryRow("SELECT data FROM entries WHERE key='saved'").Scan(&data); err != nil {
		t.Fatal(err)
	}
	var last model.Entry
	blocked := false
	for i := 0; i < 20; i++ {
		next := entry("saved", fmt.Sprint(i)+strings.Repeat("x", 1<<20))
		if err := s.Put(context.Background(), next); err != nil {
			blocked = true
			break
		}
		last = next
	}
	if !blocked {
		t.Fatal("pinned reader allowed unbounded WAL growth")
	}
	got, ok, err := s.Get(context.Background(), "saved")
	if err != nil || !ok || !bytes.Equal(got.Result.Data, last.Result.Data) {
		t.Fatalf("WAL failure lost last answer: %v %v", ok, err)
	}
	info, err := os.Stat(filepath.Join(dir, "cache.sqlite-wal"))
	if err != nil || info.Size() > walLimit+4*maxResultBytes {
		t.Fatalf("unbounded WAL size: %v %v", info, err)
	}
	tx.Rollback()
	mustPut(t, s, entry("saved", "recovered"))
	if err := s.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestBackupsDuringConcurrentWrites(t *testing.T) {
	s, _ := openTest(t, Options{})
	ctx := context.Background()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			key := fmt.Sprintf("concurrent-%d", i)
			if err := s.Put(ctx, entry(key, key)); err != nil {
				t.Error(err)
				return
			}
			if err := s.IssueToken(ctx, "fixture-"+key); err != nil {
				t.Error(err)
				return
			}
		}
	}()
	for i := 0; i < 3; i++ {
		dest := filepath.Join(t.TempDir(), "backup")
		if err := s.Backup(ctx, dest); err != nil {
			t.Fatal(err)
		}
		restored, err := Open(dest, Options{})
		if err != nil {
			t.Fatal(err)
		}
		if err := restored.Check(ctx); err != nil {
			t.Fatal(err)
		}
		// A credential is issued after its answer: the paired snapshot cannot
		// contain that credential without containing the preceding answer.
		for j := 0; j < 20; j++ {
			key := fmt.Sprintf("concurrent-%d", j)
			authorized, err := restored.Authorized(ctx, "fixture-"+key)
			if err != nil {
				t.Fatal(err)
			}
			_, saved, err := restored.Get(ctx, key)
			if err != nil || authorized && !saved {
				t.Fatalf("inconsistent paired snapshot: token %v answer %v err %v", authorized, saved, err)
			}
		}
		restored.Close()
	}
	wg.Wait()
}

func TestReadyHandlesCanceledAndClosedStorage(t *testing.T) {
	s, _ := openTest(t, Options{})
	if err := s.Ready(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.Ready(ctx); err == nil {
		t.Fatal("canceled readiness succeeded")
	}
	s.cache.Close()
	if err := s.Ready(context.Background()); err == nil {
		t.Fatal("closed storage reported ready")
	}
}

func TestReadyHasBoundedConnectionWait(t *testing.T) {
	s, _ := openTest(t, Options{})
	conn, err := s.cache.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	started := time.Now()
	if err := s.Ready(context.Background()); err == nil {
		t.Fatal("unavailable connection reported ready")
	}
	if time.Since(started) > time.Second {
		t.Fatal("readiness exceeded bounded connection wait")
	}
}

func assertAccounting(t *testing.T, s *Store, wantCount int) {
	t.Helper()
	if err := s.lock(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer s.unlock()
	var count int
	var used int64
	if err := s.cache.QueryRow("SELECT COUNT(*),COALESCE(SUM(size),0) FROM entries").Scan(&count, &used); err != nil {
		t.Fatal(err)
	}
	if count != wantCount || s.cacheEntries != count || s.cacheBytes != used {
		t.Fatalf("accounting: database %d/%d memory %d/%d expected count %d", count, used, s.cacheEntries, s.cacheBytes, wantCount)
	}
}

func TestAccountingReplacementEvictionAndReopen(t *testing.T) {
	s, dir := openTest(t, Options{MaxEntries: 2, MaxBytes: 500})
	assertAccounting(t, s, 0)
	mustPut(t, s, entry("first", "small"))
	mustPut(t, s, entry("second", "small"))
	assertAccounting(t, s, 2)
	mustPut(t, s, entry("first", strings.Repeat("x", 200)))
	assertAccounting(t, s, 2)
	mustPut(t, s, entry("first", "small again"))
	assertAccounting(t, s, 2)
	mustPut(t, s, entry("third", "count eviction"))
	assertAccounting(t, s, 2)
	if _, ok, err := s.Get(context.Background(), "second"); err != nil || ok {
		t.Fatalf("oldest entry survived count eviction: %v %v", ok, err)
	}
	mustPut(t, s, entry("fourth", strings.Repeat("x", 350)))
	assertAccounting(t, s, 1)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir, Options{MaxEntries: 2, MaxBytes: 500})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	assertAccounting(t, reopened, 1)
	mustPut(t, reopened, entry("fourth", "smaller after reopen"))
	mustPut(t, reopened, entry("fifth", "new after reopen"))
	assertAccounting(t, reopened, 2)
}

func TestAccountingFailedCommitPreservesReplacementAndEvictions(t *testing.T) {
	s, _ := openTest(t, Options{MaxEntries: 1})
	original := entry("saved", "previous valid answer")
	mustPut(t, s, original)
	// A deferred constraint lets INSERT succeed and rejects COMMIT itself.
	if _, err := s.cache.Exec(`PRAGMA foreign_keys=ON;
		CREATE TABLE commit_parent(id INTEGER PRIMARY KEY);
		CREATE TABLE commit_child(id INTEGER REFERENCES commit_parent(id) DEFERRABLE INITIALLY DEFERRED);
		CREATE TRIGGER reject_commit AFTER INSERT ON entries BEGIN INSERT INTO commit_child VALUES(1); END;`); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"saved", "new"} {
		if err := s.Put(context.Background(), entry(key, "must roll back")); err == nil || err.Error() != "cache commit failed" {
			t.Fatalf("expected COMMIT failure, got %v", err)
		}
		assertAccounting(t, s, 1)
		got, ok, err := s.Get(context.Background(), original.Key)
		if err != nil || !ok || !bytes.Equal(got.Result.Data, original.Result.Data) {
			t.Fatalf("failed commit changed saved answer: %v %v", ok, err)
		}
	}
	if _, err := s.cache.Exec("DROP TRIGGER reject_commit"); err != nil {
		t.Fatal(err)
	}
	mustPut(t, s, entry("recovered", "write after failed commit"))
	assertAccounting(t, s, 1)
}
