package database

import (
	"context"
	"database/sql"
	"io/fs"
	"strings"
	"testing"
)

// TestKnownPlacesMigration: an upgraded database gets the known places
// table, one row per name and place.
func TestKnownPlacesMigration(t *testing.T) {
	src, _ := fs.Sub(embedded, "migrations/main")
	ms, err := loadMigrations(src)
	if err != nil {
		t.Fatal(err)
	}
	v := 0
	for _, m := range ms {
		if strings.HasSuffix(m.name, "_known_places.sql") {
			v = m.version
		}
	}
	if v == 0 {
		t.Fatal("known places migration missing")
	}
	dir := t.TempDir()
	s, err := Open(dir, Options{MainMigrations: upTo(t, v-1)})
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	s, err = Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	err = s.Write(context.Background(), func(tx *sql.Tx) error {
		for _, at := range []string{"2026-01-01T00:00:00.000000000Z", "2026-01-02T00:00:00.000000000Z"} {
			if _, err := tx.Exec(`INSERT INTO known_places(user_key, ip_key, last_at) VALUES ('name:luis', 'ip:203.0.113.50', ?)
				ON CONFLICT(user_key, ip_key) DO UPDATE SET last_at = excluded.last_at`, at); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var n int
	var last string
	if err := s.Read().QueryRow("SELECT count(*), max(last_at) FROM known_places").Scan(&n, &last); err != nil || n != 1 || last != "2026-01-02T00:00:00.000000000Z" {
		t.Fatalf("rows %d last %q: %v", n, last, err)
	}
}
