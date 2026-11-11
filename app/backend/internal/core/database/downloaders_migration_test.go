package database

import (
	"context"
	"database/sql"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"testing/fstest"
)

func downloaderRows(t *testing.T, db *sql.DB, query string) [][]any {
	t.Helper()
	rows, err := db.Query(query)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	out := [][]any{}
	for rows.Next() {
		v := make([]any, len(cols))
		p := make([]any, len(cols))
		for i := range v {
			p[i] = &v[i]
		}
		if err := rows.Scan(p...); err != nil {
			t.Fatal(err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func oldDownloaderFixture(t *testing.T) (string, [][]any, [][]any) {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(dir, Options{MainMigrations: upTo(t, 40)})
	if err != nil {
		t.Fatal(err)
	}
	err = s.Write(context.Background(), func(tx *sql.Tx) error {
		for _, q := range []string{
			`INSERT INTO secrets(id,created_at,created_by,updated_at,updated_by,kind,value) VALUES(71,'t','{}','t','{}','downloader_secret','v1:synthetic-encrypted-value')`,
			`INSERT INTO downloaders(id,created_at,created_by,updated_at,updated_by,name,adapter,protocol,url,secret_ref,category,priority,path_map,after_import,seed_goal,state,username,allow_local_network,backoff_until,fail_count,last_error,checked_at,removed) VALUES(7,'a','{"kind":"user","id":1}','b','{}','Saved qBittorrent','qbittorrent','torrent','http://owned-fixture',71,'owned',7,'[{"theirs":"/d","ours":"/m"}]','leave','{"ratio":2}','disabled','saved-user',1,'later',3,'safe error','checked',0)`,
			`INSERT INTO downloaders(id,created_at,created_by,updated_at,updated_by,name,adapter,protocol,url,state,removed) VALUES(11,'c','{}','d','{}','Saved SAB','sabnzbd','usenet','http://owned-sab','auth_failed',1)`,
			`INSERT INTO downloads(id,created_at,created_by,updated_at,updated_by,downloader_id,external_id,progress,state,started_by) VALUES(13,'a','{}','b','{}',7,'owned-external-id',0.4,'paused','{}')`,
			`INSERT INTO import_created(tbl,item_id,live) VALUES('downloaders',7,1),('downloaders',11,0)`,
		} {
			if _, err := tx.Exec(q); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	before := downloaderRows(t, s.Read(), "SELECT * FROM downloaders ORDER BY id")
	deps := downloaderRows(t, s.Read(), "SELECT id, created_at, created_by, updated_at, updated_by, downloader_id, release_id, slot_ids, external_id, progress, speed, eta, state, state_reason, files_seen, save_path, started_at, finished_at, started_by FROM downloads ORDER BY id")
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "holyarrs.key"), []byte("synthetic-key-kept-outside-the-database"), 0600); err != nil {
		t.Fatal(err)
	}
	return dir, before, deps
}

func TestOptionalDownloadersMigrationKeepsData(t *testing.T) {
	dir, before, deps := oldDownloaderFixture(t)
	s, err := Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if v, err := version(s.Read()); err != nil || v < 42 {
		t.Fatalf("schema=%d error=%v", v, err)
	}
	got := downloaderRows(t, s.Read(), "SELECT * FROM downloaders ORDER BY id")
	for i := range got { // later migrations add columns at the end; the saved ones stay as they were
		if len(before) > 0 && len(got[i]) > len(before[0]) {
			got[i] = got[i][:len(before[0])]
		}
	}
	if !reflect.DeepEqual(got, before) {
		t.Fatal("saved Downloader row changed during rebuild")
	}
	if got := downloaderRows(t, s.Read(), "SELECT id, created_at, created_by, updated_at, updated_by, downloader_id, release_id, slot_ids, external_id, progress, speed, eta, state, state_reason, files_seen, save_path, started_at, finished_at, started_by FROM downloads ORDER BY id"); !reflect.DeepEqual(got, deps) {
		t.Fatal("dependent Download changed during rebuild")
	}
	var cipher string
	if err := s.Read().QueryRow("SELECT value FROM secrets WHERE id=71").Scan(&cipher); err != nil || cipher != "v1:synthetic-encrypted-value" {
		t.Fatal("vault ciphertext changed")
	}
	key, err := os.ReadFile(filepath.Join(dir, "holyarrs.key"))
	if err != nil || string(key) != "synthetic-key-kept-outside-the-database" {
		t.Fatal("external key changed")
	}
	if got := downloaderRows(t, s.Read(), "SELECT item_id,live FROM import_created WHERE tbl='downloaders' ORDER BY item_id"); !reflect.DeepEqual(got, [][]any{{int64(7), int64(1)}, {int64(11), int64(0)}}) {
		t.Fatal("import ownership changed")
	}
	if got := downloaderRows(t, s.Read(), "PRAGMA foreign_key_check"); len(got) != 0 {
		t.Fatalf("foreign key violations=%v", got)
	}
	var check string
	if err := s.Read().QueryRow("PRAGMA quick_check").Scan(&check); err != nil || check != "ok" {
		t.Fatal("database integrity check failed")
	}
	if got := downloaderRows(t, s.Read(), "PRAGMA foreign_key_list(downloads)"); len(got) == 0 {
		t.Fatal("dependent foreign key removed")
	}
	for _, kind := range []string{"transmission", "nzbget", "deluge", "rtorrent"} {
		err = s.Write(context.Background(), func(tx *sql.Tx) error {
			_, err := tx.Exec(`INSERT INTO downloaders(created_at,created_by,updated_at,updated_by,name,adapter,protocol,url) VALUES('t','{}','t','{}',?,?,?,'http://owned-fixture')`, kind, kind, map[bool]string{true: "usenet", false: "torrent"}[kind == "nzbget"])
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, q := range []string{`UPDATE downloaders SET adapter='unsupported' WHERE id=7`, `UPDATE downloaders SET name='Saved SAB' WHERE id=7`, `UPDATE downloads SET downloader_id=999999 WHERE id=13`} {
		if err := s.Write(context.Background(), func(tx *sql.Tx) error { _, err := tx.Exec(q); return err }); err == nil {
			t.Fatal("constraint lost")
		}
	}
	for _, q := range []string{`UPDATE downloaders SET state='ok' WHERE id=7`, `UPDATE import_created SET live=1 WHERE item_id=11 AND tbl='downloaders'`, `DELETE FROM downloaders WHERE id=11`} {
		if err := s.Write(context.Background(), func(tx *sql.Tx) error { _, err := tx.Exec(q); return err }); err != nil {
			t.Fatal(err)
		}
	}
	if got := downloaderRows(t, s.Read(), "SELECT live FROM import_created WHERE tbl='downloaders' ORDER BY item_id"); !reflect.DeepEqual(got, [][]any{{int64(0)}, {int64(0)}}) {
		t.Fatal("import ownership triggers lost")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var count int
	if err := s.Read().QueryRow("SELECT count(*) FROM schema_migrations WHERE version=42").Scan(&count); err != nil || count != 1 {
		t.Fatal("migration reapplied")
	}
}

func TestOptionalDownloadersMigrationRollsBack(t *testing.T) {
	dir, before, deps := oldDownloaderFixture(t)
	src, err := fs.Sub(embedded, "migrations/main")
	if err != nil {
		t.Fatal(err)
	}
	ms, err := loadMigrations(src)
	if err != nil {
		t.Fatal(err)
	}
	broken := fstest.MapFS{}
	for _, m := range ms {
		q := m.sql
		if m.version == 42 {
			q += "\nINSERT INTO missing_injected_failure_table VALUES(1);"
		}
		broken[m.name] = &fstest.MapFile{Data: []byte(q)}
	}
	if s, err := Open(dir, Options{MainMigrations: broken}); err == nil {
		_ = s.Close()
		t.Fatal("injected migration unexpectedly passed")
	}
	s, err := Open(dir, Options{MainMigrations: upTo(t, 41)})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if got := downloaderRows(t, s.Read(), "SELECT * FROM downloaders ORDER BY id"); !reflect.DeepEqual(got, before) {
		t.Fatal("failed migration changed Downloader")
	}
	if got := downloaderRows(t, s.Read(), "SELECT id, created_at, created_by, updated_at, updated_by, downloader_id, release_id, slot_ids, external_id, progress, speed, eta, state, state_reason, files_seen, save_path, started_at, finished_at, started_by FROM downloads ORDER BY id"); !reflect.DeepEqual(got, deps) {
		t.Fatal("failed migration changed dependencies")
	}
	if v, err := version(s.Read()); err != nil || v != 41 {
		t.Fatalf("failed migration version=%d error=%v", v, err)
	}
	if err := s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE downloads SET downloader_id=999999 WHERE id=13`)
		return err
	}); err == nil {
		t.Fatal("foreign keys not enforced after failed migration")
	}
	// Rollback to schema 40 requires the pre-update backup taken at 40, never an old binary
	// opening this now-schema-41 database.
	backups, err := filepath.Glob(filepath.Join(dir, "backups", "main-pre-update-0040-to-*.db"))
	if err != nil || len(backups) != 1 {
		t.Fatal("pre-upgrade rollback backup missing")
	}
	data, err := os.ReadFile(backups[0])
	if err != nil {
		t.Fatal(err)
	}
	restore := t.TempDir()
	if err := os.WriteFile(filepath.Join(restore, MainFile), data, 0600); err != nil {
		t.Fatal(err)
	}
	old, err := Open(restore, Options{MainMigrations: upTo(t, 40)})
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	if v, err := version(old.Read()); err != nil || v != 40 {
		t.Fatal("restored backup is not schema40")
	}
	if got := downloaderRows(t, old.Read(), "SELECT * FROM downloaders ORDER BY id"); !reflect.DeepEqual(got, before) {
		t.Fatal("rollback backup differs")
	}
}

func TestRebuildMigrationRestoresForeignKeys(t *testing.T) {
	for _, failure := range []string{"statement", "foreign_key", "panic", "canceled"} {
		t.Run(failure, func(t *testing.T) {
			w, err := newWriter(filepath.Join(t.TempDir(), "rebuild.db"), 1024, DefaultWriteCap)
			if err != nil {
				t.Fatal(err)
			}
			defer w.close()
			if err := w.doUncapped(context.Background(), func(tx *sql.Tx) error {
				_, err := tx.Exec(`CREATE TABLE parent(id INTEGER PRIMARY KEY); CREATE TABLE child(parent_id INTEGER REFERENCES parent(id)); INSERT INTO parent VALUES(1); INSERT INTO child VALUES(1);`)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			err = w.doRebuild(ctx, func(tx *sql.Tx) error {
				if _, err := tx.Exec(`DELETE FROM parent`); err != nil {
					return err
				}
				switch failure {
				case "statement":
					_, err := tx.Exec(`INSERT INTO absent_table VALUES(1)`)
					return err
				case "panic":
					panic("synthetic migration failure")
				case "canceled":
					cancel()
				}
				return nil
			})
			if err == nil {
				t.Fatal("broken rebuild committed")
			}
			var enabled, count int
			if err := w.db.QueryRow(`PRAGMA foreign_keys`).Scan(&enabled); err != nil || enabled != 1 {
				t.Fatalf("foreign keys=%d error=%v", enabled, err)
			}
			if err := w.db.QueryRow(`SELECT count(*) FROM parent`).Scan(&count); err != nil || count != 1 {
				t.Fatal("failed rebuild did not roll back")
			}
			if err := w.doUncapped(context.Background(), func(tx *sql.Tx) error { _, err := tx.Exec(`INSERT INTO child VALUES(999)`); return err }); err == nil {
				t.Fatal("writer accepted orphan after failed rebuild")
			}
		})
	}
}
