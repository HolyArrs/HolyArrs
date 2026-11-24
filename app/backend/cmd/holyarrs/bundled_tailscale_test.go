package main

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/plugins"
)

// The bundled Tailscale manifest asks for network.forward, parses as a
// signed manifest would, and a start of it gets the app address and a mark.
func TestBundledTailscaleForwards(t *testing.T) {
	raw, err := os.ReadFile("../../../plugins/tailscale/holyarrs-plugin.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	doc["sha256"] = strings.Repeat("a", 64)
	signed, _ := json.Marshal(doc)
	m, err := plugins.ParseManifest(signed)
	if err != nil {
		t.Fatalf("bundled manifest refused: %v", err)
	}
	if !slices.Equal(m.Access.Actions, []string{"network.forward"}) {
		t.Fatalf("actions %v", m.Access.Actions)
	}
	env := pluginEnv(m.ID, m.Access.Actions, ":6262")
	t.Cleanup(func() { accounts.RevokePluginMark(m.ID) })
	if len(env) != 2 || env[0] != "HOLYARRS_PLUGIN_TARGET=http://127.0.0.1:6262" || !strings.HasPrefix(env[1], "HOLYARRS_PLUGIN_MARK=") {
		t.Fatalf("env %v", env)
	}
	if forwards(m.ID, nil) {
		t.Error("forwarding without the scope")
	}
}
