package plugins_test

import (
	"bufio"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/plugins"
)

// devFolder writes a plugin into the developer folder of e.
func devFolder(t *testing.T, e *plugins.Engine, id, version string, prog []byte, events ...string) {
	t.Helper()
	dir := filepath.Join(e.DevDir(), id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	m := map[string]any{"id": id, "name": "Dev " + id, "version": version, "author": "Me",
		"access":          map[string]any{"hosts": []string{}, "folders": []string{}, "actions": []string{}, "events": events},
		"min_app_version": "1.0.0", "icon_file": "icon.png"}
	if len(events) == 0 {
		delete(m["access"].(map[string]any), "events")
	}
	b, _ := json.Marshal(m)
	if err := os.WriteFile(filepath.Join(dir, "holyarrs-plugin.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin"), prog, 0o700); err != nil {
		t.Fatal(err)
	}
}

func putOn(t *testing.T, h http.Handler, path string, on bool, want int) {
	t.Helper()
	b, _ := json.Marshal(map[string]bool{"on": on})
	callBody(t, h, "PUT", path, string(b), want)
}

// With developer mode off a developer plugin is neither listed nor
// installed; on, it installs unsigned, runs, and shows as a developer one.
func TestDevModeInstallsUnsignedPlugin(t *testing.T) {
	ls, pub := newListServer(t)
	e, run := newEngine(t, ls, pub)
	m := mux(e)
	devFolder(t, e, "my-plugin", "0.1.0", []byte("dev program v1"))
	ov := call(t, m, "GET", "/api/v1/plugins", http.StatusOK)
	if ov["dev_mode"] != false {
		t.Fatalf("dev mode on by default: %v", ov["dev_mode"])
	}
	for _, p := range ov["plugins"].([]any) {
		if p.(map[string]any)["id"] == "my-plugin" {
			t.Fatal("developer plugin listed with developer mode off")
		}
	}
	call(t, m, "POST", "/api/v1/plugins/my-plugin/dev-install", http.StatusUnprocessableEntity)

	putOn(t, m, "/api/v1/plugins/dev-mode", true, http.StatusOK)
	ov = call(t, m, "GET", "/api/v1/plugins", http.StatusOK)
	if ov["dev_dir"] == "" || row(t, ov, "my-plugin")["dev"] != true || row(t, ov, "my-plugin")["installed"] != false {
		t.Fatalf("dev row: %v", ov)
	}
	call(t, m, "POST", "/api/v1/plugins/my-plugin/dev-install", http.StatusOK)
	if len(run.Starts) != 1 || run.Starts[0].ID != "my-plugin" {
		t.Fatalf("starts %+v", run.Starts)
	}
	p := row(t, call(t, m, "GET", "/api/v1/plugins", http.StatusOK), "my-plugin")
	if p["installed"] != true || p["running"] != true || p["dev"] != true || p["installed_version"] != "0.1.0" {
		t.Fatalf("installed dev row: %v", p)
	}
}

// A reload picks up a new build; turning developer mode off stops the
// plugin and it can't be started again until it's back on and reloaded.
func TestDevReloadAndOff(t *testing.T) {
	ls, pub := newListServer(t)
	e, run := newEngine(t, ls, pub)
	m := mux(e)
	putOn(t, m, "/api/v1/plugins/dev-mode", true, http.StatusOK)
	devFolder(t, e, "my-plugin", "0.1.0", []byte("dev program v1"))
	call(t, m, "POST", "/api/v1/plugins/my-plugin/dev-install", http.StatusOK)
	devFolder(t, e, "my-plugin", "0.1.0", []byte("dev program v2"))
	call(t, m, "POST", "/api/v1/plugins/my-plugin/dev-install", http.StatusOK)
	got, _ := os.ReadFile(run.Starts[len(run.Starts)-1].Program)
	if string(got) != "dev program v2" {
		t.Fatalf("reload ran %q", got)
	}
	putOn(t, m, "/api/v1/plugins/dev-mode", false, http.StatusOK)
	p := row(t, call(t, m, "GET", "/api/v1/plugins", http.StatusOK), "my-plugin")
	if p["running"] != false || p["enabled"] != false || !strings.Contains(p["disabled_reason"].(string), "developer mode is off") {
		t.Fatalf("after dev mode off: %v", p)
	}
	n := len(run.Starts)
	call(t, m, "POST", "/api/v1/plugins/my-plugin/start", http.StatusUnprocessableEntity)
	if len(run.Starts) != n {
		t.Fatal("developer plugin started with developer mode off")
	}
	putOn(t, m, "/api/v1/plugins/dev-mode", true, http.StatusOK)
	call(t, m, "POST", "/api/v1/plugins/my-plugin/dev-install", http.StatusOK)
	if len(run.Starts) != n+1 {
		t.Fatal("reload after developer mode on did not start it")
	}
}

// A developer plugin whose installed program changed on disk is refused at
// start, like a signed one; a manifest naming another id is refused.
func TestDevTamperAndBadManifest(t *testing.T) {
	ls, pub := newListServer(t)
	e, run := newEngine(t, ls, pub)
	m := mux(e)
	putOn(t, m, "/api/v1/plugins/dev-mode", true, http.StatusOK)
	devFolder(t, e, "my-plugin", "0.1.0", []byte("dev program v1"))
	call(t, m, "POST", "/api/v1/plugins/my-plugin/dev-install", http.StatusOK)
	call(t, m, "POST", "/api/v1/plugins/my-plugin/stop", http.StatusOK)
	prog := filepath.Join(filepath.Dir(run.Starts[0].Program), "program")
	if err := os.WriteFile(prog, []byte("swapped"), 0o700); err != nil {
		t.Fatal(err)
	}
	call(t, m, "POST", "/api/v1/plugins/my-plugin/start", http.StatusUnprocessableEntity)

	devFolder(t, e, "other", "0.1.0", []byte("x"))
	b, _ := json.Marshal(map[string]any{"id": "not-other", "name": "x", "version": "0.1.0", "author": "x",
		"access": map[string]any{"hosts": []string{}, "folders": []string{}, "actions": []string{}}, "min_app_version": "1.0.0"})
	_ = os.WriteFile(filepath.Join(e.DevDir(), "other", "holyarrs-plugin.json"), b, 0o600)
	call(t, m, "POST", "/api/v1/plugins/other/dev-install", http.StatusUnprocessableEntity)
	call(t, m, "POST", "/api/v1/plugins/missing/dev-install", http.StatusNotFound)
	if filepath.Base(e.DevDir()) != "_dev" {
		t.Fatalf("developer folder %s can meet a plugin id", e.DevDir())
	}
}

// A developer plugin's events still need the owner's approval, and Send a
// test event writes one line marked test; signed plugins can't be sent one.
func TestDevTestEvent(t *testing.T) {
	ls, pub := newListServer(t)
	ls.publishWith("1.0.0", pluginFile, 2, eventsManifest("library.add"))
	e, run := newEngine(t, ls, pub)
	m := mux(e)
	putOn(t, m, "/api/v1/plugins/dev-mode", true, http.StatusOK)
	devFolder(t, e, "my-plugin", "0.1.0", []byte("dev program v1"), "library.add")
	call(t, m, "POST", "/api/v1/plugins/my-plugin/dev-install", http.StatusOK)
	r := bufio.NewReader(run.Starts[len(run.Starts)-1].Events)
	callBody(t, m, "POST", "/api/v1/plugins/my-plugin/test-event", `{"event":"library.add"}`, http.StatusOK)
	got, err := readLine(t, r, 3*time.Second)
	if err != nil || got["event"] != "library.add" || got["payload"].(map[string]any)["test"] != true {
		t.Fatalf("test line %v %v", got, err)
	}
	callBody(t, m, "POST", "/api/v1/plugins/my-plugin/test-event", `{"event":"library.delete"}`, http.StatusNotFound)
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusOK)
	callBody(t, m, "POST", "/api/v1/plugins/tailscale/test-event", `{"event":"library.add"}`, http.StatusUnprocessableEntity)
	// A developer folder never replaces a signed install of the same id.
	devFolder(t, e, "tailscale", "9.9.9", []byte("unsigned swap"))
	call(t, m, "POST", "/api/v1/plugins/tailscale/dev-install", http.StatusUnprocessableEntity)
	if p := row(t, call(t, m, "GET", "/api/v1/plugins", http.StatusOK), "tailscale"); p["dev"] == true || p["installed_version"] != "1.0.0" {
		t.Fatalf("signed install replaced: %v", p)
	}
}

// publishTest lists version as a test version of tailscale, signed with
// kind, served with its own manifest and file.
func publishTest(ls *listServer, version string, file []byte, n uint64, kind string) {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	m, _ := json.Marshal(map[string]any{"id": "tailscale", "name": "Tailscale", "version": version, "author": "HolyArrs",
		"access":          map[string]any{"hosts": []string{}, "folders": []string{}, "actions": []string{}},
		"min_app_version": "1.0.0", "sha256": hashOf(file)})
	if ls.extra == nil {
		ls.extra = map[string][2][]byte{}
	}
	ls.extra[version] = [2][]byte{m, file}
	zero := uint64(0)
	ls.testList = ls.sign(plugins.List{Kind: kind, Counter: n, MinBlocklistCounter: &zero,
		Entries: []plugins.Entry{{ID: "tailscale", Version: version, SHA256: hashOf(file), ManifestSHA256: hashOf(m)}}})
}

// A test version is shown but installed only for an owner who asked for
// test versions of that plugin; it keeps running through the blocklist
// check; a test list signed as the plugin list is refused.
func TestTestChannel(t *testing.T) {
	ls, pub := newListServer(t)
	e, run := newEngine(t, ls, pub)
	m := mux(e)
	publishTest(ls, "1.1.0-test.1", []byte("test build"), 1, plugins.KindTest)
	p := row(t, call(t, m, "GET", "/api/v1/plugins", http.StatusOK), "tailscale")
	if p["test_version"] != "1.1.0-test.1" || p["version"] != "1.0.0" || p["test_on"] == true {
		t.Fatalf("row before opt-in: %v", p)
	}
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusOK)
	p = row(t, call(t, m, "GET", "/api/v1/plugins", http.StatusOK), "tailscale")
	if p["installed_version"] != "1.0.0" || p["test"] == true {
		t.Fatalf("install without opt-in: %v", p)
	}
	putOn(t, m, "/api/v1/plugins/tailscale/test-versions", true, http.StatusOK)
	p = row(t, call(t, m, "GET", "/api/v1/plugins", http.StatusOK), "tailscale")
	if p["version"] != "1.1.0-test.1" || p["test_on"] != true {
		t.Fatalf("row after opt-in: %v", p)
	}
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusOK)
	p = row(t, call(t, m, "GET", "/api/v1/plugins", http.StatusOK), "tailscale")
	if p["installed_version"] != "1.1.0-test.1" || p["test"] != true || p["running"] != true {
		t.Fatalf("test install: %v", p)
	}
	got, _ := os.ReadFile(run.Starts[len(run.Starts)-1].Program)
	if string(got) != "test build" {
		t.Fatalf("ran %q", got)
	}
	if err := e.CheckBlocklist(t.Context()); err != nil {
		t.Fatal(err)
	}
	p = row(t, call(t, m, "GET", "/api/v1/plugins", http.StatusOK), "tailscale")
	if p["running"] != true {
		t.Fatalf("test version stopped by the blocklist check: %v", p)
	}

	ls2, pub2 := newListServer(t)
	e2, _ := newEngine(t, ls2, pub2)
	m2 := mux(e2)
	publishTest(ls2, "1.1.0-test.1", []byte("test build"), 1, plugins.KindPlugins)
	putOn(t, m2, "/api/v1/plugins/tailscale/test-versions", true, http.StatusOK)
	p = row(t, call(t, m2, "GET", "/api/v1/plugins", http.StatusOK), "tailscale")
	if p["test_version"] != nil {
		t.Fatalf("a list signed as the plugin list was taken as test versions: %v", p)
	}
	call(t, m2, "POST", "/api/v1/plugins/tailscale/install", http.StatusOK)
	if p = row(t, call(t, m2, "GET", "/api/v1/plugins", http.StatusOK), "tailscale"); p["installed_version"] != "1.0.0" {
		t.Fatalf("installed %v", p)
	}
}
