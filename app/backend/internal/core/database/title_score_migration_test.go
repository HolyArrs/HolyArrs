package database

import (
	"context"
	"database/sql"
	"testing"
)

// 0104 adds the score and vote count to titles: a title from an older
// build keeps every field and has no score (NULL) and 0 votes until its
// next details refresh.
func TestTitleScoreMigration(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	old, err := Open(dir, Options{MainMigrations: upTo(t, 103)})
	if err != nil {
		t.Fatal(err)
	}
	err = old.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO titles (id, created_at, created_by, updated_at, updated_by, kind, name, sort_name, metadata_source, why_added, rating, runtime_minutes)
			VALUES (7, 'c', '{"kind":"system"}', 'u', '{"kind":"system"}', 'movie', 'Old Film', 'old film', 'tmdb', 'by_hand', 'PG-13', 121)`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = old.Close()

	s, err := Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var name, rating string
	var runtime, votes int
	var score sql.NullFloat64
	if err := s.Read().QueryRow(`SELECT name, rating, runtime_minutes, score, votes FROM titles WHERE id = 7`).Scan(&name, &rating, &runtime, &score, &votes); err != nil {
		t.Fatal(err)
	}
	if name != "Old Film" || rating != "PG-13" || runtime != 121 || score.Valid || votes != 0 {
		t.Fatalf("old title %q %q %d score %v votes %d", name, rating, runtime, score, votes)
	}
}
