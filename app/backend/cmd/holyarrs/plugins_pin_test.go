package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/plugins"
)

// The pinned key must be the one the publish job checks lists with.
func TestPluginKeyMatchesPublishKey(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "deploy", "release", "plugins", "keys", "main.pub"))
	if err != nil {
		t.Skip("deploy/release/plugins not in this checkout")
	}
	pluginKeys := plugins.PinnedKeyStrings()
	if len(pluginKeys) != 1 || pluginKeys[0] != strings.TrimSpace(string(b)) {
		t.Fatalf("pinned keys %v do not match keys/main.pub", pluginKeys)
	}
}
