package database

import (
	"context"
	"database/sql"
	"reflect"
	"testing"
)

// 0123 lets an Outside list be an MDBList list. SQLite cannot change a
// CHECK, so the table is copied: every row and column must come through,
// and other kinds must still be refused.
func TestMDBListListsMigration(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	old, err := Open(dir, Options{MainMigrations: upTo(t, 122)})
	if err != nil {
		t.Fatal(err)
	}
	err = old.Write(ctx, func(tx *sql.Tx) error {
		for _, q := range []string{
			`INSERT INTO library_folders(id, created_at, created_by, updated_at, updated_by, path, kind, label) VALUES
				(1, 't', '{}', 't', '{}', '/movies', 'movie', 'Movies'),
				(2, 't', '{}', 't', '{}', '/shows', 'show', 'Shows')`,
			`INSERT INTO outside_lists(id, created_at, created_by, updated_at, updated_by, name, kind, ref, library_folder_id, last_result) VALUES
				(1, 't', '{}', 't', '{}', 'Top films', 'tmdb_list', '42', 1, 'x')`,
			`INSERT INTO outside_lists(id, created_at, created_by, updated_at, updated_by, name, kind, ref, library_folder_id, title_kind, seasons, new_seasons, sync_every_hours, last_synced_at, last_error) VALUES
				(2, 't', '{}', 't', '{}', 'Kim shows', 'trakt_list', 'kim/tv', 2, 'show', 'all', 'all', 24, 't2', 'boom')`,
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
	before := downloaderRows(t, old.Read(), "SELECT * FROM outside_lists ORDER BY id")
	_ = old.Close()

	s, err := Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if after := downloaderRows(t, s.Read(), "SELECT * FROM outside_lists ORDER BY id"); !reflect.DeepEqual(after, before) {
		t.Fatalf("rows changed:\n%v\nwant\n%v", after, before)
	}
	insert := func(id int, kind string) error {
		return s.Write(ctx, func(tx *sql.Tx) error {
			_, err := tx.Exec(`INSERT INTO outside_lists(id, created_at, created_by, updated_at, updated_by, name, kind, ref, library_folder_id) VALUES (?, 't', '{}', 't', '{}', ?, ?, 'lab/picks', 1)`, id, "list "+kind, kind)
			return err
		})
	}
	if err := insert(3, "mdblist_list"); err != nil {
		t.Fatalf("an MDBList list was refused: %v", err)
	}
	if err := insert(4, "bogus"); err == nil {
		t.Fatal("an unknown kind was let in")
	}
	var fk, schedules int
	if err := s.Read().QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil || fk != 1 {
		t.Fatalf("foreign_keys = %d %v", fk, err)
	}
	if err := s.Read().QueryRow("SELECT COUNT(*) FROM schedules WHERE kind = 'sync_outside_lists'").Scan(&schedules); err != nil || schedules != 1 {
		t.Fatalf("list schedule rows = %d %v", schedules, err)
	}
	var check string
	if err := s.Read().QueryRow("PRAGMA quick_check").Scan(&check); err != nil || check != "ok" {
		t.Fatal("database integrity check failed")
	}
}
