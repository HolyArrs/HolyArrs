package database

import (
	"context"
	"database/sql"
	"testing"
)

// TestExploreCategoryCountsMigration upgrades a database from before the
// strip counts: the table is added (add-only, explore_counts untouched),
// counts add up per member, category and day, and a member's rows go with
// the member.
func TestExploreCategoryCountsMigration(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	old, err := Open(dir, Options{MainMigrations: upTo(t, 56)})
	if err != nil {
		t.Fatal(err)
	}
	err = old.Write(ctx, func(tx *sql.Tx) error {
		for _, q := range []string{
			`INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash) VALUES (2, 't', '{}', 't', '{}', 'anna', 'member', '')`,
			`INSERT INTO explore_counts (member_id, row_type, day, shown) VALUES (2, 'hero', '2026-10-01', 3)`,
		} {
			if _, err := tx.Exec(q); err != nil {
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
	err = s.Write(ctx, func(tx *sql.Tx) error {
		for i := 0; i < 2; i++ {
			if _, err := tx.Exec(`INSERT INTO explore_category_counts (member_id, category_id, day, shown) VALUES (2, 'heists', '2026-10-02', 1)
				ON CONFLICT (member_id, category_id, day) DO UPDATE SET shown = shown + 1`); err != nil {
				return err
			}
		}
		_, err := tx.Exec(`UPDATE explore_category_counts SET opened = opened + 1 WHERE member_id = 2`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	var shown, opened, kept int
	if err := s.Read().QueryRow("SELECT shown, opened FROM explore_category_counts WHERE member_id = 2 AND category_id = 'heists'").Scan(&shown, &opened); err != nil {
		t.Fatal(err)
	}
	if shown != 2 || opened != 1 {
		t.Fatalf("shown %d opened %d", shown, opened)
	}
	if err := s.Read().QueryRow("SELECT shown FROM explore_counts WHERE member_id = 2").Scan(&kept); err != nil || kept != 3 {
		t.Fatalf("explore_counts changed: %d %v", kept, err)
	}
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("DELETE FROM users WHERE id = 2")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = s.Read().QueryRow("SELECT count(*) FROM explore_category_counts").Scan(&n)
	if n != 0 {
		t.Fatalf("%d rows outlive the member", n)
	}
}
