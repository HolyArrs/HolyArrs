package plugins_test

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

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
}

var pluginFile = []byte("plugin program bytes")

func newListServer(t *testing.T) (*listServer, ed25519.PublicKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	ls := &listServer{t: t, priv: priv, file: pluginFile}
	sum := sha256.Sum256(pluginFile)
	sha := hex.EncodeToString(sum[:])
	zero := uint64(0)
	ls.list = ls.sign(plugins.List{Kind: plugins.KindPlugins, Counter: 1, MinBlocklistCounter: &zero,
		Entries: []plugins.Entry{{ID: "tailscale", Version: "1.0.0", SHA256: sha}}})
	ls.block = ls.sign(plugins.List{Kind: plugins.KindBlocklist, Counter: 1, Entries: []plugins.Entry{}})
	ls.manifest, _ = json.Marshal(map[string]any{
		"id": "tailscale", "name": "Tailscale", "version": "1.0.0", "author": "HolyArrs",
		"access":          map[string]any{"hosts": []string{"controlplane.tailscale.com"}, "folders": []string{}, "actions": []string{}},
		"min_app_version": "1.0.0", "sha256": sha,
	})
	ls.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ls.mu.Lock()
		defer ls.mu.Unlock()
		switch r.URL.Path {
		case "/list/plugins.json":
			w.Write(ls.list)
		case "/list/blocklist.json":
			w.Write(ls.block)
		case "/list/plugins/tailscale/1.0.0/manifest.json":
			w.Write(ls.manifest)
		case "/list/plugins/tailscale/1.0.0/plugin":
			w.Write(ls.file)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(ls.srv.Close)
	return ls, pub
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
