package database

import (
	"context"
	"database/sql"
	"reflect"
	"testing"
)

// 0121 rebuilds sources to allow the site format and keeps every Source
// (every column and id), the Releases and calls that point at it and both
// import_created triggers.
func TestSourcesKeptBySiteMigration(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	old, err := Open(dir, Options{MainMigrations: upTo(t, 120)})
	if err != nil {
		t.Fatal(err)
	}
	err = old.Write(ctx, func(tx *sql.Tx) error {
		for _, q := range []string{
			`INSERT INTO secrets(id,created_at,created_by,updated_at,updated_by,kind,value) VALUES(81,'t','{}','t','{}','source_secret','v1:synthetic-encrypted-value')`,
			`INSERT INTO sources(id,created_at,created_by,updated_at,updated_by,name,protocol,format,url,secret_ref,use_for,kinds,categories,min_seeders,limits,proxy_id,priority,state,pause_reason,paused_until,extra,allow_local_network,proxy_url,proxy_secret_ref,proxy_allow_local_network,path_secret_ref,tags) VALUES(7,'a','{"kind":"user","id":1}','b','{}','Saved Torznab','torrent','torznab','http://owned-fixture',81,'{"feed":true}','["movie"]','{"movie":[2000]}',3,'{"per_hour":10}',4,9,'paused','rate_limited','later','{"k":"v"}',1,'http://proxy-fixture',81,1,81,'["4k"]')`,
			`INSERT INTO sources(id,created_at,created_by,updated_at,updated_by,name,protocol,format,url,state) VALUES(11,'c','{}','d','{}','Saved Newznab','usenet','newznab','http://owned-nzb','disabled')`,
			`INSERT INTO sources(id,created_at,created_by,updated_at,updated_by,name,protocol,format,url) VALUES(12,'e','{}','f','{}','Saved Bridge','torrent','bridge','http://owned-bridge')`,
			`INSERT INTO releases(id,created_at,created_by,updated_at,updated_by,source_id,name,link_ref,protocol,found_by) VALUES(13,'a','{}','b','{}',7,'Owned.Release','owned-link','torrent','feed')`,
			`INSERT INTO source_calls(id,source_id,at,op,ok,ms,error_kind) VALUES(5,7,'a','search',0,120,'down')`,
			`INSERT INTO import_created(tbl,item_id,live) VALUES('sources',11,1),('sources',12,1)`,
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
	cols := downloaderRows(t, old.Read(), "SELECT name, type, \"notnull\", dflt_value, pk FROM pragma_table_info('sources') ORDER BY cid")
	before := downloaderRows(t, old.Read(), "SELECT * FROM sources ORDER BY id")
	releases := downloaderRows(t, old.Read(), "SELECT * FROM releases ORDER BY id")
	calls := downloaderRows(t, old.Read(), "SELECT * FROM source_calls ORDER BY id")
	_ = old.Close()

	s, err := Open(dir, Options{MainMigrations: upTo(t, 121)})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if got := downloaderRows(t, s.Read(), "SELECT name, type, \"notnull\", dflt_value, pk FROM pragma_table_info('sources') ORDER BY cid"); !reflect.DeepEqual(got, cols) {
		t.Fatalf("columns changed: %v", got)
	}
	if got := downloaderRows(t, s.Read(), "SELECT * FROM sources ORDER BY id"); !reflect.DeepEqual(got, before) {
		t.Fatal("saved Source row changed during rebuild")
	}
	if got := downloaderRows(t, s.Read(), "SELECT * FROM releases ORDER BY id"); !reflect.DeepEqual(got, releases) {
		t.Fatal("dependent Release changed during rebuild")
	}
	if got := downloaderRows(t, s.Read(), "SELECT * FROM source_calls ORDER BY id"); !reflect.DeepEqual(got, calls) {
		t.Fatal("Source call changed during rebuild")
	}
	if got := downloaderRows(t, s.Read(), "PRAGMA foreign_key_check"); len(got) != 0 {
		t.Fatalf("foreign key violations=%v", got)
	}
	var check string
	if err := s.Read().QueryRow("PRAGMA quick_check").Scan(&check); err != nil || check != "ok" {
		t.Fatal("database integrity check failed")
	}
	for _, tbl := range []string{"releases", "source_calls"} {
		if got := downloaderRows(t, s.Read(), "SELECT \"table\" FROM pragma_foreign_key_list('"+tbl+"') WHERE \"table\"='sources'"); len(got) == 0 {
			t.Fatalf("%s lost its foreign key", tbl)
		}
	}
	err = s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO sources(created_at,created_by,updated_at,updated_by,name,protocol,format,url,extra) VALUES('t','{}','t','{}','Knaben','torrent','site','https://owned-site','{"site":"knaben"}')`)
		return err
	})
	if err != nil {
		t.Fatalf("site format refused: %v", err)
	}
	for _, q := range []string{
		`INSERT INTO sources(created_at,created_by,updated_at,updated_by,name,protocol,format,url) VALUES('t','{}','t','{}','Odd','torrent','unsupported','http://x')`,
		`UPDATE sources SET tags='not json' WHERE id=7`,
		`UPDATE sources SET name='Saved Newznab' WHERE id=7`,
		`UPDATE releases SET source_id=999999 WHERE id=13`,
	} {
		if err := s.Write(ctx, func(tx *sql.Tx) error { _, err := tx.Exec(q); return err }); err == nil {
			t.Fatalf("constraint lost: %s", q)
		}
	}
	for _, q := range []string{`UPDATE sources SET state='enabled' WHERE id=11`, `DELETE FROM sources WHERE id=12`} {
		if err := s.Write(ctx, func(tx *sql.Tx) error { _, err := tx.Exec(q); return err }); err != nil {
			t.Fatal(err)
		}
	}
	if got := downloaderRows(t, s.Read(), "SELECT live FROM import_created WHERE tbl='sources' ORDER BY item_id"); !reflect.DeepEqual(got, [][]any{{int64(0)}, {int64(0)}}) {
		t.Fatal("import ownership triggers lost")
	}
	_ = s.Close()
	s, err = Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var count int
	if err := s.Read().QueryRow("SELECT count(*) FROM schema_migrations WHERE version=121").Scan(&count); err != nil || count != 1 {
		t.Fatal("migration not recorded once")
	}
}
