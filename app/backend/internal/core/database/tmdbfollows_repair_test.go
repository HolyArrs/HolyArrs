package database

import (
	"context"
	"database/sql"
	"io/fs"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
)

// migrationsUpTo is the embedded main migrations up to and including n: a
// database made with it is one an older release left behind.
func migrationsUpTo(t *testing.T, n int) fs.FS {
	t.Helper()
	sub, err := fs.Sub(embedded, "migrations/main")
	if err != nil {
		t.Fatal(err)
	}
	out := fstest.MapFS{}
	entries, err := fs.ReadDir(sub, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		v, err := strconv.Atoi(strings.SplitN(e.Name(), "_", 2)[0])
		if err != nil || v > n {
			continue
		}
		b, err := fs.ReadFile(sub, e.Name())
		if err != nil {
			t.Fatal(err)
		}
		out[e.Name()] = &fstest.MapFile{Data: b}
	}
	return out
}

// A database that has 0056 marked done but lost tmdb_movie_follows gets the
// table back on the next start.
func TestTMDBMovieFollowsRepaired(t *testing.T) {
	dir := t.TempDir()
	// An install still at 0082: 0083 (the repair) and later run on the next start.
	s, err := Open(dir, Options{MainMigrations: migrationsUpTo(t, 82)})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Write(context.Background(), func(tx *sql.Tx) error {
		if _, err := tx.Exec(`DROP TABLE tmdb_movie_follows`); err != nil {
			return err
		}
		return nil
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
