package dbtests

import (
	"context"
	"database/sql"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// TestShowAsksMigrationKeepsAsks writes movie asks, the people who asked
// and their messages with the schema before 0021, then checks that the
// rebuild keeps every row, links and all, and that a show ask now fits.
func TestShowAsksMigrationKeepsAsks(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	old, err := database.Open(dir, database.Options{MainMigrations: upTo(t, 20)})
	if err != nil {
		t.Fatal(err)
	}
	const meta = `'2026-10-01T00:00:00.000000000Z', '{"kind":"system"}', '2026-10-01T00:00:00.000000000Z', '{"kind":"system"}'`
	const at = `'2026-10-02T00:00:00.000000000Z'`
	err = old.Write(ctx, func(tx *sql.Tx) error {
		for _, q := range []string{
			`INSERT INTO users (id, created_at, created_by, updated_at, updated_by, name, role, state, password_hash) VALUES (10, ` + meta + `, 'ana', 'member', 'active', '')`,
			`INSERT INTO users (id, created_at, created_by, updated_at, updated_by, name, role, state, password_hash) VALUES (11, ` + meta + `, 'bo', 'member', 'active', '')`,
			`INSERT INTO asks (id, created_at, created_by, updated_at, updated_by, kind, tmdb_id, name, year, state, asked_by, asked_at)
				VALUES (1, ` + meta + `, 'movie', '693134', 'Dune: Part Two', 2024, 'asked', 10, ` + at + `)`,
			`INSERT INTO asks (id, created_at, created_by, updated_at, updated_by, kind, tmdb_id, name, state, asked_by, asked_at, note)
				VALUES (2, ` + meta + `, 'movie', '900002', 'Paper Harbour', 'turned_down', 11, ` + at + `, 'not now')`,
			`INSERT INTO ask_people (ask_id, user_id, asked_at, counted) VALUES (1, 10, ` + at + `, 1), (1, 11, ` + at + `, 0), (2, 11, ` + at + `, 1)`,
			`INSERT INTO messages (id, created_at, user_id, kind, text, ask_id) VALUES (5, ` + at + `, 11, 'turned_down', 'Paper Harbour was turned down', 2)`,
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

	s, err := database.Open(dir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var a domain.Ask
	if err := database.Get(ctx, s.Read(), "asks", 1, &a); err != nil {
		t.Fatal(err)
	}
	if a.Kind != "movie" || a.Name != "Dune: Part Two" || a.Ref != "tmdb:693134" || a.SourceIDs["tmdb"] != "693134" || len(a.Seasons) != 0 || a.Seasons == nil {
		t.Fatalf("ask after the migration %+v", a)
	}
	var people, msgs int
	if err := s.Read().QueryRow("SELECT count(*) FROM ask_people").Scan(&people); err != nil || people != 3 {
		t.Fatalf("ask_people %d %v", people, err)
	}
	var askID sql.NullInt64
	if err := s.Read().QueryRow("SELECT count(*), max(ask_id) FROM messages").Scan(&msgs, &askID); err != nil || msgs != 1 || askID.Int64 != 2 {
		t.Fatalf("messages %d %v %v", msgs, askID, err)
	}
	// The links still hold: removing an ask removes its people and leaves
	// its messages without the ask.
	err = s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("DELETE FROM asks WHERE id = 2")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Read().QueryRow("SELECT count(*) FROM ask_people WHERE ask_id = 2").Scan(&people); err != nil || people != 0 {
		t.Fatalf("people of a removed ask %d %v", people, err)
	}
	if err := s.Read().QueryRow("SELECT ask_id FROM messages WHERE id = 5").Scan(&askID); err != nil || askID.Valid {
		t.Fatalf("message of a removed ask %v %v", askID, err)
	}
	// A show ask fits, and two open asks for one movie still do not.
	err = s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO asks (created_at, created_by, updated_at, updated_by, kind, tmdb_id, name, state, asked_by, asked_at, ref, source_ids, seasons)
			VALUES (` + meta + `, 'show', '1399', 'Lab Show', 'asked', 10, ` + at + `, 'tmdb:1399', '{"tmdb":"1399"}', '[1,2]')`)
		return err
	})
	if err != nil {
		t.Fatalf("show ask: %v", err)
	}
	err = s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO asks (created_at, created_by, updated_at, updated_by, kind, tmdb_id, name, state, asked_by, asked_at)
			VALUES (` + meta + `, 'movie', '693134', 'Dune again', 'asked', 11, ` + at + `)`)
		return err
	})
	if err == nil {
		t.Fatal("a second open ask for one movie was let in")
	}
}
