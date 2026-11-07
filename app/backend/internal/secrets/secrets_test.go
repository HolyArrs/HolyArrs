package secrets

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/store"
)

func openStore(t *testing.T, dir string) *store.Store {
	t.Helper()
	s, err := store.Open(dir, store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestKeyFileMadeOnceWithTightMode(t *testing.T) {
	dir := t.TempDir()
	k1, err := LoadKey(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(k1) != KeySize {
		t.Fatalf("key size %d", len(k1))
	}
	st, err := os.Stat(filepath.Join(dir, KeyFile))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && st.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", st.Mode().Perm())
	}
	k2, err := LoadKey(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if string(k1) != string(k2) {
		t.Fatal("key changed between starts")
	}
}

func TestKeyFromEnvFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "mk")
	if err := os.WriteFile(f, []byte(strings.Repeat("ab", 32)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	k, err := LoadKey(t.TempDir(), f)
	if err != nil || len(k) != KeySize || k[0] != 0xab {
		t.Fatalf("key %x %v", k, err)
	}
	if _, err := LoadKey(t.TempDir(), filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("a named key file that is missing must fail, not make a new key")
	}
	if err := os.WriteFile(f, []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadKey(t.TempDir(), f); err == nil {
		t.Fatal("bad key accepted")
	}
}

func TestPutGetEncryptedAtRest(t *testing.T) {
	dir := t.TempDir()
	s := openStore(t, dir)
	key, _ := LoadKey(dir, "")
	v, err := Open(context.Background(), s, key)
	if err != nil {
		t.Fatal(err)
	}
	const plain = "hunter2-lab-value"
	id, err := v.Put(context.Background(), "source_api_key", plain)
	if err != nil {
		t.Fatal(err)
	}
	got, err := v.Get(context.Background(), id)
	if err != nil || got != plain {
		t.Fatalf("got %q %v", got, err)
	}
	var stored string
	_ = s.Read().QueryRow("SELECT value FROM secrets WHERE id = ?", id).Scan(&stored)
	if strings.Contains(stored, plain) || !strings.HasPrefix(stored, "v1:") {
		t.Fatalf("stored %q", stored)
	}
	if err := v.Set(context.Background(), id, "new value"); err != nil {
		t.Fatal(err)
	}
	if got, _ := v.Get(context.Background(), id); got != "new value" {
		t.Fatalf("after set %q", got)
	}
	if err := v.Delete(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Get(context.Background(), id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after delete %v", err)
	}
}

func TestWrongKeyRefused(t *testing.T) {
	dir := t.TempDir()
	s := openStore(t, dir)
	key, _ := LoadKey(dir, "")
	if _, err := Open(context.Background(), s, key); err != nil {
		t.Fatal(err)
	}
	other := make([]byte, KeySize)
	if _, err := Open(context.Background(), s, other); !errors.Is(err, ErrWrongKey) {
		t.Fatalf("wrong key: %v", err)
	}
}

func TestMask(t *testing.T) {
	if Mask("") != "" || Mask("abc") != "••••••••" || strings.Contains(Mask("supersecretvalue"), "secret") {
		t.Fatalf("mask %q %q", Mask("abc"), Mask("supersecretvalue"))
	}
}

func TestMigratesPlaintext(t *testing.T) {
	dir := t.TempDir()
	s := openStore(t, dir)
	ctx := context.Background()
	err := s.Write(ctx, func(tx *sql.Tx) error {
		if _, err := store.Insert(tx, "sources", &domain.Source{Name: "Old", Protocol: "torrent", Format: "torznab", URL: "http://x/api",
			Kinds: []string{"movie"}, Categories: map[string][]int{}, State: "enabled",
			Extra: map[string]string{"apikey": "OLD_PLAIN_KEY", "lang": "en"}}, domain.System); err != nil {
			return err
		}
		_, err := tx.Exec(`INSERT INTO settings(grp, key, value, created_at, created_by, updated_at, updated_by)
			VALUES ('links', 'tmdb_token', 'plain-token-2', 'x', '{}', 'x', '{}')`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	key, _ := LoadKey(dir, "")
	v, err := Open(ctx, s, key)
	if err != nil {
		t.Fatal(err)
	}
	n, err := v.MigratePlaintext(ctx)
	if err != nil || n != 2 {
		t.Fatalf("moved %d %v", n, err)
	}
	var src domain.Source
	if err := store.Get(ctx, s.Read(), "sources", 1, &src); err != nil {
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

func TestSetSettingStoresEncrypted(t *testing.T) {
	dir := t.TempDir()
	s := openStore(t, dir)
	key, _ := LoadKey(dir, "")
	ctx := context.Background()
	v, _ := Open(ctx, s, key)
	if err := v.SetSetting(ctx, "links", "tmdb_token", "tok-a", domain.System); err != nil {
		t.Fatal(err)
	}
	if err := v.SetSetting(ctx, "links", "tmdb_token", "tok-b", domain.System); err != nil {
		t.Fatal(err)
	}
	if got, _ := v.Setting(ctx, "links", "tmdb_token"); got != "tok-b" {
		t.Fatalf("got %q", got)
	}
	var n int
	_ = s.Read().QueryRow("SELECT count(*) FROM secrets").Scan(&n)
	if n != 2 { // the check value plus one secret, reused on change
		t.Fatalf("%d secret rows", n)
	}
}

func TestCheckFile(t *testing.T) {
	dir := t.TempDir()
	s := openStore(t, dir)
	key, _ := LoadKey(dir, "")
	if _, err := Open(context.Background(), s, key); err != nil {
		t.Fatal(err)
	}
	m, _, err := s.Snapshot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckFile(m, key); err != nil {
		t.Fatalf("own key: %v", err)
	}
	if err := CheckFile(m, make([]byte, KeySize)); !errors.Is(err, ErrWrongKey) {
		t.Fatalf("other key: %v", err)
	}
}
