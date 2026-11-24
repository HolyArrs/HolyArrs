package database

import (
	"context"
	"database/sql"
	"testing"
)

// 0146 folds a "(imported)" copy of a Downloader already here into it:
// links move over, the kept one keeps its own settings and only fills empty
// ones, and a pair that would clash is left alone.
func TestMergeImportedDownloaders(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	old, err := Open(dir, Options{MainMigrations: upTo(t, 145)})
	if err != nil {
		t.Fatal(err)
	}
	err = old.Write(ctx, func(tx *sql.Tx) error {
		for _, q := range []string{
			`INSERT INTO secrets(id,created_at,created_by,updated_at,updated_by,kind,value) VALUES(9,'t','{}','t','{}','downloader_secret','v1:synthetic')`,
			`INSERT INTO downloaders(id,created_at,created_by,updated_at,updated_by,name,adapter,protocol,url,category,priority) VALUES(1,'a','{}','a','{}','SABnzbd','sabnzbd','usenet','http://10.0.0.5:8080','mine',3)`,
			`INSERT INTO downloaders(id,created_at,created_by,updated_at,updated_by,name,adapter,protocol,url,category,secret_ref,username,state) VALUES(2,'a','{}','a','{}','SABnzbd (imported)','sabnzbd','usenet','http://10.0.0.5:8080/','mine',9,'u','ok')`,
			`INSERT INTO downloaders(id,created_at,created_by,updated_at,updated_by,name,adapter,protocol,url) VALUES(3,'a','{}','a','{}','qBittorrent','qbittorrent','torrent','http://10.0.0.5:9090')`,
			`INSERT INTO downloaders(id,created_at,created_by,updated_at,updated_by,name,adapter,protocol,url) VALUES(4,'a','{}','a','{}','qBittorrent (imported)','qbittorrent','torrent','http://10.0.0.5:9090')`,
			`INSERT INTO downloaders(id,created_at,created_by,updated_at,updated_by,name,adapter,protocol,url) VALUES(5,'a','{}','a','{}','Other','qbittorrent','torrent','http://10.0.0.6:9090')`,
			`INSERT INTO downloaders(id,created_at,created_by,updated_at,updated_by,name,adapter,protocol,url) VALUES(6,'a','{}','a','{}','Other (imported)','qbittorrent','torrent','http://10.0.0.7:9090')`,
			`INSERT INTO downloads(id,created_at,created_by,updated_at,updated_by,downloader_id,external_id,progress,state,started_by) VALUES(10,'a','{}','a','{}',2,'nzo1',0.5,'paused','{}')`,
			`INSERT INTO downloads(id,created_at,created_by,updated_at,updated_by,downloader_id,external_id,progress,state,started_by) VALUES(11,'a','{}','a','{}',3,'h1',0.5,'paused','{}')`,
			`INSERT INTO downloads(id,created_at,created_by,updated_at,updated_by,downloader_id,external_id,progress,state,started_by) VALUES(12,'a','{}','a','{}',4,'h1',0.5,'paused','{}')`,
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
	_ = old.Close()
	s, err := Open(dir, Options{MainMigrations: upTo(t, 146)})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	db := s.Read()
	var merged int
	if err := db.QueryRow("SELECT count(*) FROM downloader_merges WHERE dup = 2 AND keep = 1").Scan(&merged); err != nil || merged != 1 {
		t.Fatalf("merge record: %d %v", merged, err)
	}
	var dl int64
	if err := db.QueryRow("SELECT downloader_id FROM downloads WHERE id = 10").Scan(&dl); err != nil || dl != 1 {
		t.Fatalf("download not moved to the kept SABnzbd: %d %v", dl, err)
	}
	var cat, user string
	var ref sql.NullInt64
	var prio int
	if err := db.QueryRow("SELECT category, username, secret_ref, priority FROM downloaders WHERE id = 1").Scan(&cat, &user, &ref, &prio); err != nil {
		t.Fatal(err)
	}
	if cat != "mine" || prio != 3 || user != "u" || !ref.Valid || ref.Int64 != 9 {
		t.Fatalf("kept SABnzbd: %s %d %s %v", cat, prio, user, ref)
	}
	live := map[int64]bool{}
	rows, err := db.Query("SELECT id FROM downloaders WHERE removed = 0")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id int64
		_ = rows.Scan(&id)
		live[id] = true
	}
	rows.Close()
	want := map[int64]bool{1: true, 3: true, 4: true, 5: true, 6: true}
	if len(live) != len(want) {
		t.Fatalf("live Downloaders %v, want %v", live, want)
	}
	for id := range want {
		if !live[id] {
			t.Fatalf("live Downloaders %v, want %v", live, want)
		}
	}
	var name string
	var gone sql.NullInt64
	if err := db.QueryRow("SELECT name, secret_ref FROM downloaders WHERE id = 2").Scan(&name, &gone); err != nil || name != "SABnzbd (imported) (removed 2)" || gone.Valid {
		t.Fatalf("copy: %q %v %v", name, gone, err)
	}
	var check string
	if err := db.QueryRow("PRAGMA foreign_key_check").Scan(&check); err != sql.ErrNoRows {
		t.Fatalf("foreign key violations: %v", err)
	}
}
