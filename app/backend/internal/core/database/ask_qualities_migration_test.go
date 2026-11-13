package database

import (
	"context"
	"database/sql"
	"reflect"
	"testing"
)

// 0122 gives each movie and show ask the line of "What people can ask for"
// it stands for: plain asks become Standard, 4K asks the 4K line. Every
// other column is kept, and one open ask per movie is now per line.
func TestAskQualitiesMigration(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	old, err := Open(dir, Options{MainMigrations: upTo(t, 121)})
	if err != nil {
		t.Fatal(err)
	}
	err = old.Write(ctx, func(tx *sql.Tx) error {
		for _, q := range []string{
			`INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash) VALUES (1, 't', '{}', 't', '{}', 'maria', 'member', 'HASH')`,
			`INSERT INTO asks(id, created_at, created_by, updated_at, updated_by, kind, tmdb_id, name, state, asked_by, asked_at, four_k) VALUES
				(1, 't', '{}', 't', '{}', 'movie', '10', 'Plain movie', 'asked', 1, 't', 0),
				(2, 't', '{}', 't', '{}', 'movie', '10', 'Same movie in 4K', 'asked', 1, 't', 1),
				(3, 't', '{}', 't', '{}', 'show', '20', 'A show in 4K', 'approved', 1, 't', 1),
				(4, 't', '{}', 't', '{}', 'show', '21', 'A plain show', 'turned_down', 1, 't', 0),
				(5, 't', '{}', 't', '{}', 'artist', '', 'An artist', 'asked', 1, 't', 0)`,
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
	before := downloaderRows(t, old.Read(), "SELECT * FROM asks ORDER BY id")
	_ = old.Close()

	s, err := Open(dir, Options{MainMigrations: upTo(t, 122)})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	after := downloaderRows(t, s.Read(), "SELECT * FROM asks ORDER BY id")
	for i := range before {
		if !reflect.DeepEqual(after[i][:len(before[i])], before[i]) {
			t.Fatalf("ask %d changed: %v", i+1, after[i])
		}
	}
	got := downloaderRows(t, s.Read(), "SELECT id, quality, quality_name, quality_rules_id, quality_extra FROM asks ORDER BY id")
	want := [][]any{
		{int64(1), "standard", "", int64(0), int64(0)},
		{int64(2), "4k", "4K", int64(0), int64(1)},
		{int64(3), "4k", "4K", int64(0), int64(1)},
		{int64(4), "standard", "", int64(0), int64(0)},
		{int64(5), "", "", int64(0), int64(0)},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("quality columns = %v, want %v", got, want)
	}
	// One open ask per movie and line: a second open Standard ask is refused,
	// a new line for the same movie is not.
	insert := func(id int, quality string) error {
		return s.Write(ctx, func(tx *sql.Tx) error {
			_, err := tx.Exec(`INSERT INTO asks(id, created_at, created_by, updated_at, updated_by, kind, tmdb_id, name, state, asked_by, asked_at, quality) VALUES (?, 't', '{}', 't', '{}', 'movie', '10', 'x', 'asked', 1, 't', ?)`, id, quality)
			return err
		})
	}
	if err := insert(6, "standard"); err == nil {
		t.Fatal("a second open Standard ask for one movie was let in")
	}
	if err := insert(7, "hdr"); err != nil {
		t.Fatalf("an ask for another line was refused: %v", err)
	}
	var check string
	if err := s.Read().QueryRow("PRAGMA quick_check").Scan(&check); err != nil || check != "ok" {
		t.Fatal("database integrity check failed")
	}
}
