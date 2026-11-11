package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/migrate/secretsfile"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"
)

// keyCall sends one call with an API key in its header.
func keyCall(t *testing.T, base, key, method, path, body string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(method, base+path, strings.NewReader(body))
	req.Header.Set(accounts.APIKeyHeader, key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// route: POST /api/v1/settings/import-secrets
// route: GET /api/v1/migrate/saved-keys
// route: DELETE /api/v1/migrate/saved-keys
func TestImportSecretsRoutes(t *testing.T) {
	t.Parallel()
	r := newArrAskRig(t)
	radarrKey := "your-radarr-key-1"
	u := fakeRadarr(t, radarrKey)
	file := map[string]any{"version": 1,
		"tvdb":     map[string]any{"api_key": "your-tvdb-key-2"},
		"radarr":   map[string]any{"address": u, "api_key": radarrKey, "allow_local_network": true},
		"holyarrs": map[string]any{"address": "http://127.0.0.1:1", "owner_api_key": "your-owner-key-3"},
	}
	body, _ := json.Marshal(map[string]any{"file": file})

	// Members never; a key needs the admin scope.
	r.ana.call("POST", "/api/v1/settings/import-secrets", string(body), 403)
	r.ana.call("GET", "/api/v1/migrate/saved-keys", "", 403)
	r.ana.call("DELETE", "/api/v1/migrate/saved-keys", "", 403)
	var k struct {
		Key string `json:"key"`
	}
	_ = json.Unmarshal(r.owner.call("POST", "/api/v1/auth/keys", `{"label":"w","scopes":["write"]}`, 201), &k)
	if st, _ := keyCall(t, r.srv.URL, k.Key, "POST", "/api/v1/settings/import-secrets", string(body)); st != 403 {
		t.Fatalf("a write-scope key imported: %d", st)
	}
	_ = json.Unmarshal(r.owner.call("POST", "/api/v1/auth/keys", `{"label":"import","scopes":["admin"]}`, 201), &k)

	// Bad bodies.
	if st, _ := keyCall(t, r.srv.URL, k.Key, "POST", "/api/v1/settings/import-secrets", `{"file":{"version":1,"tvdb":{"api_kee":"your-x-9"}}}`); st != 400 {
		t.Fatalf("unknown key: %d", st)
	}
	if st, _ := keyCall(t, r.srv.URL, k.Key, "POST", "/api/v1/settings/import-secrets", `nope`); st != 400 {
		t.Fatalf("not json: %d", st)
	}

	// A dry run, then the run, then again.
	dry, _ := json.Marshal(map[string]any{"file": file, "dry_run": true})
	st, out := keyCall(t, r.srv.URL, k.Key, "POST", "/api/v1/settings/import-secrets", string(dry))
	if st != 200 || !strings.Contains(out, `"result":"would set`) {
		t.Fatalf("dry %d %s", st, out)
	}
	st, out = keyCall(t, r.srv.URL, k.Key, "POST", "/api/v1/settings/import-secrets", string(body))
	if st != 200 || !strings.Contains(out, `"ok":true`) || !strings.Contains(out, `{"item":"radarr (old app)","result":"set`) {
		t.Fatalf("run %d %s", st, out)
	}
	for _, v := range []string{radarrKey, "your-tvdb-key-2", "your-owner-key-3"} {
		if strings.Contains(out, v) {
			t.Fatalf("a value in the answer")
		}
	}
	if _, out = keyCall(t, r.srv.URL, k.Key, "POST", "/api/v1/settings/import-secrets", string(body)); !strings.Contains(out, `{"item":"radarr (old app)","result":"unchanged`) {
		t.Fatalf("again %s", out)
	}

	// The Move screen sees the saved key without the key, and reads with
	// it as often as it likes.
	b := r.owner.call("GET", "/api/v1/migrate/saved-keys", "", 200)
	if !strings.Contains(string(b), `"kind":"radarr"`) || strings.Contains(string(b), radarrKey) {
		t.Fatalf("saved %s", b)
	}
	r.ana.call("POST", "/api/v1/migrate/read", `{"kind":"radarr","use_saved_key":true}`, 403)
	// An admin is not the owner.
	r.owner.call("PUT", "/api/v1/users/"+arrItoa(r.bo.id), `{"role":"admin"}`, 200)
	r.bo.call("POST", "/api/v1/migrate/probe", `{"kind":"radarr","use_saved_key":true}`, 403)
	// The key only goes to its saved address: another address has no saved key.
	b = r.owner.call("POST", "/api/v1/migrate/probe", `{"kind":"radarr","use_saved_key":true,"url":"http://127.0.0.1:1"}`, 400)
	if !strings.Contains(string(b), "no saved key for this app at this address") {
		t.Fatalf("other url %s", b)
	}
	r.owner.call("POST", "/api/v1/migrate/probe", `{"kind":"radarr","use_saved_key":true,"url":"not a url"}`, 400)
	same := strings.ToUpper(strings.Replace(u, "http://", "HTTP://", 1)) + "/"
	r.owner.call("POST", "/api/v1/migrate/probe", `{"kind":"radarr","use_saved_key":true,"url":"`+same+`"}`, 200)
	r.owner.call("POST", "/api/v1/migrate/probe", `{"kind":"radarr","use_saved_key":true}`, 200)
	b = r.owner.call("POST", "/api/v1/migrate/read", `{"kind":"radarr","use_saved_key":true}`, 201)
	if strings.Contains(string(b), radarrKey) {
		t.Fatal("key answered")
	}
	if b := r.owner.call("GET", "/api/v1/migrate/saved-keys", "", 200); !strings.Contains(string(b), `"kind":"radarr"`) || strings.Contains(string(b), radarrKey) {
		t.Fatalf("saved key gone after a read %s", b)
	}
	r.owner.call("POST", "/api/v1/migrate/read", `{"kind":"radarr","use_saved_key":true}`, 201)

	// A credential saved without the local-network leave cannot be widened
	// by the request: the fake on this machine is then not dialled.
	rk := radarrKey
	narrow := map[string]any{"version": 1, "radarr": map[string]any{"api_key": rk, "address": u}}
	nb, _ := json.Marshal(map[string]any{"file": narrow})
	keyCall(t, r.srv.URL, k.Key, "POST", "/api/v1/settings/import-secrets", string(nb))
	b = r.owner.call("POST", "/api/v1/migrate/probe", `{"kind":"radarr","use_saved_key":true,"allow_local_network":true}`, 502)
	if strings.Contains(string(b), radarrKey) {
		t.Fatal("key answered")
	}
	// Forget.
	keyCall(t, r.srv.URL, k.Key, "POST", "/api/v1/settings/import-secrets", string(body))
	if b := r.owner.call("DELETE", "/api/v1/migrate/saved-keys", "", 200); !strings.Contains(string(b), `"forgotten":1`) {
		t.Fatalf("forget %s", b)
	}
	// Events say counts, never a value.
	ev := r.owner.call("GET", "/api/v1/events?kind="+secretsfile.EventImported, "", 200)
	if !strings.Contains(string(ev), "Imported settings from a secrets file") || strings.Contains(string(ev), "your-") {
		t.Fatalf("events %s", ev)
	}
}

// The import writes the keys the Settings screens read.
// Without a signed-in owner (no sign-in in front at all) a saved key is
// never used.
func TestSavedKeyNeedsSignedInOwner(t *testing.T) {
	t.Parallel()
	srv, _, _ := arrSetup(t)
	arrDo(t, "POST", srv.URL+"/api/v1/migrate/probe", `{"kind":"radarr","use_saved_key":true}`, 403)
}

func TestImportSecretsSameKeys(t *testing.T) {
	t.Parallel()
	pairs := [][2]string{{secretsfile.LinksGroup, metadata.TraktKeyGroup},
		{secretsfile.TraktClientKey, metadata.TraktKeyName}, {secretsfile.LinksGroup, shows.TVDBKeyGroup}, {secretsfile.TVDBKeyKey, shows.TVDBKeyName}}
	for _, p := range pairs {
		if p[0] != p[1] {
			t.Errorf("the import writes %q where the Settings screen reads %q", p[0], p[1])
		}
	}
}

func TestLocalSkipCannotReadSavedAddresses(t *testing.T) {
	t.Parallel()
	r := newArrAskRig(t)
	r.owner.call("PUT", "/api/v1/auth/settings", `{"skip_login_local_network":true,"local_networks":["127.0.0.0/8","10.0.0.0/8"]}`, 200)
	for _, tc := range []struct{ method, path string }{
		{"GET", "/api/v1/migrate/saved-keys"},
	} {
		req, _ := http.NewRequest(tc.method, r.srv.URL+tc.path, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 403 {
			t.Fatalf("%s %s: %d", tc.method, tc.path, resp.StatusCode)
		}
	}
}
