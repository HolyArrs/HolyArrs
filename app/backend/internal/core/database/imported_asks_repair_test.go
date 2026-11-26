package database

import (
	"context"
	"database/sql"
	"testing"
)

// Requests approved in Seerr that an older import left waiting move to
// approved on the next start; a request that was really waiting stays.
func TestImportedApprovedAsksRepaired(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, Options{MainMigrations: migrationsUpTo(t, 143)})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Write(context.Background(), func(tx *sql.Tx) error {
		for _, q := range []string{
			`INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash) VALUES (1, 't', '{}', 't', '{}', 'luis', 'owner', 'HASH')`,
			`INSERT INTO asks (created_at, updated_at, created_by, updated_by, kind, tmdb_id, name, state, asked_by, asked_at, note, ref) VALUES ('2026-08-01T00:00:00Z', '2026-08-01T00:00:00Z', 'system', 'system', 'movie', '1', 'A', 'asked', 1, '2026-08-01T00:00:00Z', 'approved in seerr: approve it again here to add it', 'tmdb:1')`,
			`INSERT INTO asks (created_at, updated_at, created_by, updated_by, kind, tmdb_id, name, state, asked_by, asked_at, note, ref) VALUES ('2026-08-01T00:00:00Z', '2026-08-01T00:00:00Z', 'system', 'system', 'movie', '2', 'B', 'asked', 1, '2026-08-01T00:00:00Z', 'asked by Bob in overseerr; approved in overseerr: approve it again here to add it', 'tmdb:2')`,
			`INSERT INTO asks (created_at, updated_at, created_by, updated_by, kind, tmdb_id, name, state, asked_by, asked_at, note, ref) VALUES ('2026-08-01T00:00:00Z', '2026-08-01T00:00:00Z', 'system', 'system', 'movie', '3', 'C', 'asked', 1, '2026-08-01T00:00:00Z', 'asked by Bob in seerr', 'tmdb:3')`,
		} {
			if _, err := tx.Exec(q); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		_ = s.Close()
		t.Fatal(err)
	}
	_ = s.Close()

	s, err = Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	want := map[string][2]string{"1": {"approved", ""}, "2": {"approved", "asked by Bob in overseerr"}, "3": {"asked", "asked by Bob in seerr"}}
	for id, w := range want {
		var state, note string
		var decided sql.NullString
		if err := s.Read().QueryRow(`SELECT state, note, decided_at FROM asks WHERE tmdb_id = ?`, id).Scan(&state, &note, &decided); err != nil {
			t.Fatal(err)
		}
		if state != w[0] || note != w[1] || (state == "approved") != decided.Valid {
			t.Errorf("ask %s: state %q note %q decided %v", id, state, note, decided)
		}
	}
}
