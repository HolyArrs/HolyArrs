package database

import (
	"context"
	"database/sql"
	"strconv"
	"testing"
)

// TestFollowAsksMigration: people who asked for a title now in the Library
// follow it after 0061; open asks and removed titles are left alone, and
// new askers follow by default.
func TestFollowAsksMigration(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	old, err := Open(dir, Options{MainMigrations: upTo(t, 60)})
	if err != nil {
		t.Fatal(err)
	}
	const meta = `'2026-10-01T00:00:00.000000000Z', '{"kind":"system"}', '2026-10-01T00:00:00.000000000Z', '{"kind":"system"}'`
	const at = `'2026-10-02T00:00:00.000000000Z'`
	err = old.Write(ctx, func(tx *sql.Tx) error {
		// Plain SQL: today's Title has columns schema 60 does not.
		var ids []int64
		for _, st := range []string{"active", "removed"} {
			r, err := tx.Exec(`INSERT INTO titles (created_at, created_by, updated_at, updated_by, kind, gate_driver, new_parts_policy, release_gate, why_added, status, name, sort_name, state, metadata_source)
				VALUES (`+meta+`, 'movie', 'none', 'all', 'released', 'by_hand', 'released', ?, ?, ?, 'tmdb')`, "Film "+st, "film "+st, st)
			if err != nil {
				return err
			}
			id, err := r.LastInsertId()
			if err != nil {
				return err
			}
			ids = append(ids, id)
		}
		for _, q := range []string{
			`INSERT INTO users (id, created_at, created_by, updated_at, updated_by, name, role, state, password_hash) VALUES (10, ` + meta + `, 'ana', 'member', 'active', ''), (11, ` + meta + `, 'bo', 'member', 'active', '')`,
			`INSERT INTO asks (id, created_at, created_by, updated_at, updated_by, kind, tmdb_id, name, state, asked_by, asked_at, ref, source_ids, title_id)
				VALUES (1, ` + meta + `, 'movie', '1', 'Film wanted', 'approved', 10, ` + at + `, 'tmdb:1', '{"tmdb":"1"}', ` + strconv.FormatInt(ids[0], 10) + `),
				(2, ` + meta + `, 'movie', '2', 'Film removed', 'available', 10, ` + at + `, 'tmdb:2', '{"tmdb":"2"}', ` + strconv.FormatInt(ids[1], 10) + `),
				(3, ` + meta + `, 'movie', '3', 'Film open', 'asked', 11, ` + at + `, 'tmdb:3', '{"tmdb":"3"}', NULL)`,
			`INSERT INTO ask_people (ask_id, user_id, asked_at, counted) VALUES (1, 10, ` + at + `, 1), (1, 11, ` + at + `, 1), (2, 10, ` + at + `, 1), (3, 11, ` + at + `, 1)`,
		} {
			if _, err := tx.Exec(q); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		_ = old.Close()
		t.Fatal(err)
	}
	_ = old.Close()
	s, err := Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var n, ana, bo, on int
	if err := s.Read().QueryRow(`SELECT count(*), sum(user_id = 10), sum(user_id = 11) FROM follows`).Scan(&n, &ana, &bo); err != nil || n != 2 || ana != 1 || bo != 1 {
		t.Fatalf("follows %d (ana %d, bo %d) %v", n, ana, bo, err)
	}
	if err := s.Read().QueryRow(`SELECT min(follow) FROM ask_people`).Scan(&on); err != nil || on != 1 {
		t.Fatalf("askers follow by default: %d %v", on, err)
	}
}
