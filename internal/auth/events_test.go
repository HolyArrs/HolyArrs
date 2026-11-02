package auth

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/CoderLuii/holyarrs/internal/store"
)

// Sign-ins (good and bad), sign-outs and API keys made or deleted land in
// History with who and from where, never with a password or a key.
func TestAuthEvents(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/login", `{"name":"luis","password":"not the password"}`, nil)
	_, out := own.do(t, f, "POST", "/api/v1/auth/keys", `{"label":"tool key"}`)
	var k struct {
		ID  int64  `json:"id"`
		Key string `json:"key"`
	}
	_ = json.Unmarshal([]byte(out), &k)
	if resp, _ := own.do(t, f, "DELETE", "/api/v1/auth/keys/"+itoa(k.ID), ""); resp.StatusCode != 204 {
		t.Fatalf("delete key: %d", resp.StatusCode)
	}
	own.do(t, f, "POST", "/api/v1/auth/logout", "")
	es, _, err := f.s.Events(context.Background(), store.EventFilter{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, e := range es {
		seen[e.Kind] = true
		raw, _ := json.Marshal(e.Data)
		if strings.Contains(string(raw), pw) || strings.Contains(string(raw), "not the password") || (k.Key != "" && strings.Contains(string(raw), k.Key)) {
			t.Fatalf("%s holds a secret: %s", e.Kind, raw)
		}
		if e.Kind == "sign_in_failed" && (e.Data["name"] != "luis" || e.Data["ip"] == "") {
			t.Fatalf("failed sign-in data %v", e.Data)
		}
		if e.Kind == "api_key_deleted" && (e.Data["label"] != "tool key" || e.Actor.Kind != "user") {
			t.Fatalf("key deleted %+v", e)
		}
	}
	for _, kind := range []string{"signed_in", "sign_in_failed", "api_key_created", "api_key_deleted", "signed_out"} {
		if !seen[kind] {
			t.Errorf("no %s event (have %v)", kind, seen)
		}
	}
}
