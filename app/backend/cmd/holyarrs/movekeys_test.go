package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// A good read with a typed key keeps it for "Read again" (owner only);
// a read with the saved key keeps it too; forgetting the read forgets it.
func TestMoveKeyKeptUntilMoved(t *testing.T) {
	t.Parallel()
	r := lbNewAskRig(t)
	key := "typed-kept-radarr-key"
	u := lbFakeRadarr(t, key)
	saved := func() string { return string(r.owner.call("GET", "/api/v1/migrate/saved-keys", "", 200)) }

	b := r.owner.call("POST", "/api/v1/migrate/read", lbReadBody(key, u), 201)
	if strings.Contains(string(b), key) {
		t.Fatal("key answered")
	}
	if s := saved(); !strings.Contains(s, `"kind":"radarr"`) || !strings.Contains(s, u) || strings.Contains(s, key) {
		t.Fatalf("typed read not kept %s", s)
	}
	for i := 0; i < 2; i++ {
		r.owner.call("POST", "/api/v1/migrate/read", `{"kind":"radarr","use_saved_key":true}`, 201)
	}
	if s := saved(); !strings.Contains(s, `"kind":"radarr"`) {
		t.Fatalf("saved read used the key up %s", s)
	}

	// Someone other than the owner never uses or keeps a key.
	r.owner.call("PUT", "/api/v1/users/"+lbItoa(r.bo.id), `{"role":"admin"}`, 200)
	r.bo.call("POST", "/api/v1/migrate/read", `{"kind":"radarr","use_saved_key":true}`, 403)
	r.owner.call("DELETE", "/api/v1/migrate/saved-keys", "", 200)
	r.bo.call("POST", "/api/v1/migrate/read", lbReadBody(key, u), 201)
	if s := saved(); strings.Contains(s, "radarr") {
		t.Fatalf("an admin's key was kept %s", s)
	}

	// Forgetting a read forgets that app's key.
	b = r.owner.call("POST", "/api/v1/migrate/read", lbReadBody(key, u), 201)
	var made struct {
		Snapshot struct {
			ID int64 `json:"id"`
		} `json:"snapshot"`
	}
	_ = json.Unmarshal(b, &made)
	if s := saved(); !strings.Contains(s, "radarr") {
		t.Fatalf("not kept %s", s)
	}
	r.owner.call("DELETE", "/api/v1/migrate/snapshots/"+lbItoa(made.Snapshot.ID), "", 204)
	if s := saved(); strings.Contains(s, "radarr") {
		t.Fatalf("kept after its read was forgotten %s", s)
	}
}
