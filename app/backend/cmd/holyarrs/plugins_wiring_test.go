package main

import (
	"slices"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
)

func TestPluginEnvTailscale(t *testing.T) {
	got := pluginEnv("tailscale", ":6262")
	want := []string{"HOLYARRS_PLUGIN_TARGET=http://127.0.0.1:6262", "HOLYARRS_PLUGIN_MARK=" + accounts.PluginMark()}
	if !slices.Equal(got, want) {
		t.Errorf("env = %q", got)
	}
	if pluginEnv("other", ":6262") != nil {
		t.Error("other plugins get no mark")
	}
}
