//go:build labkeys

package main

import (
	"os"
	"strings"
)

// labPluginKeys adds one extra trusted plugin-list key from
// HOLYARRS_LAB_PLUGIN_KEY (base64 ed25519). Only lab builds made with
// -tags labkeys contain this; normal and release builds use the stub.
func labPluginKeys() []string {
	if k := strings.TrimSpace(os.Getenv("HOLYARRS_LAB_PLUGIN_KEY")); k != "" {
		return []string{k}
	}
	return nil
}
