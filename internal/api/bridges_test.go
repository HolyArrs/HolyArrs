package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// Connect Jackett or Prowlarr (#96), through the API, against fakes on this machine.

const bridgeJackettKey = "jackett-api-test-key"

func fakeJackettServer(t *testing.T, sites ...string) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		if r.URL.Query().Get("apikey") != bridgeJackettKey {
			_, _ = w.Write([]byte(`<error code="100" description="Invalid API Key"/>`))
			return
		}
		var b strings.Builder
		b.WriteString(`<indexers>`)
		for _, id := range sites {
			fmt.Fprintf(&b, `<indexer id="%s" configured="true"><title>Site %s</title><type>public</type><caps><categories><category id="2000"/><category id="5000"/></categories></caps></indexer>`, id, id)
		}
		b.WriteString(`</indexers>`)
		_, _ = w.Write([]byte(b.String()))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// route: GET /api/v1/bridges
// route: POST /api/v1/bridges
// route: POST /api/v1/bridges/test
// route: POST /api/v1/bridges/{id}/check
// route: DELETE /api/v1/bridges/{id}
func TestBridgeJackettConnectCheckDisconnect(t *testing.T) {
	srv, _ := sourceServer(t)
	j := fakeJackettServer(t, "alpha", "beta")
	var res struct {
		OK    bool     `json:"ok"`
		Text  string   `json:"text"`
		Sites []string `json:"sites"`
	}
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/bridges/test", `{"kind":"jackett","url":"`+j.URL+`/","key":"`+bridgeJackettKey+`"}`, 200), &res)
	if !res.OK || res.Text != "Jackett answered with 2 sites." || len(res.Sites) != 2 {
		t.Fatalf("test %+v", res)
	}
	var bad struct {
		Error string `json:"error"`
		Field string `json:"field"`
	}
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/bridges/test", `{"kind":"jackett","url":"`+j.URL+`","key":"wrong"}`, 502), &bad)
	if !strings.Contains(bad.Error, "turned down the key") {
		t.Fatalf("wrong key %+v", bad)
	}
	for body, field := range map[string]string{
		`{"kind":"sonarr","url":"http://x","key":"k"}`:        "kind",
		`{"kind":"jackett","url":"jackett:9117","key":"k"}`:   "url",
		`{"kind":"jackett","url":"http://jackett:9117"}`:      "key",
		`{"kind":"prowlarr","url":"http://p:9696","key":"k"}`: "callback_url",
	} {
		_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/bridges/test", body, 400), &bad)
		if bad.Field != field {
			t.Errorf("%s: field %q, want %q", body, bad.Field, field)
		}
	}

	var v BridgeView
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/bridges", `{"kind":"jackett","url":"`+j.URL+`","key":"`+bridgeJackettKey+`"}`, 201), &v)
	if v.Kind != "jackett" || v.URL != j.URL || v.Sites != 2 || v.Sources != 2 || v.LastCheck == nil || v.LastError != "" {
		t.Fatalf("connect %+v", v)
	}
	// Connecting the same Jackett again takes its key; it is still one connection.
	var again BridgeView
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/bridges", `{"kind":"jackett","url":"`+j.URL+`/","key":"`+bridgeJackettKey+`"}`, 201), &again)
	if again.ID != v.ID || again.Sources != 2 {
		t.Fatalf("reconnect %+v", again)
	}
	var list struct {
		Bridges []BridgeView `json:"bridges"`
	}
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/bridges", "", 200), &list)
	if len(list.Bridges) != 1 || list.Bridges[0].ID != v.ID {
		t.Fatalf("list %+v", list)
	}
	var sources struct {
		Sources []struct {
			Name string `json:"name"`
		} `json:"sources"`
	}
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/sources", "", 200), &sources)
	if len(sources.Sources) != 2 || sources.Sources[0].Name != "Site alpha" {
		t.Fatalf("sources %+v", sources)
	}
	var checked struct {
		Result struct {
			Sites, Added, Updated, Removed int
		} `json:"result"`
	}
	_ = json.Unmarshal(do(t, "POST", fmt.Sprintf("%s/api/v1/bridges/%d/check", srv.URL, v.ID), "", 200), &checked)
	if checked.Result.Sites != 2 || checked.Result.Added+checked.Result.Updated+checked.Result.Removed != 0 {
		t.Fatalf("check %+v", checked)
	}
	var gone struct {
		Sources int  `json:"sources"`
		Kept    bool `json:"kept"`
	}
	_ = json.Unmarshal(do(t, "DELETE", fmt.Sprintf("%s/api/v1/bridges/%d", srv.URL, v.ID), "", 200), &gone)
	if gone.Sources != 2 || gone.Kept {
		t.Fatalf("delete %+v", gone)
	}
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/sources", "", 200), &sources)
	if len(sources.Sources) != 0 {
		t.Fatalf("left %+v", sources)
	}
	do(t, "POST", fmt.Sprintf("%s/api/v1/bridges/%d/check", srv.URL, v.ID), "", 404)
	do(t, "DELETE", srv.URL+"/api/v1/bridges/x", "", 400)
}

// fakeProwlarrApps keeps the apps written to it and, on a sync, does what
// Prowlarr does: writes one site into each app through its old-app API.
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
	f := newOldAppFix(t)
	feed := newProwlarrFeed(t)
	p := newFakeProwlarrApps(t, feed.URL)
	body := fmt.Sprintf(`{"kind":"prowlarr","url":%q,"key":%q,"callback_url":%q}`, p.URL, prowlarrKey, f.srv.URL+"/")
	st, out := f.session("POST", "/api/v1/bridges/test", body)
	if st != 200 || !strings.Contains(out, "Prowlarr 2.6.5.5623 answered.") || !strings.Contains(out, `"sonarr":"new"`) || !strings.Contains(out, `"radarr":"new"`) {
		t.Fatalf("test %d %s", st, out)
	}
	// An API key without the admin scope cannot have old-app keys made and
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
		t.Fatalf("%d old-app keys were made for a refused call", keys)
	}
	// Connect: the old-app API turns on, a key is made per side, HolyArrs is
	// added to Prowlarr on both sides, and Prowlarr's sync writes the Sources.
	st, out = f.session("POST", "/api/v1/bridges", body)
	if st != 201 {
		t.Fatalf("connect %d %s", st, out)
	}
	var v BridgeView
	_ = json.Unmarshal([]byte(out), &v)
	if v.Kind != "prowlarr" || v.Sources != 2 || v.CallbackURL != f.srv.URL || v.LastError != "" {
		t.Fatalf("view %+v", v)
	}
	if st, body := f.session("GET", "/api/v1/settings/old-app-api", ""); st != 200 || !strings.Contains(body, `"enabled":true`) {
		t.Fatalf("old-app API not on: %s", body)
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

// A connect Prowlarr refuses takes back what it made: the old-app keys and
// the switch it turned on, and the entry it had written there.
func TestBridgeProwlarrRefusedConnectTakesBackItsKeys(t *testing.T) {
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
		t.Fatalf("%d old-app keys left by a refused connect", keys)
	}
	if st, out := f.session("GET", "/api/v1/settings/old-app-api", ""); st != 200 || !strings.Contains(out, `"enabled":false`) {
		t.Fatalf("old-app API left on: %s", out)
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

// route: POST /api/v1/bridges/find
func TestBridgeFindAnswersAList(t *testing.T) {
	srv, _ := sourceServer(t)
	var out struct {
		Found []map[string]string `json:"found"`
	}
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/bridges/find", "", 200), &out)
	if out.Found == nil {
		t.Fatal("found is not a list")
	}
}
