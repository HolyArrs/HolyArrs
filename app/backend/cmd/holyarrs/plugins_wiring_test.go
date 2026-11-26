package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
)

func TestPluginEnvTailscale(t *testing.T) {
	for _, c := range []struct {
		id     string
		scopes []string
		fwd    bool
	}{
		{"tailscale", nil, false},
		{"tailscale", []string{"network.forward"}, true},
		{"netbird", []string{"network.forward"}, true},
		{"other", nil, false},
		{"other", []string{"library.read"}, false},
	} {
		got := pluginEnv(c.id, c.scopes, ":6262")
		again := pluginEnv(c.id, c.scopes, ":6262")
		t.Cleanup(func() { accounts.RevokePluginMark(c.id) })
		// Every plugin gets the address; only a forwarder gets a mark.
		if forwards(c.id, c.scopes) != c.fwd || (len(got) == 2) != c.fwd || got[0] != "HOLYARRS_PLUGIN_TARGET=http://127.0.0.1:6262" {
			t.Errorf("%s %v: env = %q", c.id, c.scopes, got)
			continue
		}
		if c.fwd && (got[0] != "HOLYARRS_PLUGIN_TARGET=http://127.0.0.1:6262" || !strings.HasPrefix(got[1], "HOLYARRS_PLUGIN_MARK=") || slices.Equal(got, again)) {
			t.Errorf("%s: env %q then %q, want a new mark per start", c.id, got, again)
		}
	}
}

func TestPluginTokenEnv(t *testing.T) {
	env := pluginTokenEnv("demo", []string{"library.read"}, ":6262")
	t.Cleanup(func() { accounts.RevokePluginToken("demo") })
	if len(env) != 2 || env[0] != "HOLYARRS_PLUGIN_TARGET=http://127.0.0.1:6262" || !strings.HasPrefix(env[1], "HOLYARRS_PLUGIN_TOKEN=hap_") {
		t.Errorf("env = %q", env)
	}
}
