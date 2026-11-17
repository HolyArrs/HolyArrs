package database

import (
	"context"
	"database/sql"
	"testing"
)

// 0125 gives every existing API key the default AI tick boxes (all but
// Delete and Admin) and keeps the key as it was.
func TestMCPKeyGroupsMigration(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	old, err := Open(dir, Options{MainMigrations: upTo(t, 124)})
	if err != nil {
		t.Fatal(err)
	}
	err = old.Write(ctx, func(tx *sql.Tx) error {
		for _, q := range []string{
			`INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash) VALUES (1, 't', '{}', 't', '{}', 'maria', 'owner', 'HASH')`,
			`INSERT INTO api_keys(id, created_at, created_by, updated_at, updated_by, owner_user_id, label, scope, hash, hint) VALUES
				(1, 't', '{}', 't', '{}', 1, 'Claude', '["mcp"]', 'h1', 'abcd')`,
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
	s, err := Open(dir, Options{MainMigrations: upTo(t, 125)})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var label, scope, groups string
	if err := s.Read().QueryRow("SELECT label, scope, mcp_groups FROM api_keys WHERE id = 1").Scan(&label, &scope, &groups); err != nil {
		t.Fatal(err)
	}
	if label != "Claude" || scope != `["mcp"]` || groups != `["see","add","control","settings","delete","admin"]` {
		t.Fatalf("key after 0125: %q %q %q", label, scope, groups)
	}
}
