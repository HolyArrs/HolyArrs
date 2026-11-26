package database

import (
	"context"
	"database/sql"
	"reflect"
	"testing"
)

// 0156 rebuilds media_servers to add the kodi kind; every saved server,
// its id and its columns stay as they were.
func TestMediaServersKeptByKodiMigration(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	old, err := Open(dir, Options{MainMigrations: upTo(t, 155)})
	if err != nil {
		t.Fatal(err)
	}
	err = old.Write(ctx, func(tx *sql.Tx) error {
		for _, q := range []string{
			`INSERT INTO media_servers(id,created_at,created_by,updated_at,updated_by,name,kind,url,enabled,on_import,on_rename,on_delete,path_from,path_to,allow_local_network,last_ok_at,last_error,web_url,plain_password) VALUES(7,'a','{}','b','{}','Saved Navidrome','subsonic','http://owned-nd',1,1,0,1,'/d','/m',0,'ok-time','safe error','http://web',1)`,
			`INSERT INTO media_servers(id,created_at,created_by,updated_at,updated_by,name,kind,url,enabled) VALUES(11,'c','{}','d','{}','Saved Jellyfin','jellyfin','http://owned-jf',0)`,
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
	cols := downloaderRows(t, old.Read(), "SELECT name, type, \"notnull\", dflt_value, pk FROM pragma_table_info('media_servers') ORDER BY cid")
	before := downloaderRows(t, old.Read(), "SELECT * FROM media_servers ORDER BY id")
	_ = old.Close()

	s, err := Open(dir, Options{MainMigrations: upTo(t, 156)})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if got := downloaderRows(t, s.Read(), "SELECT name, type, \"notnull\", dflt_value, pk FROM pragma_table_info('media_servers') ORDER BY cid"); !reflect.DeepEqual(got, cols) {
		t.Fatalf("columns changed: %v", got)
	}
	if got := downloaderRows(t, s.Read(), "SELECT * FROM media_servers ORDER BY id"); !reflect.DeepEqual(got, before) {
		t.Fatal("saved media server row changed during rebuild")
	}
	err = s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO media_servers(id,created_at,created_by,updated_at,updated_by,name,kind,url) VALUES(12,'e','{}','f','{}','Kodi','kodi','http://owned-kodi')`)
		return err
	})
	if err != nil {
		t.Fatalf("kodi kind refused: %v", err)
	}
}
