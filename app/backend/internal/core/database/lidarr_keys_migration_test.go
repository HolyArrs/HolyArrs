package database

import (
	"context"
	"database/sql"
	"reflect"
	"testing"
)

// 0162 rebuilds compat_keys to add the lidarr side; every saved key, its
// side and its columns stay as they were.
func TestCompatKeysKeptByLidarrMigration(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	old, err := Open(dir, Options{MainMigrations: upTo(t, 161)})
	if err != nil {
		t.Fatal(err)
	}
	err = old.Write(ctx, func(tx *sql.Tx) error {
		for _, q := range []string{
			`INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash) VALUES (1, 't', '{}', 't', '{}', 'luis', 'owner', 'HASH')`,
			`INSERT INTO compat_keys(side, created_at, created_by, owner_user_id, last_used_at, hash, hint) VALUES ('sonarr', 'a', '{}', 1, 'used', 'hash-s', 'abcd')`,
			`INSERT INTO compat_keys(side, created_at, created_by, owner_user_id, hash, hint) VALUES ('radarr', 'b', '{}', 1, 'hash-r', 'efgh')`,
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
	cols := downloaderRows(t, old.Read(), "SELECT name, type, \"notnull\", dflt_value, pk FROM pragma_table_info('compat_keys') ORDER BY cid")
	before := downloaderRows(t, old.Read(), "SELECT * FROM compat_keys ORDER BY side")
	_ = old.Close()

	s, err := Open(dir, Options{MainMigrations: upTo(t, 162)})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if got := downloaderRows(t, s.Read(), "SELECT name, type, \"notnull\", dflt_value, pk FROM pragma_table_info('compat_keys') ORDER BY cid"); !reflect.DeepEqual(got, cols) {
		t.Fatalf("columns changed: %v", got)
	}
	if got := downloaderRows(t, s.Read(), "SELECT * FROM compat_keys ORDER BY side"); !reflect.DeepEqual(got, before) {
		t.Fatal("saved arr key changed during rebuild")
	}
	err = s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO compat_keys(side, created_at, created_by, owner_user_id, hash, hint) VALUES ('lidarr', 'c', '{}', 1, 'hash-l', 'ijkl')`)
		return err
	})
	if err != nil {
		t.Fatalf("lidarr side refused: %v", err)
	}
}
