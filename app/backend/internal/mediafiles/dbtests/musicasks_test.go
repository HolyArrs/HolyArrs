package dbtests

import (
	"context"
	"database/sql"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// TestMusicAsksMigrationKeepsAsks writes a movie and a show ask with their
// people and messages before 0025, then checks the rebuild keeps them and
// that an artist ask with albums now fits.
func TestMusicAsksMigrationKeepsAsks(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	old, err := database.Open(dir, database.Options{MainMigrations: upTo(t, 24)})
	if err != nil {
		t.Fatal(err)
	}
	const meta = `'2026-10-01T00:00:00.000000000Z', '{"kind":"system"}', '2026-10-01T00:00:00.000000000Z', '{"kind":"system"}'`
	const at = `'2026-10-02T00:00:00.000000000Z'`
	err = old.Write(ctx, func(tx *sql.Tx) error {
		for _, q := range []string{
			`INSERT INTO users (id, created_at, created_by, updated_at, updated_by, name, role, state, password_hash) VALUES (10, ` + meta + `, 'ana', 'member', 'active', '')`,
			`INSERT INTO asks (id, created_at, created_by, updated_at, updated_by, kind, tmdb_id, name, state, asked_by, asked_at, ref, source_ids)
				VALUES (1, ` + meta + `, 'movie', '693134', 'Dune: Part Two', 'asked', 10, ` + at + `, 'tmdb:693134', '{"tmdb":"693134"}')`,
			`INSERT INTO asks (id, created_at, created_by, updated_at, updated_by, kind, tmdb_id, name, state, asked_by, asked_at, ref, source_ids, seasons)
				VALUES (2, ` + meta + `, 'show', '1399', 'Copper Valley', 'approved', 10, ` + at + `, 'tvmaze:82', '{"tvmaze":"82"}', '[1,2]')`,
			`INSERT INTO ask_people (ask_id, user_id, asked_at, counted) VALUES (1, 10, ` + at + `, 1), (2, 10, ` + at + `, 1)`,
			`INSERT INTO messages (id, created_at, user_id, kind, text, ask_id) VALUES (5, ` + at + `, 10, 'approved', 'Copper Valley was approved', 2)`,
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
	var show domain.Ask
	if err := database.Get(ctx, s.Read(), "asks", 2, &show); err != nil {
		t.Fatal(err)
	}
	if show.Kind != "show" || show.Ref != "tvmaze:82" || show.SourceIDs["tvmaze"] != "82" || len(show.Seasons) != 2 || show.Albums == nil || len(show.Albums) != 0 {
		t.Fatalf("show ask after the migration %+v", show)
	}
	var people, msgs int
	if err := s.Read().QueryRow("SELECT count(*) FROM ask_people").Scan(&people); err != nil || people != 2 {
		t.Fatalf("ask_people %d %v", people, err)
	}
	if err := s.Read().QueryRow("SELECT count(*) FROM messages WHERE ask_id = 2").Scan(&msgs); err != nil || msgs != 1 {
		t.Fatalf("messages %d %v", msgs, err)
	}
	a := domain.Ask{Kind: "artist", Ref: "mbid:a1", SourceIDs: map[string]string{"musicbrainz": "a1"}, Seasons: []int{}, Name: "Tin Lanterns",
		State: domain.AskAsked, AskedBy: 10, AskedAt: show.AskedAt, Albums: []domain.AskAlbum{{MBID: "rg1", Name: "Harbour Lights", Year: 2019}}}
	err = s.Write(ctx, func(tx *sql.Tx) error {
		_, err := database.Insert(tx, "asks", &a, domain.Actor{Kind: "system"})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	var back domain.Ask
	if err := database.Get(ctx, s.Read(), "asks", a.ID, &back); err != nil {
		t.Fatal(err)
	}
	if back.Kind != "artist" || len(back.Albums) != 1 || back.Albums[0].Name != "Harbour Lights" || back.Albums[0].Year != 2019 {
		t.Fatalf("artist ask %+v", back)
	}
}
