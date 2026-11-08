package database

import (
	"context"
	"database/sql"
	"testing"
)

// Migration 0026 moves artists to the music "Lossless first" rules only: a
// person's own movie rules that happen to carry that name are never given to
// an artist (the music preset cannot be added under a taken name, so the
// artist keeps its rules for the person to change).
func TestMusicRulesMigrationSkipsMovieRulesOfTheSameName(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	old, err := Open(dir, Options{MainMigrations: upTo(t, 25)})
	if err != nil {
		t.Fatal(err)
	}
	const meta = `'2026-10-01T00:00:00.000000000Z', '{"kind":"system"}', '2026-10-01T00:00:00.000000000Z', '{"kind":"system"}'`
	var before int64
	err = old.Write(ctx, func(tx *sql.Tx) error {
		for _, q := range []string{
			`INSERT INTO pick_rules (id, created_at, created_by, updated_at, updated_by, name, kinds, revision, preset, body)
				SELECT 900, ` + meta + `, 'Lossless first', '["movie"]', 1, 0, body FROM pick_rules ORDER BY id LIMIT 1`,
			`INSERT INTO library_folders (id, created_at, created_by, updated_at, updated_by, path, kind, label, health) VALUES (1, ` + meta + `, 'C:/music', 'music', 'Music', 'ok')`,
			`INSERT INTO titles (id, created_at, created_by, updated_at, updated_by, kind, name, sort_name, source_ids, metadata_source, why_added, status, state, gate_driver)
				VALUES (1, ` + meta + `, 'artist', 'Lab Band', 'lab band', '{"musicbrainz":"a-1"}', 'musicbrainz', 'by_hand', 'running', 'active', 'none')`,
			`INSERT INTO versions (id, created_at, created_by, updated_at, updated_by, title_id, label, pick_rules_id, library_folder_id, folder_name, is_main)
				VALUES (1, ` + meta + `, 1, 'Main', (SELECT min(id) FROM pick_rules), 1, 'Lab Band', 1)`,
		} {
			if _, err := tx.Exec(q); err != nil {
				return err
			}
		}
		return tx.QueryRow("SELECT pick_rules_id FROM versions WHERE id = 1").Scan(&before)
	})
	_ = old.Close()
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(dir, Options{MainMigrations: upTo(t, 26)})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var after int64
	if err := s.Read().QueryRow("SELECT pick_rules_id FROM versions WHERE id = 1").Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after == 900 || after != before {
		t.Fatalf("artist rules %d -> %d (900 is the movie rules named Lossless first)", before, after)
	}
}
