package database

import (
	"context"
	"database/sql"
	"testing"
)

// 0111 adds the machine-made switch to Subtitle sites and the made mark to
// subtitles: a site from an older build keeps every field with the switch
// off, and an older subtitle reads as a person's.
func TestSubtitleMachineMadeMigration(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	old, err := Open(dir, Options{MainMigrations: upTo(t, 110)})
	if err != nil {
		t.Fatal(err)
	}
	err = old.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO subtitle_sites (id, created_at, created_by, updated_at, updated_by, name, kind, url, priority)
			VALUES (3, 'c', '{"kind":"system"}', 'u', '{"kind":"system"}', 'Old site', 'opensubtitles', 'https://api.example.test/v1', 7)`)
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
	var name string
	var prio, machine int
	if err := s.Read().QueryRow(`SELECT name, priority, machine_made FROM subtitle_sites WHERE id = 3`).Scan(&name, &prio, &machine); err != nil {
		t.Fatal(err)
	}
	if name != "Old site" || prio != 7 || machine != 0 {
		t.Fatalf("old site %q %d machine_made %d", name, prio, machine)
	}
	var cols int
	if err := s.Read().QueryRow(`SELECT count(*) FROM pragma_table_info('subtitles') WHERE name = 'made' AND "notnull" = 0`).Scan(&cols); err != nil || cols != 1 {
		t.Fatalf("made column: %d %v", cols, err)
	}
}
