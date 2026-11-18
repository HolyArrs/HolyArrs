package database

import (
	"context"
	"database/sql"
	"reflect"
	"testing"
)

// 0132 gives each person's ask settings a quality choice. Every row that was
// there keeps its limits and alert targets, and its choice starts empty
// (every line that is shown, as before).
func TestPersonQualitiesMigration(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	old, err := Open(dir, Options{MainMigrations: upTo(t, 131)})
	if err != nil {
		t.Fatal(err)
	}
	err = old.Write(ctx, func(tx *sql.Tx) error {
		for _, q := range []string{
			`INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash) VALUES
				(1, 't', '{}', 't', '{}', 'maria', 'member', 'HASH'), (2, 't', '{}', 't', '{}', 'jo', 'member', 'HASH')`,
			`INSERT INTO user_ask_settings (user_id, movies_per_week, shows_per_week, email) VALUES (1, 3, 0, 'maria@example.test'), (2, NULL, NULL, '')`,
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
	before := downloaderRows(t, old.Read(), "SELECT * FROM user_ask_settings ORDER BY user_id")
	_ = old.Close()

	s, err := Open(dir, Options{MainMigrations: upTo(t, 132)})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	after := downloaderRows(t, s.Read(), "SELECT * FROM user_ask_settings ORDER BY user_id")
	for i := range before {
		if !reflect.DeepEqual(after[i][:len(before[i])], before[i]) {
			t.Fatalf("row %d changed: %v", i+1, after[i])
		}
	}
	var n int
	if err := s.Read().QueryRow("SELECT count(*) FROM user_ask_settings WHERE qualities IS NULL").Scan(&n); err != nil || n != 2 {
		t.Fatalf("choices not empty: %d %v", n, err)
	}
	var check string
	if err := s.Read().QueryRow("PRAGMA quick_check").Scan(&check); err != nil || check != "ok" {
		t.Fatal("database integrity check failed")
	}
}
