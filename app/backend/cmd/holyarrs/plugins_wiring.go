package main

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/plugins"
)

// defaultPluginListURL serves the signed plugin list, blocklist and files.
const defaultPluginListURL = "https://plugins.holyarrs.com/"

// pluginKeys are the pinned ed25519 public keys (base64) the plugin lists
// must be signed with. Empty until the real keys are made: every install
// then refuses with "plugin signing key not set yet".
var pluginKeys = []string{}

// startPlugins builds the Engine and, outside safe mode, starts the
// enabled plugins and the blocklist check; they stop when ctx ends.
func startPlugins(ctx context.Context, c config) *plugins.Engine {
	pe := newPluginEngine(c)
	if !c.SafeMode {
		pe.StartEnabled()
		go func() { pe.Run(ctx); pe.StopAll() }()
	}
	return pe
}

// newPluginEngine builds the Plugin Engine. Off Linux it has no runner, so
// installs and starts refuse with "plugins run only on Linux".
func newPluginEngine(c config) *plugins.Engine {
	run, limits, err := plugins.DefaultRunner()
	if err != nil {
		run = nil
	}
	var keys []ed25519.PublicKey
	for _, k := range pluginKeys {
		b, err := base64.StdEncoding.DecodeString(k)
		if err != nil || len(b) != ed25519.PublicKeySize {
			log.Printf("plugins: pinned key %q is not an ed25519 public key; skipped", k)
			continue
		}
		keys = append(keys, ed25519.PublicKey(b))
	}
	return &plugins.Engine{Dir: filepath.Join(c.ConfigDir, "plugins"), ListURL: c.PluginListURL, Keys: keys, Runner: run, Limits: limits,
		HTTP: &http.Client{Timeout: 5 * time.Minute, CheckRedirect: plugins.RedirectGuard(c.PluginListURL)}}
}
