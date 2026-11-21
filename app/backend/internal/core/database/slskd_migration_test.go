package database

import (
	"context"
	"database/sql"
	"reflect"
	"testing"
)

// 0133 rebuilds sources, releases and downloaders to allow the soulseek
// protocol and the slskd adapter. Every row keeps its id, its later columns
// and the rows that point at it, and the import_created triggers come back.
func TestRowsKeptBySlskdMigration(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	old, err := Open(dir, Options{MainMigrations: upTo(t, 132)})
	if err != nil {
		t.Fatal(err)
	}
	err = old.Write(ctx, func(tx *sql.Tx) error {
		for _, q := range []string{
			`INSERT INTO sources(id,created_at,created_by,updated_at,updated_by,name,protocol,format,url,extra,tags,downloader_id) VALUES(3,'a','{}','b','{}','Saved Knaben','torrent','site','https://api.knaben.org','{"site":"knaben"}','["4k"]',7)`,
			`INSERT INTO releases(id,created_at,created_by,updated_at,updated_by,source_id,name,link_ref,protocol,size,info_hash,found_by,state) VALUES(21,'a','{}','b','{}',3,'Owned.Release','magnet:?xt=urn:btih:aa','torrent',9,'aa','feed','chosen')`,
			`INSERT INTO downloaders(id,created_at,created_by,updated_at,updated_by,name,adapter,protocol,url,category_after_import,own_only) VALUES(7,'c','{}','d','{}','Saved qBittorrent','qbittorrent','torrent','http://owned-fixture','done',1)`,
			`INSERT INTO downloads(id,created_at,created_by,updated_at,updated_by,downloader_id,release_id,external_id,progress,state,started_by) VALUES(13,'a','{}','b','{}',7,21,'owned-external-id',0.4,'paused','{}')`,
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
	queries := []string{"SELECT * FROM sources ORDER BY id", "SELECT * FROM releases ORDER BY id", "SELECT * FROM downloaders ORDER BY id",
		"SELECT * FROM downloads ORDER BY id"}
	for _, tbl := range []string{"sources", "releases", "downloaders"} {
		queries = append(queries, "SELECT name, type, \"notnull\", dflt_value, pk FROM pragma_table_info('"+tbl+"') ORDER BY cid",
			"SELECT name FROM sqlite_master WHERE tbl_name = '"+tbl+"' AND type IN ('index', 'trigger') ORDER BY name")
	}
	before := make([][][]any, len(queries))
	for i, q := range queries {
		before[i] = downloaderRows(t, old.Read(), q)
	}
	_ = old.Close()

	s, err := Open(dir, Options{MainMigrations: upTo(t, 133)})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for i, q := range queries {
		if got := downloaderRows(t, s.Read(), q); !reflect.DeepEqual(got, before[i]) {
			t.Fatalf("%s changed: %v, was %v", q, got, before[i])
		}
	}
	if got := downloaderRows(t, s.Read(), "PRAGMA foreign_key_check"); len(got) != 0 {
		t.Fatalf("foreign key violations=%v", got)
	}
	err = s.Write(ctx, func(tx *sql.Tx) error {
		for _, q := range []string{
			`INSERT INTO sources(id,created_at,created_by,updated_at,updated_by,name,protocol,format,url,extra) VALUES(4,'t','{}','t','{}','slskd','soulseek','site','http://slskd:5030','{"site":"slskd"}')`,
			`INSERT INTO releases(created_at,created_by,updated_at,updated_by,source_id,name,link_ref,protocol,found_by) VALUES('t','{}','t','{}',4,'Artist - Album [FLAC]','slskd:e30','soulseek','auto_search')`,
			`INSERT INTO downloaders(created_at,created_by,updated_at,updated_by,name,adapter,protocol,url) VALUES('t','{}','t','{}','slskd','slskd','soulseek','http://slskd:5030')`,
		} {
			if _, err := tx.Exec(q); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("soulseek refused: %v", err)
	}
	err = s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO releases(created_at,created_by,updated_at,updated_by,source_id,name,link_ref,protocol,found_by) VALUES('t','{}','t','{}',4,'x','x','ftp','feed')`)
		return err
	})
	if err == nil {
		t.Fatal("an unknown protocol was taken")
	}
}
