package plugins_test

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/plugins"
)

// An update keeps a plugin the owner stopped stopped; one turned off for
// another reason gets started by the update.
func TestUpdateKeepsOwnerStoppedPluginStopped(t *testing.T) {
	ls, pub := newListServer(t)
	e, run := newEngine(t, ls, pub)
	m := mux(e)
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusOK)
	call(t, m, "POST", "/api/v1/plugins/tailscale/stop", http.StatusOK)
	starts := len(run.Starts)

	ls.publish("1.0.1", []byte("plugin program v2"), 2)
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusOK)
	p := row(t, call(t, m, "GET", "/api/v1/plugins", http.StatusOK), "tailscale")
	if p["installed_version"] != "1.0.1" || p["enabled"] != false || p["running"] != false || p["disabled_reason"] != "stopped by the owner" {
		t.Fatalf("stopped plugin after update: %v", p)
	}
	if len(run.Starts) != starts {
		t.Fatalf("update started a stopped plugin: %d starts, want %d", len(run.Starts), starts)
	}

	// Start works on the new version.
	call(t, m, "POST", "/api/v1/plugins/tailscale/start", http.StatusOK)
	if p := row(t, call(t, m, "GET", "/api/v1/plugins", http.StatusOK), "tailscale"); p["running"] != true {
		t.Fatalf("start after update: %v", p)
	}

	// A plugin turned off by a failed start is started by the next update.
	call(t, m, "POST", "/api/v1/plugins/tailscale/stop", http.StatusOK)
	run.Err = errors.New("sandbox refused")
	_ = e.Start(context.Background(), "tailscale")
	run.Err = nil
	ls.publish("1.0.2", []byte("plugin program v3"), 3)
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusOK)
	if p := row(t, call(t, m, "GET", "/api/v1/plugins", http.StatusOK), "tailscale"); p["running"] != true || p["enabled"] != true {
		t.Fatalf("update after a failed start: %v", p)
	}
}

// A plugin that ends on its own is turned off with the reason (shown on the
// page, Start offered) and not started again by itself.
func TestPluginThatExitsOnItsOwnShowsWhy(t *testing.T) {
	ls, pub := newListServer(t)
	e, run := newEngine(t, ls, pub)
	m := mux(e)
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusOK)
	starts := len(run.Starts)
	run.Exit(errors.New("exit status 1"))

	var p map[string]any
	for i := 0; i < 200; i++ {
		p = row(t, call(t, m, "GET", "/api/v1/plugins", http.StatusOK), "tailscale")
		if p["enabled"] == false {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if p["running"] != false || p["enabled"] != false || !strings.Contains(p["disabled_reason"].(string), "stopped on its own (exit status 1)") {
		t.Fatalf("after exit: %v", p)
	}
	if len(run.Starts) != starts {
		t.Fatal("crashed plugin restarted by itself")
	}
	call(t, m, "POST", "/api/v1/plugins/tailscale/start", http.StatusOK)
	if p := row(t, call(t, m, "GET", "/api/v1/plugins", http.StatusOK), "tailscale"); p["running"] != true {
		t.Fatalf("start after exit: %v", p)
	}

	// Stop by the owner is not mistaken for a crash.
	call(t, m, "POST", "/api/v1/plugins/tailscale/stop", http.StatusOK)
	time.Sleep(50 * time.Millisecond)
	if p := row(t, call(t, m, "GET", "/api/v1/plugins", http.StatusOK), "tailscale"); p["disabled_reason"] != "stopped by the owner" {
		t.Fatalf("owner stop: %v", p)
	}
}

// A blocklist check whose cache write fails keeps the stored counters, so
// the older cached list still starts the plugin (no plugin is turned off).
func TestFailedCacheWriteKeepsPluginsStartable(t *testing.T) {
	ls, pub := newListServer(t)
	e, _ := newEngine(t, ls, pub)
	m := mux(e)
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusOK)
	caches, _ := filepath.Glob(filepath.Join(e.Dir, "list*.json"))
	if len(caches) != 1 {
		t.Fatalf("cached list: %v", caches)
	}
	old, err := os.ReadFile(caches[0])
	if err != nil {
		t.Fatal(err)
	}
	// A folder where the cache file goes makes the write fail.
	os.Remove(caches[0])
	if err := os.Mkdir(caches[0], 0o700); err != nil {
		t.Fatal(err)
	}
	ls.mu.Lock()
	ls.block = ls.sign(plugins.List{Kind: plugins.KindBlocklist, Counter: 2, Entries: []plugins.Entry{}})
	ls.mu.Unlock()
	ls.publish("1.0.0", pluginFile, 5)
	if err := e.CheckBlocklist(context.Background()); err == nil {
		t.Fatal("failed cache write not reported")
	}
	os.Remove(caches[0])
	if err := os.WriteFile(caches[0], old, 0o600); err != nil {
		t.Fatal(err)
	}
	call(t, m, "POST", "/api/v1/plugins/tailscale/stop", http.StatusOK)
	call(t, m, "POST", "/api/v1/plugins/tailscale/start", http.StatusOK)
	if p := row(t, call(t, m, "GET", "/api/v1/plugins", http.StatusOK), "tailscale"); p["running"] != true {
		t.Fatalf("plugin turned off after a failed cache write: %v", p)
	}
}

// A toggle or select with no default reads as off / the first choice and
// never blocks a save; a bad number is refused.
func TestSettingsWithoutDefaults(t *testing.T) {
	ls, pub := newListServer(t)
	ls.publishWith("1.0.0", pluginFile, 1, func(m map[string]any) {
		m["settings"] = []map[string]any{
			{"key": "on", "label": "On", "type": "toggle"},
			{"key": "pick", "label": "Pick", "type": "select", "options": []string{"a", "b"}},
			{"key": "name", "label": "Name", "type": "text"},
			{"key": "n", "label": "N", "type": "number"},
		}
	})
	e, run := newEngine(t, ls, pub)
	m := mux(e)
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusOK)
	v := call(t, m, "GET", "/api/v1/plugins/tailscale/settings", http.StatusOK)
	if vals := v["values"].(map[string]any); vals["on"] != "false" || vals["pick"] != "a" {
		t.Fatalf("defaults: %v", v)
	}
	// Empty values for untouched fields save fine.
	v = callBody(t, m, "PUT", "/api/v1/plugins/tailscale/settings", `{"values":{"on":"","pick":"","name":"hi","n":""}}`, http.StatusOK)
	if vals := v["values"].(map[string]any); vals["name"] != "hi" || vals["on"] != "false" || vals["pick"] != "a" {
		t.Fatalf("after save: %v", v)
	}
	env := strings.Join(run.Starts[len(run.Starts)-1].Env, " ")
	if !strings.Contains(env, "HOLYARRS_PLUGIN_SETTING_ON=false") || !strings.Contains(env, "HOLYARRS_PLUGIN_SETTING_PICK=a") {
		t.Fatalf("env: %s", env)
	}
	for _, n := range []string{"NaN", "Inf", "-Inf", "1e999"} {
		callBody(t, m, "PUT", "/api/v1/plugins/tailscale/settings", `{"values":{"n":"`+n+`"}}`, http.StatusUnprocessableEntity)
	}
	callBody(t, m, "PUT", "/api/v1/plugins/tailscale/settings", `{"values":{"n":"12.5"}}`, http.StatusOK)
}

// A save that is stored but whose restart fails answers with the saved
// values and the start error, not as a failed save.
func TestSettingsSavedButRestartFails(t *testing.T) {
	ls, pub := newListServer(t)
	e, run := newEngine(t, ls, pub)
	m := mux(e)
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusOK)
	run.Err = errors.New("sandbox refused")
	v := callBody(t, m, "PUT", "/api/v1/plugins/tailscale/settings", `{"values":{"port":"9100"}}`, http.StatusOK)
	if v["values"].(map[string]any)["port"] != "9100" || !strings.Contains(v["start_error"].(string), "sandbox refused") {
		t.Fatalf("save with failed restart: %v", v)
	}
	if p := row(t, call(t, m, "GET", "/api/v1/plugins", http.StatusOK), "tailscale"); p["running"] != false || p["enabled"] != false {
		t.Fatalf("plugin after failed restart: %v", p)
	}
}
