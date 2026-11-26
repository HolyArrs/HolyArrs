package database

import (
	"context"
	"database/sql"
	"reflect"
	"testing"
)

// 0160 rebuilds sources, releases and downloaders to allow a plugin's own
// download type and keeps every row, id, column, index, trigger and the rows
// that point at them.
func TestPluginDownloadTypesMigrationKeepsRows(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	old, err := Open(dir, Options{MainMigrations: upTo(t, 159)})
	if err != nil {
		t.Fatal(err)
	}
	err = old.Write(ctx, func(tx *sql.Tx) error {
		for _, q := range []string{
			`INSERT INTO sources(id,created_at,created_by,updated_at,updated_by,name,protocol,format,url,secret_ref,use_for,kinds,categories,min_seeders,limits,proxy_id,priority,state,pause_reason,paused_until,extra,allow_local_network,proxy_url,proxy_secret_ref,proxy_allow_local_network,path_secret_ref,tags,downloader_id,multi_languages,no_language_languages) VALUES(3,'a','{}','b','{}','Saved feed','torrent','torznab','http://owned-src',4,'{"auto":true}','["movie"]','{"movie":[2000]}',2,'{"gap_ms":5}',6,7,'disabled','down','later','{"x":"y"}',1,'http://proxy',8,1,9,'["4k"]',7,'["en"]','["fr"]')`,
			`INSERT INTO sources(id,created_at,created_by,updated_at,updated_by,name,protocol,format,url) VALUES(5,'c','{}','d','{}','Saved nzb','usenet','newznab','http://owned-nzb')`,
			`INSERT INTO releases(id,created_at,created_by,updated_at,updated_by,source_id,name,link_ref,protocol,size,published_at,seeders,peers,info_hash,flags,parsed,matched_slot_ids,verdict,found_by,state,hold_until) VALUES(21,'a','{}','b','{}',3,'Owned.Release.2020','link','torrent',99,'p',4,5,'abc','["f"]','{"q":1}','[1]','{"ok":true}','hand_search','held','until')`,
			`INSERT INTO releases(id,created_at,created_by,updated_at,updated_by,source_id,name,link_ref,protocol,found_by) VALUES(22,'c','{}','d','{}',5,'Owned.Nzb','nzb','usenet','feed')`,
			`INSERT INTO downloaders(id,created_at,created_by,updated_at,updated_by,name,adapter,protocol,url,state,seed_goal,tags) VALUES(7,'a','{}','b','{}','Saved qBittorrent','qbittorrent','torrent','http://owned-qb','disabled','{"ratio":2}','["x"]')`,
			`INSERT INTO downloaders(id,created_at,created_by,updated_at,updated_by,name,adapter,protocol,url) VALUES(8,'c','{}','d','{}','Plugin client','plugin-torrent','torrent','plugin://dl')`,
			`INSERT INTO downloads(id,created_at,created_by,updated_at,updated_by,downloader_id,release_id,external_id,state,started_by) VALUES(13,'a','{}','b','{}',7,21,'owned-ext','paused','{}')`,
			`INSERT INTO source_calls(id,source_id,at,op,ok,ms) VALUES(1,3,'t','search',1,40)`,
			`INSERT INTO import_created(tbl,item_id,live) VALUES('sources',3,1),('downloaders',7,1)`,
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
	queries := []string{
		"SELECT * FROM sources ORDER BY id", "SELECT * FROM releases ORDER BY id", "SELECT * FROM downloaders ORDER BY id",
		"SELECT * FROM downloads ORDER BY id", "SELECT * FROM source_calls ORDER BY id",
		"SELECT m.name, p.cid, p.name, p.type, p.\"notnull\", p.dflt_value, p.pk FROM sqlite_master m, pragma_table_info(m.name) p WHERE m.name IN ('sources','releases','downloaders') ORDER BY m.name, p.cid",
		"SELECT name, sql FROM sqlite_master WHERE type IN ('index','trigger') AND tbl_name IN ('sources','releases','downloaders') ORDER BY name",
	}
	before := make([][][]any, len(queries))
	for i, q := range queries {
		before[i] = downloaderRows(t, old.Read(), q)
	}
	_ = old.Close()

	s, err := Open(dir, Options{MainMigrations: upTo(t, 160)})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for i, q := range queries {
		if got := downloaderRows(t, s.Read(), q); !reflect.DeepEqual(got, before[i]) {
			t.Fatalf("changed by the rebuild: %s\n got %v\nwant %v", q, got, before[i])
		}
	}
	if got := downloaderRows(t, s.Read(), "PRAGMA foreign_key_check"); len(got) != 0 {
		t.Fatalf("foreign key violations=%v", got)
	}
	for tbl, parent := range map[string]string{"releases": "sources", "source_calls": "sources", "downloads": "releases", "slots": "releases", "downloader_attempts": "downloaders"} {
		if got := downloaderRows(t, s.Read(), "SELECT 1 FROM pragma_foreign_key_list('"+tbl+"') WHERE \"table\"='"+parent+"'"); len(got) == 0 {
			t.Fatalf("%s lost its foreign key to %s", tbl, parent)
		}
	}
	err = s.Write(ctx, func(tx *sql.Tx) error {
		for _, q := range []string{
			`INSERT INTO sources(id,created_at,created_by,updated_at,updated_by,name,protocol,format,url) VALUES(30,'t','{}','t','{}','Plugin search','soulseek','torznab','plugin://slskd')`,
			`INSERT INTO releases(created_at,created_by,updated_at,updated_by,source_id,name,link_ref,protocol,found_by) VALUES('t','{}','t','{}',30,'Album','plugin://slskd/x','soulseek','hand_search')`,
			`INSERT INTO downloaders(created_at,created_by,updated_at,updated_by,name,adapter,protocol,url) VALUES('t','{}','t','{}','Plugin soulseek','plugin','soulseek','plugin://slskd')`,
		} {
			if _, err := tx.Exec(q); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("plugin download type refused: %v", err)
	}
	for _, q := range []string{
		`UPDATE sources SET protocol='Soulseek' WHERE id=30`,
		`UPDATE sources SET protocol='x' WHERE id=30`,
		`UPDATE releases SET protocol='9lives' WHERE id=21`,
		`UPDATE releases SET protocol='bad_type' WHERE id=21`,
		`UPDATE downloaders SET protocol='a-very-long-download-type-name-xx' WHERE id=8`,
		`UPDATE downloaders SET adapter='unsupported' WHERE id=8`,
		`UPDATE downloaders SET name='Plugin client' WHERE id=7`,
		`UPDATE releases SET source_id=999 WHERE id=21`,
	} {
		if err := s.Write(ctx, func(tx *sql.Tx) error { _, err := tx.Exec(q); return err }); err == nil {
			t.Fatalf("constraint lost: %s", q)
		}
	}
	for _, q := range []string{`UPDATE sources SET state='enabled' WHERE id=3`, `UPDATE downloaders SET state='ok' WHERE id=7`} {
		if err := s.Write(ctx, func(tx *sql.Tx) error { _, err := tx.Exec(q); return err }); err != nil {
			t.Fatal(err)
		}
	}
	if got := downloaderRows(t, s.Read(), "SELECT live FROM import_created ORDER BY tbl"); !reflect.DeepEqual(got, [][]any{{int64(0)}, {int64(0)}}) {
		t.Fatal("import ownership triggers lost")
	}
	if err := s.Write(ctx, func(tx *sql.Tx) error { _, err := tx.Exec(`DELETE FROM sources WHERE id=3`); return err }); err != nil {
		t.Fatal(err)
	}
	if got := downloaderRows(t, s.Read(), "SELECT id, release_id FROM downloads"); !reflect.DeepEqual(got, [][]any{{int64(13), nil}}) {
		t.Fatalf("release cascade lost: %v", got)
	}
}
