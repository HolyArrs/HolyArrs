package database

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	_ "modernc.org/sqlite"
)

// Release proof B2: a config folder made by an older build, holding real
// looking data, opened by this build.

// mainUpTo is the embedded main migration set cut at version upTo, plus
// any extra files.
func mainUpTo(t *testing.T, upTo int, extra fstest.MapFS) fstest.MapFS {
	t.Helper()
	src, _ := fs.Sub(embedded, "migrations/main")
	ms, err := loadMigrations(src)
	if err != nil {
		t.Fatal(err)
	}
	out := fstest.MapFS{}
	for _, m := range ms {
		if m.version <= upTo {
			out[m.name] = &fstest.MapFile{Data: []byte(m.sql)}
		}
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

func latestMain(t *testing.T) int {
	t.Helper()
	src, _ := fs.Sub(embedded, "migrations/main")
	ms, err := loadMigrations(src)
	if err != nil {
		t.Fatal(err)
	}
	return ms[len(ms)-1].version
}

// mainBack is the main migration version n steps before the latest one.
// Numbers may skip, so latest-n is not always a real version.
func mainBack(t *testing.T, n int) int {
	t.Helper()
	src, _ := fs.Sub(embedded, "migrations/main")
	ms, err := loadMigrations(src)
	if err != nil {
		t.Fatal(err)
	}
	return ms[len(ms)-1-n].version
}

const (
	ts = "'2026-10-01T00:00:00.000000000Z'"
	by = `'{"kind":"system"}'`
)

// seedSQL fills the tables a person cares about: people, a sign-in,
// settings, a folder, titles, a downloader and a download half way.
var seedSQL = []string{
	`INSERT INTO users (id, created_at, created_by, updated_at, updated_by, name, role, password_hash) VALUES
		(1, ` + ts + `, ` + by + `, ` + ts + `, ` + by + `, 'maya', 'owner', '$argon2id$v=19$m=65536,t=3,p=2$c2FsdA$aGFzaA'),
		(2, ` + ts + `, ` + by + `, ` + ts + `, ` + by + `, 'sam', 'member', '$argon2id$v=19$m=65536,t=3,p=2$c2FsdA$aGFzaB')`,
	`INSERT INTO sessions (hash, user_id, csrf, created_at, rotated_at, last_seen_at, expires_at, user_agent) VALUES
		('sess-hash-1', 1, 'csrf-1', ` + ts + `, ` + ts + `, ` + ts + `, '2036-10-01T00:00:00.000000000Z', 'Firefox')`,
	`INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES
		('this_app', 'proof_setting', 'kept', ` + ts + `, ` + by + `, ` + ts + `, ` + by + `)`,
	`INSERT INTO library_folders (id, created_at, created_by, updated_at, updated_by, path, kind, label) VALUES
		(1, ` + ts + `, ` + by + `, ` + ts + `, ` + by + `, '/data/movies', 'movie', 'Movies'),
		(2, ` + ts + `, ` + by + `, ` + ts + `, ` + by + `, '/data/shows', 'show', 'Shows')`,
	`INSERT INTO titles (id, created_at, created_by, updated_at, updated_by, kind, name, sort_name, year, metadata_source, why_added) VALUES
		(1, ` + ts + `, ` + by + `, ` + ts + `, ` + by + `, 'movie', 'Arrival', 'arrival', 2016, 'tmdb', 'by_hand'),
		(2, ` + ts + `, ` + by + `, ` + ts + `, ` + by + `, 'show', 'Severance', 'severance', 2022, 'tmdb', 'by_hand')`,
	`INSERT INTO downloaders (id, created_at, created_by, updated_at, updated_by, name, adapter, protocol, url) VALUES
		(1, ` + ts + `, ` + by + `, ` + ts + `, ` + by + `, 'qBit', 'qbittorrent', 'torrent', 'http://10.0.0.5:8080')`,
	`INSERT INTO downloads (id, created_at, created_by, updated_at, updated_by, downloader_id, external_id, progress, state, save_path, started_by) VALUES
		(1, ` + ts + `, ` + by + `, ` + ts + `, ` + by + `, 1, 'abcdef0123456789', 0.42, 'downloading', '/downloads/arrival', ` + by + `)`,
}

// kept are checks that must answer the same before and after the update.
var kept = []struct{ q, want string }{
	{"SELECT group_concat(name || ':' || role, ',') FROM (SELECT * FROM users ORDER BY id)", "maya:owner,sam:member"},
	{"SELECT password_hash FROM users WHERE id = 1", "$argon2id$v=19$m=65536,t=3,p=2$c2FsdA$aGFzaA"},
	{"SELECT user_id || ':' || csrf FROM sessions WHERE hash = 'sess-hash-1'", "1:csrf-1"},
	{"SELECT value FROM settings WHERE grp = 'this_app' AND key = 'proof_setting'", "kept"},
	{"SELECT group_concat(path, ',') FROM (SELECT * FROM library_folders ORDER BY id)", "/data/movies,/data/shows"},
	{"SELECT group_concat(name || ' ' || year, ',') FROM (SELECT * FROM titles ORDER BY id)", "Arrival 2016,Severance 2022"},
	{"SELECT name || ' ' || url FROM downloaders WHERE id = 1", "qBit http://10.0.0.5:8080"},
	{"SELECT external_id || ' ' || state || ' ' || progress || ' ' || save_path FROM downloads WHERE id = 1", "abcdef0123456789 downloading 0.42 /downloads/arrival"},
}

func checkKept(t *testing.T, db *sql.DB, where string) {
	t.Helper()
	for _, k := range kept {
		var got sql.NullString
		if err := db.QueryRow(k.q).Scan(&got); err != nil || got.String != k.want {
			t.Errorf("%s: %s = %q (%v), want %q", where, k.q, got.String, err, k.want)
		}
	}
}

// oldFolder makes a config folder at main schema upTo with the seed data
// and one history line in the events database.
func oldFolder(t *testing.T, upTo int) string {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(dir, Options{MainMigrations: mainUpTo(t, upTo, nil)})
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := s.Version(); v != upTo {
		t.Fatalf("old version %d", v)
	}
	ctx := context.Background()
	err = s.Write(ctx, func(tx *sql.Tx) error {
		for _, q := range seedSQL {
			if _, err := tx.Exec(q); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("seed at %d: %v", upTo, err)
	}
	_, err = s.events.db.Exec(`INSERT INTO events (at, actor, kind, title_id, download_id, summary, created_at, created_by, updated_at, updated_by)
		VALUES (` + ts + `, ` + by + `, 'download.grabbed', 1, 1, 'Arrival sent to qBit', ` + ts + `, ` + by + `, ` + ts + `, ` + by + `)`)
	if err != nil {
		t.Fatal(err)
	}
	checkKept(t, s.main.db, "before")
	_ = s.Close()
	return dir
}

func historyLine(t *testing.T, s *Store) string {
	t.Helper()
	var got string
	_ = s.events.db.QueryRow("SELECT summary FROM events WHERE kind = 'download.grabbed'").Scan(&got)
	return got
}

// TestUpgradeFromOlderSchemas opens folders made at three older schema
// points with this build: every migration runs once, the data is kept,
// one checked pre-update backup is written, and a second start runs none.
func TestUpgradeFromOlderSchemas(t *testing.T) {
	latest := latestMain(t)
	for _, from := range []int{40, 90, mainBack(t, 1)} {
		t.Run("from_"+strconv.Itoa(from), func(t *testing.T) {
			dir := oldFolder(t, from)
			updating := 0
			s, err := Open(dir, Options{OnUpdating: func() { updating++ }})
			if err != nil {
				t.Fatalf("upgrade from %d: %v", from, err)
			}
			t.Cleanup(func() { _ = s.Close() })
			if v, _ := s.Version(); v != latest || updating != 1 {
				t.Fatalf("version %d (want %d), updating called %d", v, latest, updating)
			}
			var rows, distinct int
			_ = s.main.db.QueryRow("SELECT count(*), count(DISTINCT version) FROM schema_migrations").Scan(&rows, &distinct)
			src, _ := fs.Sub(embedded, "migrations/main")
			all, _ := loadMigrations(src)
			if rows != len(all) || distinct != len(all) {
				t.Fatalf("schema_migrations rows %d distinct %d, want %d", rows, distinct, len(all))
			}
			checkKept(t, s.main.db, "after")
			if h := historyLine(t, s); h != "Arrival sent to qBit" {
				t.Fatalf("history %q", h)
			}
			copies := preUpdateCopies(t, dir)
			if len(copies) != 1 || !strings.Contains(copies[0], "main-pre-update-") {
				t.Fatalf("pre-update copies %v", copies)
			}
			if err := CheckBackupSum(copies[0]); err != nil {
				t.Fatalf("backup sum: %v", err)
			}
			// The backup is the old data, whole.
			b, err := sql.Open("sqlite", "file:"+copies[0]+"?mode=ro")
			if err != nil {
				t.Fatal(err)
			}
			if v, _ := SchemaVersion(b); v != from {
				t.Fatalf("backup version %d, want %d", v, from)
			}
			checkKept(t, b, "backup")
			_ = b.Close()
			_ = s.Close()

			// Second start: nothing to run, no new backup.
			s, err = Open(dir, Options{OnUpdating: func() { updating++ }})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = s.Close() })
			if updating != 1 || len(preUpdateCopies(t, dir)) != 1 {
				t.Fatalf("second start updated again (%d) or copied (%v)", updating, preUpdateCopies(t, dir))
			}
			checkKept(t, s.main.db, "restart")
		})
	}
}

// TestRealUpgradeWithFailingMigration: a broken step on top of the real
// set refuses to start with the update error, the pre-update backup holds
// the old data, and the failed step left nothing behind.
func TestRealUpgradeWithFailingMigration(t *testing.T) {
	latest := latestMain(t)
	dir := oldFolder(t, 90)
	bad := mainUpTo(t, latest, fstest.MapFS{
		"9999_broken.sql": {Data: []byte("CREATE TABLE proof_half(x); UPDATE users SET name = 'gone'; INSERT INTO no_such_table VALUES (1);")},
	})
	_, err := Open(dir, Options{MainMigrations: bad})
	if !errors.Is(err, ErrMigration) || !strings.Contains(err.Error(), "9999_broken.sql") || !strings.Contains(err.Error(), "undone") {
		t.Fatalf("got %v", err)
	}
	copies := preUpdateCopies(t, dir)
	if len(copies) != 1 {
		t.Fatalf("copies %v", copies)
	}
	if err := CheckBackupSum(copies[0]); err != nil {
		t.Fatal(err)
	}
	b, err := sql.Open("sqlite", "file:"+copies[0]+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	checkKept(t, b, "backup")
	_ = b.Close()
	// The good steps before it stay; the broken one is undone, so this
	// build (without the broken step) starts on the same folder.
	s, err := Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if v, _ := s.Version(); v != latest {
		t.Fatalf("version %d", v)
	}
	checkKept(t, s.main.db, "after failed step")
	var n int
	_ = s.main.db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name = 'proof_half'").Scan(&n)
	if n != 0 {
		t.Fatal("half of the broken step is left")
	}
}

// TestRealDowngradeIsRefused: a folder this build updated, opened by an
// older build, is refused with a plain message and left as it was.
func TestRealDowngradeIsRefused(t *testing.T) {
	latest := latestMain(t)
	dir := oldFolder(t, 90)
	s, err := Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	older := mainBack(t, 2)
	_, err = Open(dir, Options{MainMigrations: mainUpTo(t, older, nil)})
	var nd *NewerDataError
	if !errors.As(err, &nd) || nd.Have != latest || nd.Know != older || !strings.Contains(err.Error(), "newer HolyArrs") {
		t.Fatalf("got %v", err)
	}
	s, err = Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	checkKept(t, s.main.db, "after refused downgrade")
}
