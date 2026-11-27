package database

import (
	"context"
	"database/sql"
	"reflect"
	"strings"
	"testing"
)

// 0163 rebuilds subtitles so made may be transcribed, and keeps every row,
// column and index; a made value it does not know is still refused.
func TestSubtitlesKeptByTranscribedMigration(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	old, err := Open(dir, Options{MainMigrations: upTo(t, 162)})
	if err != nil {
		t.Fatal(err)
	}
	err = old.Write(ctx, func(tx *sql.Tx) error {
		for _, q := range []string{
			`INSERT INTO library_folders(id,created_at,created_by,updated_at,updated_by,path,kind,label,health) VALUES(1,'a','{}','b','{}','/owned-library','movie','Movies','ok')`,
			`INSERT INTO titles(id,created_at,created_by,updated_at,updated_by,kind,name,sort_name,year,source_ids,metadata_source,why_added,state) VALUES(1,'a','{}','b','{}','movie','Owned','Owned',2010,'{}','tmdb','by_hand','active')`,
			`INSERT INTO versions(id,created_at,created_by,updated_at,updated_by,title_id,label,pick_rules_id,library_folder_id,folder_name,is_main) VALUES(1,'a','{}','b','{}',1,'HD',(SELECT min(id) FROM pick_rules),1,'Owned (2010)',1)`,
			`INSERT INTO files(id,created_at,created_by,updated_at,updated_by,slot_ids,version_id,relative_path,added_at,release_name,origin,placed_by) VALUES(4,'a','{}','b','{}','[]',1,'Owned (2010)/Owned.mkv','a','Owned.2010','imported','copy')`,
			`INSERT INTO subtitle_sites(id,created_at,created_by,updated_at,updated_by,name,kind,url,state) VALUES(3,'a','{}','b','{}','Saved site','fake','http://owned-site','enabled')`,
			`INSERT INTO subtitles(id,created_at,created_by,updated_at,updated_by,file_id,language,forced,hearing_impaired,place,relative_path,site_id,site_name,site_key,score,score_parts,origin,state,saved_at,sync,made) VALUES(21,'a','{"kind":"user","id":1}','b','{}',4,'fr',0,1,'beside','Owned (2010)/Owned.fr.srt',3,'Saved site','k1',40,'["x"]','fetched','present','s','{"offset_ms":5}','ai_translated')`,
			`INSERT INTO subtitles(id,created_at,created_by,updated_at,updated_by,file_id,language,place,track_index,codec,origin,state) VALUES(22,'c','{}','d','{}',4,'en','built_in',2,'subrip','found','present')`,
			`INSERT INTO subtitles(id,created_at,created_by,updated_at,updated_by,file_id,language,forced,place,origin,state,tries,first_tried_at,last_tried_at,next_try_at) VALUES(23,'e','{}','f','{}',4,'de',1,'wish','fetched','none_good_enough',3,'t1','t2','t3')`,
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
	const colsQ = `SELECT name, type, "notnull", dflt_value, pk FROM pragma_table_info('subtitles') ORDER BY cid`
	const idxQ = `SELECT name, sql FROM sqlite_master WHERE type = 'index' AND tbl_name = 'subtitles' ORDER BY name`
	cols := downloaderRows(t, old.Read(), colsQ)
	idx := downloaderRows(t, old.Read(), idxQ)
	before := downloaderRows(t, old.Read(), "SELECT * FROM subtitles ORDER BY id")
	_ = old.Close()

	s, err := Open(dir, Options{MainMigrations: upTo(t, 163)})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if got := downloaderRows(t, s.Read(), colsQ); !reflect.DeepEqual(got, cols) {
		t.Fatalf("columns changed: %v", got)
	}
	if got := downloaderRows(t, s.Read(), idxQ); !reflect.DeepEqual(got, idx) {
		t.Fatalf("indexes changed: %v", got)
	}
	if got := downloaderRows(t, s.Read(), "SELECT * FROM subtitles ORDER BY id"); !reflect.DeepEqual(got, before) {
		t.Fatal("a subtitle row changed during the rebuild")
	}
	ins := func(id int, made string) error {
		return s.Write(ctx, func(tx *sql.Tx) error {
			_, err := tx.Exec(`INSERT INTO subtitles(id,created_at,created_by,updated_at,updated_by,file_id,language,place,relative_path,origin,state,made) VALUES(?,'a','{}','b','{}',4,'it','beside',?,'fetched','present',?)`,
				id, "Owned (2010)/Owned.it"+made+".srt", made)
			return err
		})
	}
	if err := ins(30, "transcribed"); err != nil {
		t.Fatalf("transcribed refused: %v", err)
	}
	if err := ins(31, "guessed"); err == nil || !strings.Contains(err.Error(), "CHECK") {
		t.Fatalf("unknown made let in: %v", err)
	}
	// The beside path is still one row per file and name.
	if err := ins(32, "transcribed"); err == nil || !strings.Contains(err.Error(), "UNIQUE") {
		t.Fatalf("second row at one name: %v", err)
	}
	// Deleting the File still takes its subtitles with it.
	if err := s.Write(ctx, func(tx *sql.Tx) error { _, err := tx.Exec("DELETE FROM files WHERE id = 4"); return err }); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.Read().QueryRow("SELECT count(*) FROM subtitles").Scan(&n); err != nil || n != 0 {
		t.Fatalf("%d subtitles left, %v", n, err)
	}
}
