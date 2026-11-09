package mediaservers

import (
	"context"
	"errors"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// Removing a media server also deletes its token from the vault.
func TestRemoveDeletesVaultToken(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	m, err := Add(ctx, w.s, w.v, Input{Name: "p", Kind: "plex", URL: "http://10.0.0.9:32400", Token: "tk"}, domain.System)
	if err != nil || m.TokenSecretID == nil {
		t.Fatal(err, m)
	}
	sid := *m.TokenSecretID
	if err := Remove(ctx, w.s, w.v, m.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := w.v.Get(ctx, sid); err == nil {
		t.Fatal("token kept after remove")
	}
	if _, err := getServer(ctx, w.s.Read(), m.ID); !errors.Is(err, database.ErrNotFound) {
		t.Fatal(err)
	}
}

// A new token replaces the stored one; the old secret is gone.
func TestChangeNewTokenReplacesSecret(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	m, err := Add(ctx, w.s, w.v, Input{Name: "p", Kind: "plex", URL: "http://10.0.0.9:32400", Token: "a"}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	old := *m.TokenSecretID
	m2, err := Change(ctx, w.s, w.v, m.ID, Input{Name: "p", Token: "b"}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if m2.TokenSecretID == nil {
		t.Fatal("no secret id")
	}
	// The freed id may be reused, so check the old value is gone instead.
	if *m2.TokenSecretID != old {
		if _, err := w.v.Get(ctx, old); err == nil {
			t.Fatal("old token kept")
		}
	}
	vals, err := w.v.Values(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range vals {
		if x == "a" {
			t.Fatal("old token kept")
		}
	}
	if tok, err := w.v.Get(ctx, *m2.TokenSecretID); err != nil || tok != "b" {
		t.Fatal(tok, err)
	}
	got, err := getServer(ctx, w.s.Read(), m.ID)
	if err != nil || got.TokenSecretID == nil || *got.TokenSecretID != *m2.TokenSecretID {
		t.Fatal("stored row not updated", err)
	}
}
