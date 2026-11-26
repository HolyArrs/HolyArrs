//go:build !labkeys

package main

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/plugins"
)

func TestDefaultBuildIgnoresLabPluginKey(t *testing.T) {
	t.Setenv("HOLYARRS_LAB_PLUGIN_KEY", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	if got := labPluginKeys(); len(got) != 0 {
		t.Fatalf("default build trusted a lab key: %v", got)
	}
	e := newPluginEngine(config{ConfigDir: t.TempDir(), PluginListURL: defaultPluginListURL})
	pluginKeys := plugins.PinnedKeyStrings()
	if len(e.Keys) != len(pluginKeys) {
		t.Fatalf("keys = %d, want only the %d pinned keys", len(e.Keys), len(pluginKeys))
	}
}
