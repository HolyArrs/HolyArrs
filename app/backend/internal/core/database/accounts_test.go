package database

import (
	"context"
	"database/sql"
	"io/fs"
	"testing"
	"testing/fstest"
)

// upTo is the main migration set up to and including version v.
func upTo(t *testing.T, v int) fs.FS {
	t.Helper()
	src, _ := fs.Sub(embedded, "migrations/main")
	ms, err := loadMigrations(src)
	if err != nil {
		t.Fatal(err)
	}
	out := fstest.MapFS{}
	for _, m := range ms {
		if m.version <= v {
			out[m.name] = &fstest.MapFile{Data: []byte(m.sql)}
		}
	}
	return out
}

// TestAccountsKeepTheOwner upgrades a database from before accounts: the
// owner, their password, sessions and API keys all survive, and the new
// roles, links, invites and backup codes work.
func TestAccountsKeepTheOwner(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	old, err := Open(dir, Options{MainMigrations: upTo(t, 7)})
	if err != nil {
		t.Fatal(err)
	}
	err = old.Write(ctx, func(tx *sql.Tx) error {
		for _, q := range []string{
			`INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash) VALUES (1, 't', '{}', 't', '{}', 'luis', 'owner', 'HASH')`,
			`INSERT INTO sessions(hash, user_id, csrf, created_at, rotated_at, last_seen_at, expires_at) VALUES ('s1', 1, 'c', 't', 't', 't', 't')`,
			`INSERT INTO api_keys(created_at, created_by, updated_at, updated_by, owner_user_id, label, hash, hint) VALUES ('t', '{}', 't', '{}', 1, 'k', 'h1', 'abcd')`,
		} {
			if _, err := tx.Exec(q); err != nil {
				return err
			}
		}
		return nil
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
	var name, role, hash, perms string
	if err := s.Read().QueryRow("SELECT name, role, password_hash, perms FROM users WHERE id = 1").Scan(&name, &role, &hash, &perms); err != nil {
		t.Fatal(err)
	}
	if name != "luis" || role != "owner" || hash != "HASH" {
		t.Fatalf("owner changed: %s %s %s", name, role, hash)
	}
	var n int
	_ = s.Read().QueryRow("SELECT (SELECT count(*) FROM sessions WHERE user_id = 1) + (SELECT count(*) FROM api_keys WHERE owner_user_id = 1)").Scan(&n)
	if n != 2 {
		t.Fatalf("sessions and keys lost: %d left", n)
	}
	err = s.Write(ctx, func(tx *sql.Tx) error {
		for _, q := range []string{
			`INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash, perms) VALUES (2, 't', '{}', 't', '{}', 'ana', 'admin', 'x', '["approve"]')`,
			`INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash) VALUES (3, 't', '{}', 't', '{}', 'kid', 'member', '')`,
			`INSERT INTO user_links(user_id, provider, external_id, external_name, created_at) VALUES (3, 'plex', '42', 'kid', 't')`,
			`INSERT INTO invites(hash, created_at, created_by, role, perms, expires_at) VALUES ('ih', 't', 1, 'member', '[]', 't')`,
			`INSERT INTO backup_codes(user_id, hash) VALUES (2, 'bh')`,
		} {
			if _, err := tx.Exec(q); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO users(created_at, created_by, updated_at, updated_by, name, role, password_hash) VALUES ('t', '{}', 't', '{}', 'two', 'owner', 'x')`,
		`INSERT INTO users(created_at, created_by, updated_at, updated_by, name, role, password_hash) VALUES ('t', '{}', 't', '{}', 'boss', 'superuser', 'x')`,
		`INSERT INTO user_links(user_id, provider, external_id, external_name, created_at) VALUES (2, 'plex', '42', 'x', 't')`,
	} {
		if err := s.Write(ctx, func(tx *sql.Tx) error { _, err := tx.Exec(q); return err }); err == nil {
			t.Fatalf("allowed: %s", q)
		}
	}
	if fk := s.Read().QueryRow("SELECT count(*) FROM pragma_foreign_key_check").Scan(&n); fk != nil || n != 0 {
		t.Fatalf("foreign key check: %v %d", fk, n)
	}
}
