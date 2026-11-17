package plugins_test

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/plugins"
	"github.com/HolyArrs/holyarrs/app/backend/internal/plugins/pluginstest"
)

// listServer serves a signed plugin list, blocklist, manifest and file over
// TLS, like the real list server.
type listServer struct {
	t    *testing.T
	priv ed25519.PrivateKey
	srv  *httptest.Server

	mu       sync.Mutex
	list     []byte
	block    []byte
	manifest []byte
	file     []byte
	version  string
	redirect string // when set, plugins.json redirects here
}

var pluginFile = []byte("plugin program bytes")

func newListServer(t *testing.T) (*listServer, ed25519.PublicKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	ls := &listServer{t: t, priv: priv, file: pluginFile, version: "1.0.0"}
	sum := sha256.Sum256(pluginFile)
	sha := hex.EncodeToString(sum[:])
	zero := uint64(0)
	ls.block = ls.sign(plugins.List{Kind: plugins.KindBlocklist, Counter: 1, Entries: []plugins.Entry{}})
	ls.manifest, _ = json.Marshal(map[string]any{
		"id": "tailscale", "name": "Tailscale", "version": "1.0.0", "author": "HolyArrs",
		"access":          map[string]any{"hosts": []string{"controlplane.tailscale.com"}, "folders": []string{}, "actions": []string{}},
		"min_app_version": "1.0.0", "sha256": sha,
	})
	ls.list = ls.sign(plugins.List{Kind: plugins.KindPlugins, Counter: 1, MinBlocklistCounter: &zero,
		Entries: []plugins.Entry{{ID: "tailscale", Version: "1.0.0", SHA256: sha, ManifestSHA256: hashOf(ls.manifest)}}})
	ls.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ls.mu.Lock()
		defer ls.mu.Unlock()
		base := "/list/plugins/tailscale/" + ls.version
		switch r.URL.Path {
		case "/list/plugins.json":
			if ls.redirect != "" {
				http.Redirect(w, r, ls.redirect, http.StatusFound)
				return
			}
			w.Write(ls.list)
		case "/list/blocklist.json":
			w.Write(ls.block)
		case base + "/manifest.json":
			w.Write(ls.manifest)
		case base + "/plugin":
			w.Write(ls.file)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(ls.srv.Close)
	return ls, pub
}

func hashOf(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func (ls *listServer) sign(l plugins.List) []byte {
	ls.t.Helper()
	b, err := plugins.SignList(ls.priv, l)
	if err != nil {
		ls.t.Fatal(err)
	}
	return b
}

func newEngine(t *testing.T, ls *listServer, keys ...ed25519.PublicKey) (*plugins.Engine, *pluginstest.FakeRunner) {
	t.Helper()
	run := &pluginstest.FakeRunner{}
	e := &plugins.Engine{Dir: filepath.Join(t.TempDir(), "plugins"), ListURL: ls.srv.URL + "/list", Keys: keys,
		Runner: run, HTTP: ls.srv.Client()}
	t.Cleanup(e.StopAll)
	return e, run
}

func call(t *testing.T, h http.Handler, method, path string, want int) map[string]any {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	if rec.Code != want {
		t.Fatalf("%s %s = %d %s, want %d", method, path, rec.Code, rec.Body, want)
	}
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return out
}

func mux(e *plugins.Engine) *http.ServeMux {
	m := http.NewServeMux()
	(&plugins.Handlers{Engine: e}).Routes(m)
	return m
}

func row(t *testing.T, ov map[string]any, id string) map[string]any {
	t.Helper()
	for _, p := range ov["plugins"].([]any) {
		if p := p.(map[string]any); p["id"] == id {
			return p
		}
	}
	t.Fatalf("plugin %s not listed: %v", id, ov)
	return nil
}

func TestInstallStartStopRemove(t *testing.T) {
	ls, pub := newListServer(t)
	e, run := newEngine(t, ls, pub)
	m := mux(e)

	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusOK)
	if len(run.Starts) != 1 || run.Starts[0].ID != "tailscale" {
		t.Fatalf("starts after install: %+v", run.Starts)
	}
	p := row(t, call(t, m, "GET", "/api/v1/plugins", http.StatusOK), "tailscale")
	if p["installed"] != true || p["running"] != true || p["enabled"] != true || p["installed_version"] != "1.0.0" || p["access"] == nil {
		t.Fatalf("after install: %v", p)
	}

	call(t, m, "POST", "/api/v1/plugins/tailscale/stop", http.StatusOK)
	p = row(t, call(t, m, "GET", "/api/v1/plugins", http.StatusOK), "tailscale")
	if p["running"] != false || p["enabled"] != false || p["disabled_reason"] == "" {
		t.Fatalf("after stop: %v", p)
	}

	call(t, m, "POST", "/api/v1/plugins/tailscale/start", http.StatusOK)
	if len(run.Starts) != 2 {
		t.Fatalf("starts after start: %d", len(run.Starts))
	}

	call(t, m, "DELETE", "/api/v1/plugins/tailscale?keep=1", http.StatusOK)
	p = row(t, call(t, m, "GET", "/api/v1/plugins", http.StatusOK), "tailscale")
	if p["installed"] != true || p["running"] != false || p["enabled"] != false {
		t.Fatalf("after remove keep: %v", p)
	}

	call(t, m, "DELETE", "/api/v1/plugins/tailscale", http.StatusOK)
	p = row(t, call(t, m, "GET", "/api/v1/plugins", http.StatusOK), "tailscale")
	if p["installed"] != false {
		t.Fatalf("after remove: %v", p)
	}
	if _, err := os.Stat(filepath.Join(e.Dir, "tailscale")); !os.IsNotExist(err) {
		t.Fatalf("plugin folder still there: %v", err)
	}
	call(t, m, "DELETE", "/api/v1/plugins/tailscale", http.StatusNotFound)
}

func TestStartEnabledOnAppStart(t *testing.T) {
	ls, pub := newListServer(t)
	e, _ := newEngine(t, ls, pub)
	if _, err := e.Install(context.Background(), "tailscale"); err != nil {
		t.Fatal(err)
	}
	e.StopAll()

	// A new Engine on the same folder, as after a restart.
	run := &pluginstest.FakeRunner{}
	e2 := &plugins.Engine{Dir: e.Dir, ListURL: e.ListURL, Keys: e.Keys, Runner: run, HTTP: e.HTTP}
	defer e2.StopAll()
	e2.StartEnabled()
	if len(run.Starts) != 1 {
		t.Fatalf("enabled plugin not started: %d", len(run.Starts))
	}

	// A tampered program fails CheckStart and stays disabled.
	e2.StopAll()
	prog, _ := filepath.Glob(filepath.Join(e.Dir, "tailscale", "1.0.0-*", "program"))
	if len(prog) != 1 {
		t.Fatalf("program: %v", prog)
	}
	if err := os.WriteFile(prog[0], []byte("swapped"), 0o600); err != nil {
		t.Fatal(err)
	}
	run3 := &pluginstest.FakeRunner{}
	e3 := &plugins.Engine{Dir: e.Dir, ListURL: e.ListURL, Keys: e.Keys, Runner: run3, HTTP: e.HTTP}
	e3.StartEnabled()
	if len(run3.Starts) != 0 {
		t.Fatal("tampered plugin started")
	}
	p := row(t, call(t, mux(e3), "GET", "/api/v1/plugins", http.StatusOK), "tailscale")
	if p["enabled"] != false || !strings.Contains(p["disabled_reason"].(string), "does not match") {
		t.Fatalf("tampered plugin: %v", p)
	}
}

func TestStartEnabledDoesNothingBeforeFirstInstall(t *testing.T) {
	ls, pub := newListServer(t)
	e, run := newEngine(t, ls, pub)
	e.StartEnabled()
	if len(run.Starts) != 0 {
		t.Fatal("started without an install")
	}
	if _, err := os.Stat(e.Dir); !os.IsNotExist(err) {
		t.Fatalf("plugins folder made before an install: %v", err)
	}
}

func TestInstallRefusedWithoutPinnedKey(t *testing.T) {
	ls, _ := newListServer(t)
	e, run := newEngine(t, ls)
	out := call(t, mux(e), "POST", "/api/v1/plugins/tailscale/install", http.StatusConflict)
	if out["error"] != "plugin signing key not set yet" {
		t.Fatalf("error = %v", out["error"])
	}
	if len(run.Starts) != 0 {
		t.Fatal("started without a key")
	}
	ov := call(t, mux(e), "GET", "/api/v1/plugins", http.StatusOK)
	if ov["key_set"] != false {
		t.Fatalf("key_set: %v", ov)
	}
}

func TestInstallRefusedOffLinux(t *testing.T) {
	ls, pub := newListServer(t)
	e, _ := newEngine(t, ls, pub)
	e.Runner = nil
	out := call(t, mux(e), "POST", "/api/v1/plugins/tailscale/install", http.StatusNotImplemented)
	if out["error"] != "plugins run only on Linux" {
		t.Fatalf("error = %v", out["error"])
	}
}

func TestInstallRefusesBadSignature(t *testing.T) {
	ls, _ := newListServer(t)
	other, _, _ := ed25519.GenerateKey(nil)
	e, run := newEngine(t, ls, other)
	out := call(t, mux(e), "POST", "/api/v1/plugins/tailscale/install", http.StatusBadGateway)
	if !strings.Contains(out["error"].(string), "signature") {
		t.Fatalf("error = %v", out["error"])
	}
	if len(run.Starts) != 0 {
		t.Fatal("started with a bad signature")
	}
	if ov := call(t, mux(e), "GET", "/api/v1/plugins", http.StatusOK); len(ov["plugins"].([]any)) != 0 {
		t.Fatalf("listed from a badly signed list: %v", ov)
	}
}

func TestInstallRefusesPlainHTTP(t *testing.T) {
	ls, pub := newListServer(t)
	e, _ := newEngine(t, ls, pub)
	e.ListURL = strings.Replace(e.ListURL, "https://", "http://", 1)
	call(t, mux(e), "POST", "/api/v1/plugins/tailscale/install", http.StatusBadGateway)
}

func TestBlocklistCheckStopsAndDisables(t *testing.T) {
	ls, pub := newListServer(t)
	e, _ := newEngine(t, ls, pub)
	if _, err := e.Install(context.Background(), "tailscale"); err != nil {
		t.Fatal(err)
	}
	// A failed fetch changes nothing.
	ls.mu.Lock()
	ls.block = []byte("not a list")
	ls.mu.Unlock()
	if err := e.CheckBlocklist(context.Background()); err == nil {
		t.Fatal("bad blocklist accepted")
	}
	if p := row(t, call(t, mux(e), "GET", "/api/v1/plugins", http.StatusOK), "tailscale"); p["running"] != true {
		t.Fatalf("stopped by a bad blocklist: %v", p)
	}

	ls.mu.Lock()
	ls.block = ls.sign(plugins.List{Kind: plugins.KindBlocklist, Counter: 2, Entries: []plugins.Entry{{ID: "tailscale"}}})
	ls.mu.Unlock()
	e.BlockEvery = 10 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { e.Run(ctx); close(done) }()
	deadline := 500
	var p map[string]any
	for ; deadline > 0; deadline-- {
		p = row(t, call(t, mux(e), "GET", "/api/v1/plugins", http.StatusOK), "tailscale")
		if p["enabled"] == false {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done
	if p["enabled"] != false || p["running"] != false || !strings.Contains(p["disabled_reason"].(string), "blocklist") {
		t.Fatalf("blocklisted plugin: %v", p)
	}
	// Starting it again is refused and it stays disabled.
	call(t, mux(e), "POST", "/api/v1/plugins/tailscale/start", http.StatusUnprocessableEntity)
}

// publish serves a new signed version of tailscale with list counter n.
func (ls *listServer) publish(version string, file []byte, n uint64) {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	sha := hashOf(file)
	ls.version, ls.file = version, file
	ls.manifest, _ = json.Marshal(map[string]any{
		"id": "tailscale", "name": "Tailscale", "version": version, "author": "HolyArrs",
		"access":          map[string]any{"hosts": []string{}, "folders": []string{}, "actions": []string{}},
		"min_app_version": "1.0.0", "sha256": sha,
	})
	zero := uint64(0)
	ls.list = ls.sign(plugins.List{Kind: plugins.KindPlugins, Counter: n, MinBlocklistCounter: &zero,
		Entries: []plugins.Entry{{ID: "tailscale", Version: version, SHA256: sha, ManifestSHA256: hashOf(ls.manifest)}}})
}

// Before install the row already shows the signed manifest's access; a
// manifest that no longer matches the list's hash shows none.
func TestListShowsListedManifest(t *testing.T) {
	ls, pub := newListServer(t)
	e, _ := newEngine(t, ls, pub)
	p := row(t, call(t, mux(e), "GET", "/api/v1/plugins", http.StatusOK), "tailscale")
	if p["installed"] != false || p["access"] == nil || p["name"] != "Tailscale" || p["author"] != "HolyArrs" {
		t.Fatalf("listed row: %v", p)
	}

	ls2, pub2 := newListServer(t)
	e2, _ := newEngine(t, ls2, pub2)
	ls2.mu.Lock()
	var m map[string]any
	_ = json.Unmarshal(ls2.manifest, &m)
	m["description"] = "changed after signing"
	ls2.manifest, _ = json.Marshal(m)
	ls2.mu.Unlock()
	if p := row(t, call(t, mux(e2), "GET", "/api/v1/plugins", http.StatusOK), "tailscale"); p["access"] != nil || p["description"] != nil {
		t.Fatalf("tampered manifest shown: %v", p)
	}
}

func TestInstallRefusesTamperedManifest(t *testing.T) {
	ls, pub := newListServer(t)
	e, run := newEngine(t, ls, pub)
	ls.mu.Lock()
	var m map[string]any
	_ = json.Unmarshal(ls.manifest, &m)
	m["access"] = map[string]any{"hosts": []string{"evil.example"}, "folders": []string{}, "actions": []string{}}
	ls.manifest, _ = json.Marshal(m) // program hash still matches
	ls.mu.Unlock()
	out := call(t, mux(e), "POST", "/api/v1/plugins/tailscale/install", http.StatusUnprocessableEntity)
	if !strings.Contains(out["error"].(string), "manifest") || len(run.Starts) != 0 {
		t.Fatalf("tampered manifest: %v, starts %d", out, len(run.Starts))
	}
}

func TestInstallRefusesRedirectToHTTP(t *testing.T) {
	ls, pub := newListServer(t)
	e, _ := newEngine(t, ls, pub)
	ls.redirect = "http://" + strings.TrimPrefix(ls.srv.URL, "https://") + "/list/plugins.json"
	out := call(t, mux(e), "POST", "/api/v1/plugins/tailscale/install", http.StatusBadGateway)
	if !strings.Contains(out["error"].(string), "redirect") {
		t.Fatalf("error = %v", out["error"])
	}
	ls.redirect = "https://other.example/list/plugins.json"
	out = call(t, mux(e), "POST", "/api/v1/plugins/tailscale/install", http.StatusBadGateway)
	if !strings.Contains(out["error"].(string), "redirect") {
		t.Fatalf("other host: error = %v", out["error"])
	}
}

func TestConcurrentStartDuringInstallRunsNewVersion(t *testing.T) {
	ls, pub := newListServer(t)
	e, run := newEngine(t, ls, pub)
	if _, err := e.Install(context.Background(), "tailscale"); err != nil {
		t.Fatal(err)
	}
	two := []byte("plugin program bytes, version two")
	ls.publish("1.1.0", two, 2)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if _, err := e.Install(context.Background(), "tailscale"); err != nil {
			t.Error(err)
		}
	}()
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _ = e.Start(context.Background(), "tailscale") }()
	}
	wg.Wait()
	last := run.Starts[len(run.Starts)-1]
	if !strings.Contains(last.Program, "1.1.0-") {
		t.Fatalf("running %s, want version 1.1.0", last.Program)
	}
	p := row(t, call(t, mux(e), "GET", "/api/v1/plugins", http.StatusOK), "tailscale")
	if p["installed_version"] != "1.1.0" || p["running"] != true {
		t.Fatalf("after install: %v", p)
	}
}

func TestFailedStartDisables(t *testing.T) {
	ls, pub := newListServer(t)
	e, run := newEngine(t, ls, pub)
	m := mux(e)
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusOK)
	ov := call(t, m, "GET", "/api/v1/plugins", http.StatusOK)
	if ov["blocklist_checked_at"] == nil || ov["blocklist_stale"] != false {
		t.Fatalf("blocklist check time: %v", ov)
	}
	call(t, m, "POST", "/api/v1/plugins/tailscale/stop", http.StatusOK)
	run.Err = errors.New("sandbox refused")
	if err := e.Start(context.Background(), "tailscale"); err == nil {
		t.Fatal("start succeeded")
	}
	p := row(t, call(t, m, "GET", "/api/v1/plugins", http.StatusOK), "tailscale")
	if p["enabled"] != false || !strings.Contains(p["disabled_reason"].(string), "sandbox refused") {
		t.Fatalf("after failed start: %v", p)
	}
}

func TestPluginWritesRefuseAPIKeys(t *testing.T) {
	ls, pub := newListServer(t)
	e, run := newEngine(t, ls, pub)
	m := mux(e)
	for _, r := range []struct{ method, path string }{
		{"POST", "/api/v1/plugins/tailscale/install"}, {"POST", "/api/v1/plugins/tailscale/start"},
		{"POST", "/api/v1/plugins/tailscale/stop"}, {"DELETE", "/api/v1/plugins/tailscale"},
	} {
		req := httptest.NewRequest(r.method, r.path, nil)
		req = req.WithContext(webserver.ContextWith(req.Context(), webserver.Caller{KeyID: 7, Scopes: []string{webserver.ScopeAdmin}}))
		rec := httptest.NewRecorder()
		m.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s %s with an admin key = %d %s", r.method, r.path, rec.Code, rec.Body)
		}
	}
	if len(run.Starts) != 0 {
		t.Fatal("installed with an API key")
	}
}

func TestTailscaleStatusAndSignOut(t *testing.T) {
	ls, pub := newListServer(t)
	e, run := newEngine(t, ls, pub)
	m := mux(e)

	call(t, m, "GET", "/api/v1/plugins/tailscale/status", http.StatusNotFound)
	call(t, m, "POST", "/api/v1/plugins/tailscale/signout", http.StatusNotFound)

	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusOK)
	if st := call(t, m, "GET", "/api/v1/plugins/tailscale/status", http.StatusOK); st["state"] != "not_connected" {
		t.Fatalf("status before the plugin wrote one: %v", st)
	}
	data := run.Starts[0].DataDir
	body := `{"state":"waiting_for_sign_in","login_url":"https://login.tailscale.com/a/x","last_updated":"2026-11-16T00:00:00Z"}`
	if err := os.WriteFile(filepath.Join(data, "status.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	st := call(t, m, "GET", "/api/v1/plugins/tailscale/status", http.StatusOK)
	if st["state"] != "waiting_for_sign_in" || st["login_url"] != "https://login.tailscale.com/a/x" || st["login_qr"] == nil {
		t.Fatalf("status: %v", st)
	}

	// An API key never sees the sign-in link.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/plugins/tailscale/status", nil)
	req = req.WithContext(webserver.ContextWith(req.Context(), webserver.Caller{KeyID: 7}))
	m.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "login_") {
		t.Fatalf("status through a key = %d %s", rec.Code, rec.Body)
	}

	if out := call(t, m, "POST", "/api/v1/plugins/tailscale/signout", http.StatusOK); out["removed_remotely"] != true {
		t.Fatalf("sign-out answer: %v", out)
	}
	if len(run.Starts) != 2 || len(run.Starts[1].Args) != 1 || run.Starts[1].Args[0] != "--sign-out" {
		t.Fatalf("sign-out run: %+v", run.Starts)
	}
	p := row(t, call(t, m, "GET", "/api/v1/plugins", http.StatusOK), "tailscale")
	if p["running"] != false || p["enabled"] != false {
		t.Fatalf("after sign-out: %v", p)
	}
}

// exitErr stands in for *exec.ExitError.
type exitErr int

func (e exitErr) Error() string { return "exit status" }
func (e exitErr) ExitCode() int { return int(e) }

func TestTailscaleSignOutLocalOnly(t *testing.T) {
	ls, pub := newListServer(t)
	e, run := newEngine(t, ls, pub)
	m := mux(e)
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusOK)
	run.OneShot = func(plugins.Spec) error { return exitErr(3) }
	out := call(t, m, "POST", "/api/v1/plugins/tailscale/signout", http.StatusOK)
	if out["signed_out"] != true || out["removed_remotely"] != false {
		t.Fatalf("logout failed, state wiped: %v", out)
	}
	run.OneShot = func(plugins.Spec) error { return exitErr(1) }
	call(t, m, "POST", "/api/v1/plugins/tailscale/signout", http.StatusInternalServerError)
}

func TestTailscaleSignOutKeepsSecurityReason(t *testing.T) {
	ls, pub := newListServer(t)
	e, run := newEngine(t, ls, pub)
	m := mux(e)
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusOK)
	// The blocklist turns the plugin off while the sign-out runs.
	run.OneShot = func(plugins.Spec) error {
		st, err := plugins.NewStore(e.Dir)
		if err != nil {
			return err
		}
		return st.SetEnabled("tailscale", false, "on the blocklist")
	}
	call(t, m, "POST", "/api/v1/plugins/tailscale/signout", http.StatusOK)
	p := row(t, call(t, m, "GET", "/api/v1/plugins", http.StatusOK), "tailscale")
	if p["enabled"] != false || p["disabled_reason"] != "on the blocklist" {
		t.Fatalf("security reason overwritten: %v", p)
	}
	// A plugin already off for a security reason is not signed out at all.
	run.OneShot = nil
	call(t, m, "POST", "/api/v1/plugins/tailscale/signout", http.StatusUnprocessableEntity)
	p = row(t, call(t, m, "GET", "/api/v1/plugins", http.StatusOK), "tailscale")
	if p["disabled_reason"] != "on the blocklist" {
		t.Fatalf("after refused sign-out: %v", p)
	}
}

func TestTailscaleStatusSafeRead(t *testing.T) {
	ls, pub := newListServer(t)
	e, run := newEngine(t, ls, pub)
	m := mux(e)
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusOK)
	data := run.Starts[0].DataDir
	status := filepath.Join(data, "status.json")
	write := func(body string) {
		t.Helper()
		os.Remove(status)
		if err := os.WriteFile(status, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	// Unknown fields are dropped; only the known ones come back.
	write(`{"state":"connected","tsnet_name":"holyarrs","secret_thing":"x"}`)
	st := call(t, m, "GET", "/api/v1/plugins/tailscale/status", http.StatusOK)
	if st["state"] != "connected" || st["tsnet_name"] != "holyarrs" || st["secret_thing"] != nil {
		t.Fatalf("status fields: %v", st)
	}

	// Sign-in links off Tailscale's host, or not https, are dropped with no QR.
	for _, link := range []string{"http://login.tailscale.com/a/x", "https://evil.example/a/x", "javascript:alert(1)", "https://login.tailscale.com.evil.example/a"} {
		write(`{"state":"waiting_for_sign_in","login_url":"` + link + `"}`)
		st = call(t, m, "GET", "/api/v1/plugins/tailscale/status", http.StatusOK)
		if st["login_url"] != nil || st["login_qr"] != nil {
			t.Fatalf("untrusted link %q kept: %v", link, st)
		}
	}
	// The configured login server's host is trusted.
	hm := http.NewServeMux()
	(&plugins.Handlers{Engine: e, LoginServer: "https://hs.example.net"}).Routes(hm)
	write(`{"state":"waiting_for_sign_in","login_url":"https://hs.example.net/register/x"}`)
	if st = call(t, hm, "GET", "/api/v1/plugins/tailscale/status", http.StatusOK); st["login_url"] == nil || st["login_qr"] == nil {
		t.Fatalf("login server link dropped: %v", st)
	}

	// Anything but a regular file is refused.
	os.Remove(status)
	if err := os.Mkdir(status, 0o700); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/plugins/tailscale/status", nil))
	if rec.Code == http.StatusOK {
		t.Fatalf("directory read as status: %s", rec.Body)
	}
	os.Remove(status)

	// Only the first 64 KiB is read: a huge file fails to decode.
	write(`{"state":"connected","tsnet_name":"` + strings.Repeat("a", 70<<10) + `"}`)
	rec = httptest.NewRecorder()
	m.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/plugins/tailscale/status", nil))
	if rec.Code == http.StatusOK {
		t.Fatalf("oversized status read: %d", rec.Body.Len())
	}
}

// TestListMissingListIsCalm: no list server yet (404) shows no plugins and
// no problem; a list with a bad signature is flagged.
func TestListMissingListIsCalm(t *testing.T) {
	ls, pub := newListServer(t)
	e, _ := newEngine(t, ls, pub)
	e.ListURL = ls.srv.URL + "/nowhere"
	out := call(t, mux(e), "GET", "/api/v1/plugins", http.StatusOK)
	if out["list_bad"] != false || len(out["plugins"].([]any)) != 0 {
		t.Fatalf("missing list: %v", out)
	}

	ls2, _ := newListServer(t)
	other, _, _ := ed25519.GenerateKey(nil)
	e2, _ := newEngine(t, ls2, other)
	if out := call(t, mux(e2), "GET", "/api/v1/plugins", http.StatusOK); out["list_bad"] != true {
		t.Fatalf("bad signature not flagged: %v", out)
	}
}
