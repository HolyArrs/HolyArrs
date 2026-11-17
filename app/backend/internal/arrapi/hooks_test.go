package arrapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// receiver stands in for Pulsarr: it records every call and answers with
// the status code set on it, after the delay set on it.
type receiver struct {
	srv   *httptest.Server
	mu    sync.Mutex
	code  int
	seq   []int // codes for the next calls, before code
	delay time.Duration
	calls []call
}

type call struct {
	method, uri string
	header      http.Header
	body        map[string]any
}

func newReceiver(t *testing.T) *receiver {
	t.Helper()
	rc := &receiver{code: 200}
	rc.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		rc.mu.Lock()
		rc.calls = append(rc.calls, call{method: r.Method, uri: r.URL.RequestURI(), header: r.Header.Clone(), body: m})
		code, delay := rc.code, rc.delay
		if len(rc.seq) > 0 {
			code, rc.seq = rc.seq[0], rc.seq[1:]
		}
		rc.mu.Unlock()
		if delay > 0 {
			time.Sleep(delay)
		}
		w.WriteHeader(code)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	t.Cleanup(rc.srv.Close)
	return rc
}

func (rc *receiver) answer(code int) {
	rc.mu.Lock()
	rc.code = code
	rc.mu.Unlock()
}

func (rc *receiver) got() []call {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	return append([]call(nil), rc.calls...)
}

// hookRig serves both sides with a vault, as a key with the given
// permissions would call them.
type hookRig struct {
	t        *testing.T
	st       *database.Store
	vault    *secrets.Vault
	son, rad http.Handler
	caller   webserver.Caller
}

func newHookRig(t *testing.T) *hookRig {
	t.Helper()
	st := settingsStore(t)
	v, err := secrets.Open(context.Background(), st, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	d := Deps{Store: st, Secrets: v}
	return &hookRig{t: t, st: st, vault: v, son: Sonarr(d), rad: Radarr(d),
		caller: webserver.Caller{UserID: 1, Name: "owner", Via: "api_key", Role: webserver.RoleOwner}}
}

func (h *hookRig) do(side, method, path, body string) (int, string) {
	h.t.Helper()
	srv := h.son
	if side == "radarr" {
		srv = h.rad
	}
	req := httptest.NewRequest(method, "/"+side+"/api/v3/"+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(webserver.ContextWith(req.Context(), h.caller))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

// pulsarrHook is the hook body Pulsarr sends, as its manual lists it.
func pulsarrHook(id int64, name, url, secret string) string {
	m := map[string]any{
		"name": name, "implementation": "Webhook", "implementationName": "Webhook", "configContract": "WebhookSettings",
		"infoLink": "https://example.invalid/webhook", "tags": []any{},
		"onGrab": false, "onDownload": true, "onUpgrade": false, "onImportComplete": false, "onRename": false, "onSeriesAdd": false,
		"onSeriesDelete": false, "onEpisodeFileDelete": false, "onEpisodeFileDeleteForUpgrade": false, "onHealthIssue": false,
		"includeHealthWarnings": false, "onHealthRestored": false, "onApplicationUpdate": false, "onManualInteractionRequired": false,
		"supportsOnGrab": true, "supportsOnDownload": true, "supportsOnUpgrade": true,
		"fields": []any{
			map[string]any{"name": "url", "value": url},
			map[string]any{"name": "method", "value": 1},
			map[string]any{"name": "username", "value": ""},
			map[string]any{"name": "password", "value": ""},
			map[string]any{"name": "headers", "value": []any{map[string]any{"key": "X-Pulsarr-Secret", "value": secret}}},
		},
	}
	if id > 0 {
		m["id"] = id
	}
	b, _ := json.Marshal(m)
	return string(b)
}

func fieldOf(t *testing.T, hook map[string]any, name string) any {
	t.Helper()
	for _, f := range hook["fields"].([]any) {
		if m := f.(map[string]any); m["name"] == name {
			return m["value"]
		}
	}
	t.Fatalf("no field %s in %v", name, hook)
	return nil
}

func failedOn(t *testing.T, code int, body, property string) {
	t.Helper()
	var fs []failure
	if code != 400 || json.Unmarshal([]byte(body), &fs) != nil || len(fs) == 0 || !strings.EqualFold(fs[0].PropertyName, property) {
		t.Fatalf("want a 400 failed check on %s, got %d %s", property, code, body)
	}
}

// A hook is saved only after HolyArrs reached it with a test event that
// carries the secret header. A refused test is a failed check on the
// address field, and nothing is kept.
func TestHooks_AddTestsFirst(t *testing.T) {
	h := newHookRig(t)
	rc := newReceiver(t)
	address := rc.srv.URL + "/v1/notifications/webhook?instanceId=holyarrs1"
	rc.answer(401)
	st, body := h.do("sonarr", "POST", "notification", pulsarrHook(0, "pulsarr-box", address, "s3cret"))
	failedOn(t, st, body, "Url")
	if strings.Contains(body, "s3cret") {
		t.Fatalf("the secret came back: %s", body)
	}
	calls := rc.got()
	if len(calls) != 1 || calls[0].method != "POST" || calls[0].uri != "/v1/notifications/webhook?instanceId=holyarrs1" ||
		calls[0].header.Get("X-Pulsarr-Secret") != "s3cret" || calls[0].body["eventType"] != "Test" || calls[0].body["instanceName"] != AppName {
		t.Fatalf("test call: %+v", calls)
	}
	if _, ok := calls[0].body["movie"]; ok {
		t.Fatal("a test must not carry a movie: Pulsarr reads any movie as ready")
	}
	if st, body := h.do("sonarr", "GET", "notification", ""); st != 200 || strings.TrimSpace(body) != "[]" {
		t.Fatalf("a refused hook was kept: %d %s", st, body)
	}

	rc.answer(200)
	st, body = h.do("sonarr", "POST", "notification", pulsarrHook(0, "pulsarr-box", address, "s3cret"))
	if st != 201 || strings.Contains(body, "s3cret") {
		t.Fatalf("add: %d %s", st, body)
	}
	var added map[string]any
	_ = json.Unmarshal([]byte(body), &added)
	id := int64(added["id"].(float64))
	st, body = h.do("sonarr", "GET", "notification", "")
	var list []map[string]any
	if st != 200 || json.Unmarshal([]byte(body), &list) != nil || len(list) != 1 || strings.Contains(body, "s3cret") {
		t.Fatalf("list: %d %s", st, body)
	}
	got := list[0]
	if int64(got["id"].(float64)) != id || got["name"] != "pulsarr-box" || got["implementation"] != "Webhook" || got["onDownload"] != true || got["onGrab"] != false ||
		fieldOf(t, got, "url") != address {
		t.Fatalf("listed hook: %v", got)
	}
	hs := fieldOf(t, got, "headers").([]any)
	if len(hs) != 1 || hs[0].(map[string]any)["key"] != "X-Pulsarr-Secret" || hs[0].(map[string]any)["value"] == "" {
		t.Fatalf("headers: %v", hs)
	}
	var kept string
	_ = h.st.Read().QueryRow("SELECT body FROM compat_hooks WHERE id = ?", id).Scan(&kept)
	if strings.Contains(kept, "s3cret") || strings.Contains(kept, "instanceId") {
		t.Fatalf("the body column holds a secret or the address: %s", kept)
	}
	// The other side keeps its own hooks.
	if st, body := h.do("radarr", "GET", "notification", ""); st != 200 || strings.TrimSpace(body) != "[]" {
		t.Fatalf("radarr list: %d %s", st, body)
	}
	// A second hook with the same name is refused.
	st, body = h.do("sonarr", "POST", "notification", pulsarrHook(0, "PULSARR-box", address, "s3cret"))
	failedOn(t, st, body, "Name")
}

// A change replaces the hook in place after the same test. A masked header
// keeps the stored value, but only on the same host.
func TestHooks_ChangeTestDelete(t *testing.T) {
	h := newHookRig(t)
	rc := newReceiver(t)
	address := rc.srv.URL + "/v1/notifications/webhook?instanceId=a"
	st, body := h.do("sonarr", "POST", "notification", pulsarrHook(0, "box", address, "first"))
	if st != 201 {
		t.Fatalf("add: %d %s", st, body)
	}
	var added map[string]any
	_ = json.Unmarshal([]byte(body), &added)
	id := int64(added["id"].(float64))
	moved := rc.srv.URL + "/v1/notifications/webhook?instanceId=b"
	if st, body := h.do("sonarr", "PUT", fmt.Sprintf("notification/%d", id), pulsarrHook(id, "box", moved, "second")); st != 202 || !strings.Contains(body, "instanceId=b") {
		t.Fatalf("change: %d %s", st, body)
	}
	if c := rc.got(); c[len(c)-1].header.Get("X-Pulsarr-Secret") != "second" || c[len(c)-1].uri != "/v1/notifications/webhook?instanceId=b" {
		t.Fatalf("change test call: %+v", c[len(c)-1])
	}
	// Sent back masked, the stored secret is used.
	if st, body := h.do("sonarr", "PUT", fmt.Sprintf("notification/%d", id), pulsarrHook(id, "box", moved, maskedKey)); st != 202 {
		t.Fatalf("masked change: %d %s", st, body)
	}
	if c := rc.got(); c[len(c)-1].header.Get("X-Pulsarr-Secret") != "second" {
		t.Fatalf("masked header not filled: %+v", c[len(c)-1].header)
	}
	// A masked secret never goes to another host.
	other := newReceiver(t)
	st, body = h.do("sonarr", "PUT", fmt.Sprintf("notification/%d", id), pulsarrHook(id, "box", other.srv.URL+"/x", maskedKey))
	failedOn(t, st, body, "Headers")
	if len(other.got()) != 0 {
		t.Fatal("the stored secret was sent to another host")
	}
	// The test route: 2xx when the receiver says 2xx, else a failed check
	// on the address. It may use the stored hook's secret.
	before := len(rc.got())
	if st, body := h.do("sonarr", "POST", "notification/test", pulsarrHook(id, "box", moved, maskedKey)); st != 200 {
		t.Fatalf("test: %d %s", st, body)
	}
	if c := rc.got(); len(c) != before+1 || c[before].header.Get("X-Pulsarr-Secret") != "second" || c[before].body["eventType"] != "Test" {
		t.Fatalf("test call: %+v", c)
	}
	rc.answer(500)
	st, body = h.do("sonarr", "POST", "notification/test", pulsarrHook(0, "box", moved, "second"))
	failedOn(t, st, body, "Url")
	rc.answer(200)
	if st, _ := h.do("sonarr", "PUT", "notification/999", pulsarrHook(999, "box", moved, "x")); st != 404 {
		t.Fatalf("unknown id: %d", st)
	}
	// The other side cannot reach this side's hook.
	if st, _ := h.do("radarr", "DELETE", fmt.Sprintf("notification/%d", id), ""); st != 404 {
		t.Fatalf("other side delete: %d", st)
	}
	var secretID int64
	_ = h.st.Read().QueryRow("SELECT secret_id FROM compat_hooks WHERE id = ?", id).Scan(&secretID)
	if st, body := h.do("sonarr", "DELETE", fmt.Sprintf("notification/%d", id), ""); st != 200 {
		t.Fatalf("delete: %d %s", st, body)
	}
	if st, _ := h.do("sonarr", "DELETE", fmt.Sprintf("notification/%d", id), ""); st != 404 {
		t.Fatalf("delete again: %d", st)
	}
	if _, err := h.vault.Get(context.Background(), secretID); err == nil {
		t.Fatal("the hook's secret stayed in the vault")
	}
}

// Bad bodies are refused before anything is called: an address that is not
// http or https with a host, a sign-in inside the address, a header that
// would change how the call is framed, and records of another kind.
func TestHooks_RefusedBodies(t *testing.T) {
	h := newHookRig(t)
	rc := newReceiver(t)
	for _, c := range []struct{ body, property string }{
		{pulsarrHook(0, "a", "ftp://example.invalid/x", "s"), "Url"},
		{pulsarrHook(0, "a", "http:///nohost", "s"), "Url"},
		{pulsarrHook(0, "a", "http://user:pw@"+strings.TrimPrefix(rc.srv.URL, "http://")+"/x", "s"), "Url"},
		{strings.Replace(pulsarrHook(0, "a", rc.srv.URL, "s"), "X-Pulsarr-Secret", "Content-Length", 1), "Headers"},
		{strings.Replace(pulsarrHook(0, "a", rc.srv.URL, "s"), "X-Pulsarr-Secret", "Host", 1), "Headers"},
		{strings.Replace(pulsarrHook(0, "a", rc.srv.URL, "s"), "X-Pulsarr-Secret", "Bad Name", 1), "Headers"},
		{strings.Replace(pulsarrHook(0, "a", rc.srv.URL, "s"), `"value":"s"`, `"value":"x\r\nInjected: 1"`, 1), "Headers"},
		{strings.Replace(pulsarrHook(0, "a", rc.srv.URL, "s"), `"implementation":"Webhook"`, `"implementation":"PlexServer"`, 1), "Implementation"},
		{strings.Replace(pulsarrHook(0, "a", rc.srv.URL, "s"), `"name":"a"`, `"name":""`, 1), "Name"},
	} {
		st, body := h.do("sonarr", "POST", "notification", c.body)
		failedOn(t, st, body, c.property)
	}
	if n := len(rc.got()); n != 0 {
		t.Fatalf("%d calls for refused bodies", n)
	}
	if st, _ := h.do("sonarr", "POST", "notification", `{"name":`); st != 400 {
		t.Fatalf("broken JSON: %d", st)
	}
}

// Hooks make HolyArrs call out, so only a key whose maker may change
// settings can write or test them. Anyone on the side may list them.
func TestHooks_NeedSettingsPermission(t *testing.T) {
	h := newHookRig(t)
	rc := newReceiver(t)
	h.caller = webserver.Caller{UserID: 2, Name: "staff", Via: "api_key", Role: "member", Perms: []webserver.Perm{webserver.PermManageLibrary}}
	for _, c := range [][2]string{{"POST", "notification"}, {"POST", "notification/test"}, {"PUT", "notification/1"}, {"DELETE", "notification/1"}} {
		if st, _ := h.do("sonarr", c[0], c[1], pulsarrHook(0, "a", rc.srv.URL, "s")); st != 403 {
			t.Errorf("%s %s: %d", c[0], c[1], st)
		}
	}
	if st, _ := h.do("sonarr", "GET", "notification", ""); st != 200 {
		t.Errorf("list: %d", st)
	}
	if len(rc.got()) != 0 {
		t.Fatal("a call went out")
	}
}

// The test call stops in time, inside Pulsarr's own ten-second wait.
func TestHooks_TestStopsInTime(t *testing.T) {
	old := hookTestTimeout
	hookTestTimeout = 300 * time.Millisecond
	t.Cleanup(func() { hookTestTimeout = old })
	h := newHookRig(t)
	rc := newReceiver(t)
	rc.delay = 2 * time.Second
	start := time.Now()
	st, body := h.do("radarr", "POST", "notification/test", pulsarrHook(0, "a", rc.srv.URL, "s"))
	failedOn(t, st, body, "Url")
	if time.Since(start) > 1500*time.Millisecond {
		t.Fatalf("the test took %s", time.Since(start))
	}
}
