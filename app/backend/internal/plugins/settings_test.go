package plugins_test

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/plugins"
)

// Settings: defaults show until saved, a save restarts the running plugin with
// the values as env, a secret is sealed on disk and never shown back, and
// Remove takes the settings with it.
func TestPluginSettings(t *testing.T) {
	ls, pub := newListServer(t)
	e, run := newEngine(t, ls, pub)
	e.SettingsKey = []byte("0123456789abcdef0123456789abcdef")
	m := mux(e)
	call(t, m, "GET", "/api/v1/plugins/tailscale/settings", http.StatusNotFound)
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusOK)

	v := call(t, m, "GET", "/api/v1/plugins/tailscale/settings", http.StatusOK)
	vals := v["values"].(map[string]any)
	if vals["port"] != "8090" || vals["read_only"] != "true" || vals["mode"] != "safe" || v["secrets_set"].(map[string]any)["pin"] != false {
		t.Fatalf("defaults: %v", v)
	}
	if _, ok := vals["pin"]; ok {
		t.Fatal("secret shown in values")
	}
	if env := run.Starts[0].Env; !slices.Contains(env, "HOLYARRS_PLUGIN_SETTING_PORT=8090") {
		t.Fatalf("first start env: %v", env)
	}

	v = callBody(t, m, "PUT", "/api/v1/plugins/tailscale/settings", `{"values":{"port":"9000","read_only":"false","mode":"fast","pin":"4321-abcd"}}`, http.StatusOK)
	if v["values"].(map[string]any)["port"] != "9000" || v["secrets_set"].(map[string]any)["pin"] != true {
		t.Fatalf("after save: %v", v)
	}
	if len(run.Starts) != 2 {
		t.Fatalf("save did not restart the plugin: %d starts", len(run.Starts))
	}
	env := run.Starts[1].Env
	for _, want := range []string{"HOLYARRS_PLUGIN_SETTING_PORT=9000", "HOLYARRS_PLUGIN_SETTING_READ_ONLY=false", "HOLYARRS_PLUGIN_SETTING_MODE=fast", "HOLYARRS_PLUGIN_SETTING_PIN=4321-abcd"} {
		if !slices.Contains(env, want) {
			t.Fatalf("env misses %s: %v", want, env)
		}
	}
	raw, _ := os.ReadFile(filepath.Join(e.Dir, "tailscale", "settings.json"))
	if strings.Contains(string(raw), "4321-abcd") || !strings.Contains(string(raw), "sealed:") {
		t.Fatalf("secret not sealed on disk: %s", raw)
	}

	// An empty secret keeps the saved one; Clear drops it.
	callBody(t, m, "PUT", "/api/v1/plugins/tailscale/settings", `{"values":{"pin":""}}`, http.StatusOK)
	if v := call(t, m, "GET", "/api/v1/plugins/tailscale/settings", http.StatusOK); v["secrets_set"].(map[string]any)["pin"] != true {
		t.Fatalf("empty secret cleared it: %v", v)
	}
	callBody(t, m, "PUT", "/api/v1/plugins/tailscale/settings", `{"clear":["pin"]}`, http.StatusOK)
	if v := call(t, m, "GET", "/api/v1/plugins/tailscale/settings", http.StatusOK); v["secrets_set"].(map[string]any)["pin"] != false {
		t.Fatalf("clear kept it: %v", v)
	}

	// Refusals: unknown field, wrong type, a choice not offered.
	for _, body := range []string{`{"values":{"nope":"1"}}`, `{"values":{"port":"abc"}}`, `{"values":{"read_only":"maybe"}}`, `{"values":{"mode":"slow"}}`, `{"clear":["port"]}`} {
		callBody(t, m, "PUT", "/api/v1/plugins/tailscale/settings", body, http.StatusUnprocessableEntity)
	}

	// Remove deletes the settings; a reinstall starts from the defaults.
	call(t, m, "DELETE", "/api/v1/plugins/tailscale", http.StatusOK)
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusOK)
	if v := call(t, m, "GET", "/api/v1/plugins/tailscale/settings", http.StatusOK); v["values"].(map[string]any)["port"] != "8090" {
		t.Fatalf("settings survived remove: %v", v)
	}
}

// Without a key a secret can't be saved, and nothing is written in the clear.
func TestPluginSettingsNoKey(t *testing.T) {
	ls, pub := newListServer(t)
	e, _ := newEngine(t, ls, pub)
	if _, err := e.Install(context.Background(), "tailscale"); err != nil {
		t.Fatal(err)
	}
	if err := e.SaveSettings("tailscale", plugins.SettingsInput{Values: map[string]string{"pin": "x"}}); err == nil {
		t.Fatal("secret saved without a key")
	}
	if _, err := os.Stat(filepath.Join(e.Dir, "tailscale", "settings.json")); !os.IsNotExist(err) {
		t.Fatalf("settings written: %v", err)
	}
}
