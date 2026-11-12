package database

import (
	"context"
	"database/sql"
	"testing"
)

// 0107 adds the chosen episode order to titles: a show from an older build
// keeps its fields and follows TMDB's own numbering (NULL).
func TestEpisodeGroupMigration(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	old, err := Open(dir, Options{MainMigrations: upTo(t, 104)})
	if err != nil {
		t.Fatal(err)
	}
	err = old.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO titles (id, created_at, created_by, updated_at, updated_by, kind, name, sort_name, metadata_source, why_added, episode_order)
			VALUES (7, 'c', '{"kind":"system"}', 'u', '{"kind":"system"}', 'show', 'Old Show', 'old show', 'tmdb', 'by_hand', 'aired')`)
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
	var name, order string
	var group sql.NullString
	if err := s.Read().QueryRow(`SELECT name, episode_order, episode_group_id FROM titles WHERE id = 7`).Scan(&name, &order, &group); err != nil {
		t.Fatal(err)
	}
	if name != "Old Show" || order != "aired" || group.Valid {
		t.Fatalf("old show %q %q group %v", name, order, group)
	}
}
