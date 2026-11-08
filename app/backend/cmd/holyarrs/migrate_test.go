package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// fakeRadarr answers the few Radarr calls the read makes (written from the
// Radarr manual), with secrets that must never come back.
func fakeRadarr(t *testing.T, key string) string {
	t.Helper()
	routes := map[string]string{
		"/api/v3/system/status": `{"appName":"Radarr","version":"6.4.4","migrationVersion":242}`,
		"/api/v3/movie":         `[{"id":1,"tmdbId":603,"title":"The Matrix","year":1999,"path":"/movies/The Matrix (1999)","monitored":true,"qualityProfileId":1,"tags":[]}]`,
		"/api/v3/indexer":       `[{"id":1,"name":"Nyaa","implementation":"Torznab","fields":[{"name":"baseUrl","value":"http://x/"},{"name":"apiKey","value":"SECRET_IN_RADARR"}]}]`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != key {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		b, ok := routes[r.URL.Path]
		if !ok {
			b = "[]"
			if strings.HasSuffix(r.URL.Path, "/naming") {
				b = "{}"
			}
		}
		_, _ = w.Write([]byte(b))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func readBody(key, url string) string {
	b, _ := json.Marshal(map[string]any{"kind": "radarr", "url": url, "key": key, "allow_local_network": true})
	return string(b)
}

// route: POST /api/v1/migrate/probe
// route: POST /api/v1/migrate/read
// route: GET /api/v1/migrate/snapshots
// route: GET /api/v1/migrate/snapshots/{id}
// route: DELETE /api/v1/migrate/snapshots/{id}
func TestMigrateRoutes(t *testing.T) {
	t.Parallel()
	srv, _, _ := arrSetup(t)
	key := "typed-radarr-key"
	u := fakeRadarr(t, key)
	base := srv.URL + "/api/v1/migrate/"

	b := arrDo(t, "POST", base+"probe", readBody(key, u), 200)
	if !strings.Contains(string(b), `"version":"6.4.4"`) || strings.Contains(string(b), key) {
		t.Fatalf("probe %s", b)
	}
	arrDo(t, "POST", base+"probe", readBody("wrong", u), 422)
	arrDo(t, "POST", base+"probe", `{"kind":"readarr","url":"http://x"}`, 400)
	arrDo(t, "POST", base+"probe", `{"kind":"radarr"}`, 400)
	arrDo(t, "POST", base+"probe", `{"kind":"radarr","path":"relative/backup.zip"}`, 400)
	arrDo(t, "POST", base+"probe", `not json`, 400)
	arrDo(t, "POST", base+"probe", `{"kind":"radarr","path":"`+filepath.ToSlash(filepath.Join(t.TempDir(), "none.zip"))+`"}`, 422)
	// Without the local switch the fake (on this machine) is not dialled.
	arrDo(t, "POST", base+"probe", `{"kind":"radarr","url":"`+u+`","key":"`+key+`"}`, 502)

	b = arrDo(t, "POST", base+"read", readBody(key, u), 201)
	if strings.Contains(string(b), "SECRET_IN_RADARR") || strings.Contains(string(b), key) || !strings.Contains(string(b), `indexer \"Nyaa\": apiKey`) {
		t.Fatalf("read %s", b)
	}
	var made struct {
		Snapshot struct {
			ID     int64          `json:"id"`
			Counts map[string]int `json:"counts"`
		} `json:"snapshot"`
	}
	_ = json.Unmarshal(b, &made)
	if made.Snapshot.ID == 0 || made.Snapshot.Counts["titles"] != 1 {
		t.Fatalf("made %s", b)
	}
	id := arrItoa(made.Snapshot.ID)
	if b := arrDo(t, "GET", base+"snapshots", "", 200); !strings.Contains(string(b), `"kind":"radarr"`) {
		t.Fatalf("list %s", b)
	}
	b = arrDo(t, "GET", base+"snapshots/"+id, "", 200)
	if !strings.Contains(string(b), `"The Matrix"`) || strings.Contains(string(b), "SECRET_IN_RADARR") {
		t.Fatalf("get %s", b)
	}
	arrDo(t, "GET", base+"snapshots/999", "", 404)
	arrDo(t, "GET", base+"snapshots/abc", "", 404)
	arrDo(t, "DELETE", base+"snapshots/"+id, "", 204)
	arrDo(t, "DELETE", base+"snapshots/"+id, "", 404)
	arrDo(t, "DELETE", base+"snapshots/x", "", 404)
	if b := arrDo(t, "GET", base+"snapshots", "", 200); strings.Contains(string(b), `"kind":"radarr"`) {
		t.Fatalf("still listed %s", b)
	}
}

// TestMigratePermissions: the move routes are for the owner and admins,
// with a password sign-in and, for keys, the admin scope.
func TestMigratePermissions(t *testing.T) {
	t.Parallel()
	r := newArrAskRig(t)
	key := "typed-radarr-key"
	u := fakeRadarr(t, key)
	routes := []struct{ method, path, body string }{
		{"POST", "/api/v1/migrate/probe", readBody(key, u)},
		{"POST", "/api/v1/migrate/read", readBody(key, u)},
		{"GET", "/api/v1/migrate/snapshots", ""},
		{"GET", "/api/v1/migrate/snapshots/1", ""},
		{"DELETE", "/api/v1/migrate/snapshots/1", ""},
	}
	for _, rt := range routes {
		r.ana.call(rt.method, rt.path, rt.body, 403) // a member
	}
	r.owner.call("POST", "/api/v1/migrate/read", readBody(key, u), 201)
	r.owner.call("GET", "/api/v1/migrate/snapshots/1", "", 200)

	// The local-network skip signs in without a password: no move routes.
	r.owner.call("PUT", "/api/v1/auth/settings", `{"skip_login_local_network":true,"local_networks":["127.0.0.0/8","10.0.0.0/8"]}`, 200)
	jar, _ := cookiejar.New(nil)
	bare := &arrSignedIn{t: t, base: r.srv.URL, c: &http.Client{Jar: jar}}
	for _, rt := range routes {
		if b := bare.call(rt.method, rt.path, rt.body, 403); !strings.Contains(string(b), "confirm your local password") {
			t.Errorf("%s %s: %s", rt.method, rt.path, b)
		}
	}
	r.owner.call("PUT", "/api/v1/auth/settings", `{"skip_login_local_network":false}`, 200)

	keyFor := func(scopes string) string {
		var v struct {
			Key string `json:"key"`
		}
		_ = json.Unmarshal(r.owner.call("POST", "/api/v1/auth/keys", `{"label":"k`+scopes+`","scopes":["`+scopes+`"]}`, 201), &v)
		return v.Key
	}
	withKey := func(k, method, path, body string) int {
		req, _ := http.NewRequest(method, r.srv.URL+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(accounts.APIKeyHeader, k)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return resp.StatusCode
	}
	writeKey, readKey, adminKey := keyFor("write"), keyFor("read"), keyFor("admin")
	for _, rt := range routes {
		if st := withKey(writeKey, rt.method, rt.path, rt.body); st != 403 {
			t.Errorf("write key %s %s: %d", rt.method, rt.path, st)
		}
		if st := withKey(readKey, rt.method, rt.path, rt.body); st != 403 {
			t.Errorf("read key %s %s: %d", rt.method, rt.path, st)
		}
	}
	if st := withKey(adminKey, "GET", "/api/v1/migrate/snapshots", ""); st != 200 {
		t.Fatalf("admin key list: %d", st)
	}
	for _, p := range []string{"POST /api/v1/migrate/probe", "POST /api/v1/migrate/read", "GET /api/v1/migrate/snapshots",
		"GET /api/v1/migrate/snapshots/{id}", "DELETE /api/v1/migrate/snapshots/{id}"} {
		if accounts.RouteScope(p) != webserver.ScopeAdmin {
			t.Errorf("%s scope %s", p, accounts.RouteScope(p))
		}
	}
}
