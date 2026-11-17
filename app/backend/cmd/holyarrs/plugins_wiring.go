package main

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/plugins"
)

// defaultPluginListURL serves the signed plugin list, blocklist and files.
const defaultPluginListURL = "https://plugins.holyarrs.com/"

// pluginKeys are the pinned ed25519 public keys (base64) the plugin lists
// must be signed with: the main key, then the backup key kept offline.
var pluginKeys = []string{
	"BFcI/1RFf6k7cRqXaj+YYQsfZu1MQWmiFYktV2QIBHE=", // main
	"ENa3RVFV9VfQgZ/uyaKItB9xmniR36sWM17HWXi+llY=", // backup
}

// startPlugins builds the Engine and, outside safe mode, starts the
// enabled plugins and the blocklist check; they stop when ctx ends.
func startPlugins(ctx context.Context, c config) *plugins.Engine {
	pe := newPluginEngine(c)
	pe.Env = func(id string) []string { return pluginEnv(id, c.Addr) }
	if !c.SafeMode {
		pe.StartEnabled()
		go func() { pe.Run(ctx); pe.StopAll() }()
	}
	return pe
}

// pluginEnv hands the Tailscale plugin HolyArrs' loopback address and the
// per-start mark it must set on every visit (accounts.PluginMark).
func pluginEnv(id, addr string) []string {
	if id != "tailscale" {
		return nil
	}
	_, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		port = defaultPort
	}
	return []string{"HOLYARRS_PLUGIN_TARGET=http://127.0.0.1:" + port, "HOLYARRS_PLUGIN_MARK=" + accounts.PluginMark()}
}

// newPluginEngine builds the Plugin Engine. Off Linux it has no runner, so
// installs and starts refuse with "plugins run only on Linux".
func newPluginEngine(c config) *plugins.Engine {
	run, _, err := plugins.DefaultRunner()
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
	return &plugins.Engine{Dir: filepath.Join(c.ConfigDir, "plugins"), ListURL: c.PluginListURL, Keys: keys, Runner: run,
		HTTP: &http.Client{Timeout: 5 * time.Minute, CheckRedirect: plugins.RedirectGuard(c.PluginListURL)}}
}
