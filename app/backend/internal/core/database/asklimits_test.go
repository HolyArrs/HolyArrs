package database

import (
	"context"
	"database/sql"
	"strings"
	"testing"
)

// TestAskLimitsPerKindMigration upgrades a database from the shared weekly
// limit (up to 0032) to one limit per kind: each kind takes the old number,
// a kind that did not count gets 0 (no limit), and a
// person's own limit keeps working.
func TestAskLimitsPerKindMigration(t *testing.T) {
	for _, c := range []struct {
		name                 string
		settings             []string // key=value in this_app
		movies, shows, music string   // "" when no row is written (the code default 10 applies)
		annaShows, annaMusic sql.NullInt64
	}{
		{name: "shows off", settings: []string{"ask_per_week=3", "ask_shows_count=false", "ask_music_count=true"}, movies: "3", shows: "0", music: "3", annaShows: sql.NullInt64{Int64: 0, Valid: true}},
		{name: "all count", settings: []string{"ask_per_week=7"}, movies: "7", shows: "7", music: "7"},
		{name: "never saved", settings: nil},
		{name: "music off, no number", settings: []string{"ask_music_count=false"}, music: "0", annaMusic: sql.NullInt64{Int64: 0, Valid: true}},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			ctx := context.Background()
			old, err := Open(dir, Options{MainMigrations: upTo(t, 32)})
			if err != nil {
				t.Fatal(err)
			}
			err = old.Write(ctx, func(tx *sql.Tx) error {
				qs := []string{
					`INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash) VALUES (2, 't', '{}', 't', '{}', 'anna', 'member', '')`,
					`INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash) VALUES (3, 't', '{}', 't', '{}', 'bo', 'member', '')`,
					`INSERT INTO user_ask_settings (user_id, per_week) VALUES (2, 5)`,
					`INSERT INTO user_ask_settings (user_id, per_week, email) VALUES (3, NULL, 'bo@example.com')`,
				}
				for _, q := range qs {
					if _, err := tx.Exec(q); err != nil {
						return err
					}
				}
				for _, kv := range c.settings {
					k, v, _ := strings.Cut(kv, "=")
					if _, err := tx.Exec(`INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('this_app', ?, ?, 't', '{}', 't', '{}')`, k, v); err != nil {
						return err
					}
				}
				return nil
			})
			_ = old.Close()
			if err != nil {
				t.Fatal(err)
			}
			s, err := Open(dir, Options{})
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			got := func(key string) string {
				var v string
				_ = s.Read().QueryRow("SELECT value FROM settings WHERE grp = 'this_app' AND key = ?", key).Scan(&v)
				return v
			}
			if m, sh, mu := got("ask_movies_per_week"), got("ask_shows_per_week"), got("ask_music_per_week"); m != c.movies || sh != c.shows || mu != c.music {
				t.Fatalf("per kind: movies %q shows %q music %q", m, sh, mu)
			}
			var all, m, sh, mu sql.NullInt64
			if err := s.Read().QueryRow("SELECT per_week, movies_per_week, shows_per_week, music_per_week FROM user_ask_settings WHERE user_id = 2").Scan(&all, &m, &sh, &mu); err != nil {
				t.Fatal(err)
			}
			if all.Int64 != 5 || m.Valid || sh != c.annaShows || mu != c.annaMusic {
				t.Fatalf("anna: all %v movies %v shows %v music %v", all, m, sh, mu)
			}
			var email string
			if err := s.Read().QueryRow("SELECT email FROM user_ask_settings WHERE user_id = 3 AND per_week IS NULL AND shows_per_week IS NULL").Scan(&email); err != nil || email != "bo@example.com" {
				t.Fatalf("bo: %q %v", email, err)
			}
		})
	}
}
