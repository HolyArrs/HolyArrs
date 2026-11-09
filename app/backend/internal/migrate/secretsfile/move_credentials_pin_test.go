package secretsfile

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func pinCount(t *testing.T, d Deps, q string, args ...any) int {
	t.Helper()
	var n int
	if err := d.Store.Read().QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// move_credentials points at one vault secret per old app. Today: a save
// replaces the old secret (never a second one), the list is in MoveKinds
// order with no key in it, forgetting one app or all removes the secret too,
// and a consumed credential cannot be consumed twice.
func TestPinMoveCredentialStore(t *testing.T) {
	d := deps(t)
	ctx := context.Background()
	by := domain.Actor{Kind: "user", ID: 1}
	// Other secrets in the vault (a crowd) stay untouched by every call.
	for i := 0; i < 25; i++ {
		if err := d.Store.Write(ctx, func(tx *sql.Tx) error { _, err := d.Vault.PutTx(tx, "other", "keep-me", domain.System); return err }); err != nil {
			t.Fatal(err)
		}
	}
	others := pinCount(t, d, "SELECT count(*) FROM secrets")
	save := func(kind, addr, key string) {
		t.Helper()
		if err := SaveMoveCredential(ctx, d.Store, d.Vault, kind, MoveCredential{Address: addr, APIKey: key, AllowLocalNetwork: kind == "sonarr"}, by); err != nil {
			t.Fatalf("save %s: %v", kind, err)
		}
	}
	save("sonarr", "HTTP://Sonarr.LAN:8989/", "key-1")
	save("radarr", "http://radarr:7878", "key-2")
	save("seerr", "https://seerr.example/base/", "key-3")
	if n := pinCount(t, d, "SELECT count(*) FROM secrets"); n != others+3 {
		t.Fatalf("secrets %d, want %d", n, others+3)
	}
	// Replace: still one row per app and one secret.
	save("sonarr", "http://sonarr2:8989", "key-1b")
	if n := pinCount(t, d, "SELECT count(*) FROM move_credentials"); n != 3 {
		t.Fatalf("rows %d", n)
	}
	if n := pinCount(t, d, "SELECT count(*) FROM secrets"); n != others+3 {
		t.Fatalf("secrets after replace %d", n)
	}
	c, ok, err := ReadMoveCredential(ctx, d.Store, d.Vault, "sonarr")
	if err != nil || !ok || c.APIKey != "key-1b" || c.Address != "http://sonarr2:8989" || !c.AllowLocalNetwork {
		t.Fatalf("read %+v %v %v", c, ok, err)
	}
	// Address is stored lower-case host, no trailing slash, path kept.
	if c, _, _ := ReadMoveCredential(ctx, d.Store, d.Vault, "seerr"); c.Address != "https://seerr.example/base" {
		t.Fatalf("seerr address %q", c.Address)
	}
	// List: MoveKinds order (radarr, sonarr, ..., seerr), never a key.
	list, err := ListMoveCredentials(ctx, d.Store, d.Vault)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(list)
	if string(b) != `[{"kind":"radarr","address":"http://radarr:7878","allow_local_network":false},{"kind":"sonarr","address":"http://sonarr2:8989","allow_local_network":true},{"kind":"seerr","address":"https://seerr.example/base","allow_local_network":false}]` {
		t.Fatalf("list %s", b)
	}
	if strings.Contains(string(b), "key-") {
		t.Fatal("a key in the list")
	}
	// An unknown app, and a read of a kind with nothing saved.
	if err := SaveMoveCredential(ctx, d.Store, d.Vault, "plex", MoveCredential{Address: "http://x"}, by); err == nil {
		t.Fatal("unknown app saved")
	}
	if _, ok, err := ReadMoveCredential(ctx, d.Store, d.Vault, "bazarr"); ok || err != nil {
		t.Fatalf("bazarr %v %v", ok, err)
	}
	// Forget one, then all; the others' secrets stay.
	if n, err := ForgetMoveCredential(ctx, d.Store, "sonarr"); err != nil || n != 1 {
		t.Fatalf("forget one: %d %v", n, err)
	}
	if n, err := ForgetMoveCredential(ctx, d.Store, "sonarr"); err != nil || n != 0 {
		t.Fatalf("forget again: %d %v", n, err)
	}
	if n, err := ForgetMoveCredential(ctx, d.Store, ""); err != nil || n != 2 {
		t.Fatalf("forget all: %d %v", n, err)
	}
	if n := pinCount(t, d, "SELECT count(*) FROM secrets"); n != others {
		t.Fatalf("secrets at the end %d, want %d", n, others)
	}
}
