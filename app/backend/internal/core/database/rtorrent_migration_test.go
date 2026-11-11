package database

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// 0106 rebuilds downloaders to allow the rtorrent adapter and keeps every
// Downloader, its later columns, the rows that point at it and both
// import_created triggers.
func TestDownloadersKeptByRTorrentMigration(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	old, err := Open(dir, Options{MainMigrations: upTo(t, 103)})
	if err != nil {
		t.Fatal(err)
	}
	err = old.Write(ctx, func(tx *sql.Tx) error {
		for _, q := range []string{
			`INSERT INTO secrets(id,created_at,created_by,updated_at,updated_by,kind,value) VALUES(71,'t','{}','t','{}','downloader_secret','v1:synthetic-encrypted-value')`,
			`INSERT INTO downloaders(id,created_at,created_by,updated_at,updated_by,name,adapter,protocol,url,secret_ref,category,priority,path_map,after_import,seed_goal,state,username,allow_local_network,backoff_until,fail_count,last_error,checked_at,removed,writable,clear_finished,tags,category_movies,category_shows,category_music) VALUES(7,'a','{"kind":"user","id":1}','b','{}','Saved Deluge','deluge','torrent','http://owned-fixture',71,'owned',7,'[{"theirs":"/d","ours":"/m"}]','remove','{"ratio":2}','disabled','saved-user',1,'later',3,'safe error','checked',0,0,0,'["4k"]','films','tv','tunes')`,
			`INSERT INTO downloaders(id,created_at,created_by,updated_at,updated_by,name,adapter,protocol,url,state,removed) VALUES(11,'c','{}','d','{}','Saved SAB','sabnzbd','usenet','http://owned-sab','auth_failed',1)`,
			`INSERT INTO downloads(id,created_at,created_by,updated_at,updated_by,downloader_id,external_id,progress,state,started_by) VALUES(13,'a','{}','b','{}',7,'owned-external-id',0.4,'paused','{}')`,
			`INSERT INTO downloader_attempts(id,downloader_id,release_id,slot_id,marker,category,state,created_at,updated_at) VALUES(5,7,1,2,'owned-marker','owned','pending','a','b')`,
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
	cols := downloaderRows(t, old.Read(), "SELECT name, type, \"notnull\", dflt_value, pk FROM pragma_table_info('downloaders') ORDER BY cid")
	before := downloaderRows(t, old.Read(), "SELECT * FROM downloaders ORDER BY id")
	// Later migrations may add columns to downloads, so compare only the
	// columns this schema had.
	var depCols []string
	for _, r := range downloaderRows(t, old.Read(), "SELECT name FROM pragma_table_info('downloads') ORDER BY cid") {
		depCols = append(depCols, `"`+fmt.Sprint(r[0])+`"`)
	}
	depQuery := "SELECT " + strings.Join(depCols, ", ") + " FROM downloads ORDER BY id"
	deps := downloaderRows(t, old.Read(), depQuery)
	attempts := downloaderRows(t, old.Read(), "SELECT * FROM downloader_attempts ORDER BY id")
	_ = old.Close()

	s, err := Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if got := downloaderRows(t, s.Read(), "SELECT name, type, \"notnull\", dflt_value, pk FROM pragma_table_info('downloaders') ORDER BY cid"); !reflect.DeepEqual(got, cols) {
		t.Fatalf("columns changed: %v", got)
	}
	if got := downloaderRows(t, s.Read(), "SELECT * FROM downloaders ORDER BY id"); !reflect.DeepEqual(got, before) {
		t.Fatal("saved Downloader row changed during rebuild")
	}
	if got := downloaderRows(t, s.Read(), depQuery); !reflect.DeepEqual(got, deps) {
		t.Fatal("dependent Download changed during rebuild")
	}
	if got := downloaderRows(t, s.Read(), "SELECT * FROM downloader_attempts ORDER BY id"); !reflect.DeepEqual(got, attempts) {
		t.Fatal("Downloader attempt changed during rebuild")
	}
	if got := downloaderRows(t, s.Read(), "PRAGMA foreign_key_check"); len(got) != 0 {
		t.Fatalf("foreign key violations=%v", got)
	}
	var check string
	if err := s.Read().QueryRow("PRAGMA quick_check").Scan(&check); err != nil || check != "ok" {
		t.Fatal("database integrity check failed")
	}
	for _, tbl := range []string{"downloads", "downloader_attempts"} {
		if got := downloaderRows(t, s.Read(), "SELECT \"table\" FROM pragma_foreign_key_list('"+tbl+"') WHERE \"table\"='downloaders'"); len(got) == 0 {
			t.Fatalf("%s lost its foreign key", tbl)
		}
	}
	err = s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO downloaders(created_at,created_by,updated_at,updated_by,name,adapter,protocol,url) VALUES('t','{}','t','{}','rTorrent','rtorrent','torrent','http://owned-fixture:8000/RPC2')`)
		return err
	})
	if err != nil {
		t.Fatalf("rtorrent refused: %v", err)
	}
	for _, q := range []string{
		`INSERT INTO downloaders(created_at,created_by,updated_at,updated_by,name,adapter,protocol,url) VALUES('t','{}','t','{}','Odd','unsupported','torrent','http://x')`,
		`UPDATE downloaders SET tags='not json' WHERE id=7`,
		`UPDATE downloaders SET name='Saved SAB' WHERE id=7`,
		`UPDATE downloads SET downloader_id=999999 WHERE id=13`,
	} {
		if err := s.Write(ctx, func(tx *sql.Tx) error { _, err := tx.Exec(q); return err }); err == nil {
			t.Fatalf("constraint lost: %s", q)
		}
	}
	for _, q := range []string{`UPDATE downloaders SET state='ok' WHERE id=7`, `UPDATE import_created SET live=1 WHERE item_id=11 AND tbl='downloaders'`, `DELETE FROM downloaders WHERE id=11`} {
		if err := s.Write(ctx, func(tx *sql.Tx) error { _, err := tx.Exec(q); return err }); err != nil {
			t.Fatal(err)
		}
	}
	if got := downloaderRows(t, s.Read(), "SELECT live FROM import_created WHERE tbl='downloaders' ORDER BY item_id"); !reflect.DeepEqual(got, [][]any{{int64(0)}, {int64(0)}}) {
		t.Fatal("import ownership triggers lost")
	}
	_ = s.Close()
	s, err = Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var count int
	if err := s.Read().QueryRow("SELECT count(*) FROM schema_migrations WHERE version=106").Scan(&count); err != nil || count != 1 {
		t.Fatal("migration not recorded once")
	}
}
