package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

// sourceCaps is a small Torznab caps answer.
const sourceCaps = `<?xml version="1.0"?><caps><limits max="50"/><searching><search available="yes" supportedParams="q"/>
<movie-search available="yes" supportedParams="q,imdbid"/></searching><categories><category id="2000"/></categories></caps>`

// fakeProwlarrApps keeps the apps written to it and, on a sync, does what
// Prowlarr does: writes one site into each app through its arr API.
type fakeProwlarrApps struct {
	*httptest.Server
	mu           sync.Mutex
	apps         []map[string]any
	syncs        int
	pushed       map[string]int
	refuseRadarr bool // answers the Radarr app the way Prowlarr does when its test fails
}

func newFakeProwlarrApps(t *testing.T, feedURL string) *fakeProwlarrApps {
	p := &fakeProwlarrApps{pushed: map[string]int{}}
	p.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/1/api" && r.URL.Query().Get("apikey") == prowlarrKey {
			_, _ = w.Write([]byte(sourceCaps)) // the site's feed, on Prowlarr's own address
			return
		}
		if r.Header.Get("X-Api-Key") != prowlarrKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		p.mu.Lock()
		defer p.mu.Unlock()
		switch {
		case r.URL.Path == "/api/v1/system/status":
			_ = json.NewEncoder(w).Encode(map[string]any{"appName": "Prowlarr", "version": "2.6.5.5623"})
		case r.URL.Path == "/api/v1/applications/schema":
			var out []map[string]any
			for _, impl := range []string{"Sonarr", "Radarr"} {
				out = append(out, map[string]any{"implementation": impl, "configContract": impl + "Settings", "fields": []any{
					map[string]any{"name": "prowlarrUrl"}, map[string]any{"name": "baseUrl"}, map[string]any{"name": "apiKey"}, map[string]any{"name": "syncCategories", "value": []any{2000, 5000}}}})
			}
			_ = json.NewEncoder(w).Encode(out)
		case r.URL.Path == "/api/v1/applications" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(p.apps)
		case r.URL.Path == "/api/v1/applications" && r.Method == http.MethodPost:
			var a map[string]any
			_ = json.NewDecoder(r.Body).Decode(&a)
			if p.refuseRadarr && a["implementation"] == "Radarr" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode([]map[string]any{{"propertyName": "BaseUrl", "errorMessage": "Unable to connect to Radarr"}})
				return
			}
			a["id"] = len(p.apps) + 1
			p.apps = append(p.apps, a)
			_ = json.NewEncoder(w).Encode(a)
		case strings.HasPrefix(r.URL.Path, "/api/v1/applications/") && r.Method == http.MethodDelete:
			id := strings.TrimPrefix(r.URL.Path, "/api/v1/applications/")
			for i, a := range p.apps {
				if fmt.Sprint(a["id"]) == id {
					p.apps = append(p.apps[:i], p.apps[i+1:]...)
					break
				}
			}
		case strings.HasPrefix(r.URL.Path, "/api/v1/applications/") && r.Method == http.MethodPut:
			_, _ = w.Write([]byte("{}"))
		case r.URL.Path == "/api/v1/command":
			p.syncs++
			for _, a := range p.apps {
				base, key := appField(a, "baseUrl"), appField(a, "apiKey")
				p.pushed[base] += pushIndexer(t, base, key, appField(a, "prowlarrUrl"))
			}
			_, _ = w.Write([]byte(`{"id":1}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(p.Close)
	return p
}
func appField(a map[string]any, name string) string {
	for _, x := range a["fields"].([]any) {
		f := x.(map[string]any)
		if f["name"] == name {
			s, _ := f["value"].(string)
			return s
		}
	}
	return ""
}

// pushIndexer writes one Torznab site into an app the way Prowlarr's sync does.
func pushIndexer(t *testing.T, base, key, feedURL string) int {
	req, _ := http.NewRequest("GET", base+"/api/v3/indexer/schema", nil)
	req.Header.Set("X-Api-Key", key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		t.Errorf("schema at %s: %v %v", base, err, resp)
		return 0
	}
	var tmpls []map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&tmpls)
	resp.Body.Close()
	for _, tm := range tmpls {
		if tm["implementation"] != "Torznab" {
			continue
		}
		body := fillIndexer(t, tm, "Lab Tracker (Prowlarr)", feedURL+"/1/", map[string]any{"categories": []any{2000, 5000}})
		body["priority"], body["enableRss"], body["tags"] = 25, true, []any{}
		b, _ := json.Marshal(body)
		req, _ := http.NewRequest("POST", base+"/api/v3/indexer", bytes.NewReader(b))
		req.Header.Set("X-Api-Key", key)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil || resp.StatusCode != 201 {
			t.Errorf("push at %s: %v %v", base, err, resp)
			return 0
		}
		resp.Body.Close()
		return 1
	}
	return 0
}
func TestBridgeProwlarrConnectUsesTheOldAppSync(t *testing.T) {
	t.Parallel()
	f := newOldAppFix(t)
	feed := newProwlarrFeed(t)
	p := newFakeProwlarrApps(t, feed.URL)
	body := fmt.Sprintf(`{"kind":"prowlarr","url":%q,"key":%q,"callback_url":%q}`, p.URL, prowlarrKey, f.srv.URL+"/")
	st, out := f.session("POST", "/api/v1/bridges/test", body)
	if st != 200 || !strings.Contains(out, "Prowlarr 2.6.5.5623 answered.") || !strings.Contains(out, `"sonarr":"new"`) || !strings.Contains(out, `"radarr":"new"`) {
		t.Fatalf("test %d %s", st, out)
	}
	// An API key without the admin scope cannot have arr keys made and
	// sent to an address it names (the same gates as making one by hand).
	st, out = f.session("POST", "/api/v1/auth/keys", `{"label":"rw","scopes":["read","write"]}`)
	if st != 201 {
		t.Fatalf("api key %d %s", st, out)
	}
	if st, _, out := f.call(&http.Client{}, "", "POST", "/api/v1/bridges", body, "X-Api-Key", aiField(out, "key")); st != 403 {
		t.Fatalf("a read/write API key connected Prowlarr: %d %s", st, out)
	}
	p.mu.Lock()
	if len(p.apps) != 0 {
		t.Fatalf("Prowlarr got %d apps from a refused call", len(p.apps))
	}
	p.mu.Unlock()
	var keys int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM compat_keys").Scan(&keys)
	if keys != 0 {
		t.Fatalf("%d arr keys were made for a refused call", keys)
	}
	// Making the keys is a risky action: a sign-in whose password proof is
	// older than 10 minutes is asked to confirm it, and nothing is made. The
	// first-run setup session is the exception for its first hour; f.own holds that session.
	exec := func(q string, args ...any) {
		if err := f.s.WriteCore(context.Background(), func(tx *sql.Tx) error {
			_, err := tx.Exec(q, args...)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	hourAgo := database.FormatTime(time.Now().Add(-time.Hour - time.Minute))
	noKeys := func(what string) {
		t.Helper()
		_ = f.s.Read().QueryRow("SELECT count(*) FROM compat_keys").Scan(&keys)
		if keys != 0 {
			t.Fatalf("%d arr keys were made for %s", keys, what)
		}
	}
	// The setup session after its 24-hour cap.
	dayAgo := database.FormatTime(time.Now().Add(-25 * time.Hour))
	exec("UPDATE sessions SET local_auth_at = ?, created_at = ?", hourAgo, dayAgo)
	if st, out := f.session("POST", "/api/v1/bridges", body); st != 403 || !strings.Contains(out, `"reauth"`) {
		t.Fatalf("setup session after its day made arr keys: %d %s", st, out)
	}
	noKeys("an old setup session")
	// A normal password sign-in with a stale proof.
	jar, _ := cookiejar.New(nil)
	other := &http.Client{Jar: jar}
	st, _, out = f.call(other, "", "POST", "/api/v1/auth/login", `{"name":"luis","password":"a long test passphrase"}`)
	if st != 200 {
		t.Fatalf("login %d %s", st, out)
	}
	otherCSRF := aiField(out, "csrf")
	exec("UPDATE sessions SET local_auth_at = ? WHERE family NOT LIKE 'setup.%'", hourAgo)
	if st, _, out := f.call(other, otherCSRF, "POST", "/api/v1/bridges", body); st != 403 || !strings.Contains(out, `"reauth"`) {
		t.Fatalf("stale sign-in made arr keys: %d %s", st, out)
	}
	noKeys("a stale sign-in")
	// The setup session inside its hour connects with no fresh password proof.
	exec("UPDATE sessions SET created_at = ? WHERE family LIKE 'setup.%'", database.FormatTime(time.Now().Add(-50*time.Minute)))
	// Connect: the arr API turns on, a key is made per side, HolyArrs is
	// added to Prowlarr on both sides, and Prowlarr's sync writes the Sources.
	st, out = f.session("POST", "/api/v1/bridges", body)
	if st != 201 {
		t.Fatalf("connect %d %s", st, out)
	}
	var v sources.BridgeView
	_ = json.Unmarshal([]byte(out), &v)
	if v.Kind != "prowlarr" || v.Sources != 2 || v.CallbackURL != f.srv.URL || v.LastError != "" {
		t.Fatalf("view %+v", v)
	}
	if st, body := f.session("GET", "/api/v1/settings/old-app-api", ""); st != 200 || !strings.Contains(body, `"enabled":true`) {
		t.Fatalf("arr API not on: %s", body)
	}
	p.mu.Lock()
	if len(p.apps) != 2 || p.syncs != 1 || p.pushed[f.srv.URL+"/sonarr"] != 1 || p.pushed[f.srv.URL+"/radarr"] != 1 {
		t.Fatalf("prowlarr apps %d syncs %d pushed %v", len(p.apps), p.syncs, p.pushed)
	}
	for _, a := range p.apps {
		if k := appField(a, "apiKey"); len(k) != 32 || a["syncLevel"] != "fullSync" || appField(a, "prowlarrUrl") != p.URL {
			t.Fatalf("app %+v", a)
		}
	}
	p.mu.Unlock()
	// The keys exist now: connecting again would replace them, so it asks first.
	if st, out := f.session("POST", "/api/v1/bridges", body); st != 409 || !strings.Contains(out, `"need":"keys"`) {
		t.Fatalf("second connect %d %s", st, out)
	}
	// Disconnect: HolyArrs leaves Prowlarr's apps, and the Sources Prowlarr
	// wrote go here (Prowlarr does not remove them when an app goes).
	if st, out := f.session("DELETE", fmt.Sprintf("/api/v1/bridges/%d", v.ID), ""); st != 200 || !strings.Contains(out, `"sources":2`) {
		t.Fatalf("delete %d %s", st, out)
	}
	p.mu.Lock()
	if len(p.apps) != 0 {
		t.Fatalf("after disconnect apps %d", len(p.apps))
	}
	p.mu.Unlock()
	var left int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM sources WHERE json_extract(extra, '$.old_app') IS NOT NULL").Scan(&left)
	if left != 0 {
		t.Fatalf("%d of Prowlarr's Sources stayed", left)
	}
}

// A connect Prowlarr refuses takes back what it made: the arr keys and
// the switch it turned on, and the entry it had written there.
func TestBridgeProwlarrRefusedConnectTakesBackItsKeys(t *testing.T) {
	t.Parallel()
	f := newOldAppFix(t)
	p := newFakeProwlarrApps(t, "")
	p.refuseRadarr = true
	body := fmt.Sprintf(`{"kind":"prowlarr","url":%q,"key":%q,"callback_url":%q}`, p.URL, prowlarrKey, f.srv.URL)
	if st, out := f.session("POST", "/api/v1/bridges", body); st != 502 || !strings.Contains(out, "Unable to connect") {
		t.Fatalf("connect %d %s", st, out)
	}
	var keys int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM compat_keys").Scan(&keys)
	if keys != 0 {
		t.Fatalf("%d arr keys left by a refused connect", keys)
	}
	if st, out := f.session("GET", "/api/v1/settings/old-app-api", ""); st != 200 || !strings.Contains(out, `"enabled":false`) {
		t.Fatalf("arr API left on: %s", out)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.apps) != 0 {
		t.Fatalf("%d entries left in Prowlarr", len(p.apps))
	}
	if st, out := f.session("GET", "/api/v1/bridges", ""); st != 200 || strings.Contains(out, `"kind"`) {
		t.Fatalf("a bridge was kept: %s", out)
	}
}
