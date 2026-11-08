package main

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

// registerHooks must hook the Source rewriter into the vault, so an
// upgrade moves plain indexer keys out of the extras column.
func TestRegisterHooksEncryptsSourceExtras(t *testing.T) {
	registerHooks()
	dir := t.TempDir()
	s, err := database.Open(dir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := database.Insert(tx, "sources", &domain.Source{Name: "Old", Protocol: "torrent", Format: "torznab", URL: "http://x/api",
			Kinds: []string{"movie"}, Categories: map[string][]int{}, State: "enabled",
			Extra: map[string]string{"apikey": testPlainKey}}, database.System)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	key, err := secrets.LoadKey(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	v, err := secrets.Open(ctx, s, key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.MigratePlaintext(ctx); err != nil {
		t.Fatal(err)
	}
	var raw string
	if err := s.Read().QueryRowContext(ctx, "SELECT extra FROM sources").Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, testPlainKey) {
		t.Fatalf("extras not encrypted: %s", raw)
	}
}

// Module routes sit behind sign-in, under the URL base, and under the body
// limit once an owner exists (the full routes() stack of a running app).
func TestRoutesModuleRoutesNeedAuthAndURLBase(t *testing.T) {
	c, err := parseFlags([]string{"-config", t.TempDir(), "-url-base", "holy"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	addr, _, _, _ := runApp(t, c)
	base := "http://" + addr
	owner, anon := newClient(), newClient()
	do := func(cl *http.Client, method, path, body string) (int, string) {
		t.Helper()
		req, _ := http.NewRequest(method, base+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := cl.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, resp.Header.Get("Content-Type") + " " + string(b)
	}
	if st, b := do(owner, "POST", "/holy/api/v1/auth/setup", `{"name":"luis","password":"a long test passphrase"}`); st != 201 {
		t.Fatalf("setup: %d %s", st, b)
	}
	if st, _ := do(anon, "GET", "/holy/api/v1/sources", ""); st != http.StatusUnauthorized {
		t.Errorf("sources with no sign-in: %d", st)
	}
	if st, b := do(anon, "GET", "/api/v1/sources", ""); st == 200 && strings.HasPrefix(b, "application/json") {
		t.Errorf("sources answered outside the base")
	}
	big := `{"name":"` + strings.Repeat("x", maxBody+10) + `"}`
	if st, _ := do(anon, "POST", "/holy/api/v1/sources", big); st != http.StatusRequestEntityTooLarge {
		t.Errorf("oversized body: %d", st)
	}
}

// testPlainKey is a fake key the start-up hook must move out of extras.
var testPlainKey = "OLD_PLAIN_KEY"
