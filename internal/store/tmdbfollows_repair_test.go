package store

import (
	"context"
	"database/sql"
	"testing"
)

// A database that has 0056 marked done but lost tmdb_movie_follows gets the
// table back on the next start.
func TestTMDBMovieFollowsRepaired(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Write(context.Background(), func(tx *sql.Tx) error {
		if _, err := tx.Exec(`DROP TABLE tmdb_movie_follows`); err != nil {
			return err
		}
		// Back to how a database at 0082 looks: the later migrations run
		// again, so what 0085 added goes too.
		for _, col := range []string{"title_kind", "seasons", "new_seasons"} {
			if _, err := tx.Exec(`ALTER TABLE outside_lists DROP COLUMN ` + col); err != nil {
				return err
			}
		}
		_, err := tx.Exec(`DELETE FROM schema_migrations WHERE version >= 83`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()

	s, err = Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var n int
	if err := s.Read().QueryRow(`SELECT count(*) FROM sqlite_master WHERE name IN ('tmdb_movie_follows', 'tmdb_movie_follows_title')`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("got %d of the table and its index, want 2", n)
	}
}
