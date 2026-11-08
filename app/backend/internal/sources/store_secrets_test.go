package sources

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

func openMigrationStore(t *testing.T, dir string) *database.Store {
	t.Helper()
	s, err := database.Open(dir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestMigratesPlaintext(t *testing.T) {
	dir := t.TempDir()
	s := openMigrationStore(t, dir)
	ctx := context.Background()
	err := s.Write(ctx, func(tx *sql.Tx) error {
		if _, err := database.Insert(tx, "sources", &domain.Source{Name: "Old", Protocol: "torrent", Format: "torznab", URL: "http://x/api",
			Kinds: []string{"movie"}, Categories: map[string][]int{}, State: "enabled",
			Extra: map[string]string{"apikey": "OLD_PLAIN_KEY", "lang": "en"}}, database.System); err != nil {
			return err
		}
		_, err := tx.Exec(`INSERT INTO settings(grp, key, value, created_at, created_by, updated_at, updated_by)
			VALUES ('links', 'tmdb_token', 'plain-token-2', 'x', '{}', 'x', '{}')`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	key, _ := secrets.LoadKey(dir, "")
	v, err := secrets.Open(ctx, s, key)
	if err != nil {
		t.Fatal(err)
	}
	secrets.RegisterRewriter("sources", EncryptPlainExtras)
	n, err := v.MigratePlaintext(ctx)
	if err != nil || n != 2 {
		t.Fatalf("moved %d %v", n, err)
	}
	var src domain.Source
	if err := database.Get(ctx, s.Read(), "sources", 1, &src); err != nil {
		t.Fatal(err)
	}
	if _, left := src.Extra["apikey"]; left || src.Extra["lang"] != "en" || src.SecretRef == nil {
		t.Fatalf("source after: %+v", src)
	}
	if got, _ := v.Get(ctx, *src.SecretRef); got != "OLD_PLAIN_KEY" {
		t.Fatalf("moved key %q", got)
	}
	tok, err := v.Setting(ctx, "links", "tmdb_token")
	if err != nil || tok != "plain-token-2" {
		t.Fatalf("token %q %v", tok, err)
	}
	var raw string
	_ = s.Read().QueryRow("SELECT value FROM settings WHERE key = 'tmdb_token'").Scan(&raw)
	if strings.Contains(raw, "plain-token-2") {
		t.Fatalf("setting still plain: %q", raw)
	}
	if n, _ := v.MigratePlaintext(ctx); n != 0 {
		t.Fatalf("second run moved %d", n)
	}
}
