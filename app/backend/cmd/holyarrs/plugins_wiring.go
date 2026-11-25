package main

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"github.com/HolyArrs/holyarrs/app/backend/internal/plugins/hooks"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"slices"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/alerts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/plugins"
)

// defaultPluginListURL serves the signed plugin list, blocklist and files.
const defaultPluginListURL = "https://plugins.holyarrs.com/"

// startPlugins builds the Engine and, outside safe mode, starts the
// enabled plugins and the blocklist check; they stop when ctx ends.
func startPlugins(ctx context.Context, c config, key []byte, appVersion string, reg *hooks.Registry, sender *alerts.Sender) *plugins.Engine {
	pe := newPluginEngine(c)
	if sender != nil {
		// A plugin that was stopped catches up on what it missed.
		pe.Replay = func(ctx context.Context, events []string, after int64, limit int) []plugins.Replayed {
			var out []plugins.Replayed
			for _, r := range sender.PluginReplay(ctx, events, after, limit) {
				out = append(out, plugins.Replayed{Event: r.Event, ID: r.ID, Payload: r.Payload})
			}
			return out
		}
		pe.LastEventID = sender.Store.LastEventID
	}
	pe.SettingsKey = key
	pe.AppVersion = appVersion
	pe.Env = func(id string) []string { return pluginEnv(id, pe.GrantedScopes(id), c.Addr) }
	pe.Token = func(id string, scopes []string) []string { return pluginTokenEnv(id, scopes, c.Addr) }
	pe.Revoke = func(id string) {
		accounts.RevokePluginToken(id)
		accounts.RevokePluginMark(id)
		if reg != nil {
			reg.Drop(id) // its waiting work, tools, widgets, pages and streams end too
		}
	}
	pe.Forwards = func(id string) bool { return forwards(id, pe.GrantedScopes(id)) }
	if !c.SafeMode {
		pe.StartEnabled()
		go pe.Run(ctx) // run stops the plugins as it returns
	}
	return pe
}

// newLanguages keeps the language packs in <config>/languages, fetched from
// the plugin list address and checked with the same pinned keys.
func newLanguages(c config, pe *plugins.Engine, upgraded func() bool) *plugins.Languages {
	return &plugins.Languages{Dir: filepath.Join(c.ConfigDir, "languages"), Engine: pe, Upgraded: upgraded}
}

// languageWait is how long the first start of a new version waits for the
// language packs before the app opens.
const languageWait = 10 * time.Second

// keepLanguages brings the packs up to date for this version: on the first
// start of a new version it waits up to languageWait, then keeps trying in
// the background.
func keepLanguages(ctx context.Context, l *plugins.Languages, accountLangs func() []string) {
	if l == nil {
		return
	}
	if !l.NewVersion() {
		go l.Keep(ctx, accountLangs)
		return
	}
	wctx, cancel := context.WithTimeout(ctx, languageWait)
	if err := l.Ensure(wctx, accountLangs()); err != nil {
		log.Printf("languages: not up to date for this version yet, showing English until they are: %v", err)
	}
	cancel()
	go l.Keep(ctx, accountLangs)
}

// pluginEnv hands a forwarding plugin (Tailscale, or one granted
// network.forward) HolyArrs' loopback address and the per-start mark it must
// set on every visit (accounts.PluginMark).
// Each call is a start, so it rotates that plugin's mark.
// A plugin without network.forward still gets the address, so one that
// expects it starts; with no mark, nothing it hands in is believed.
func pluginEnv(id string, scopes []string, addr string) []string {
	if !forwards(id, scopes) {
		return []string{pluginTarget(addr)}
	}
	return []string{pluginTarget(addr), "HOLYARRS_PLUGIN_MARK=" + accounts.IssuePluginMark(id)}
}

// forwards says whether id hands visits in from outside: only a plugin the
// owner granted network.forward, Tailscale included.
func forwards(id string, scopes []string) bool {
	return slices.Contains(scopes, "network.forward")
}

// pluginTokenEnv gives a start its fresh API token and the app's loopback
// address, the only loopback port its network lock then lets it reach.
func pluginTokenEnv(id string, scopes []string, addr string) []string {
	return []string{pluginTarget(addr), "HOLYARRS_PLUGIN_TOKEN=" + accounts.IssuePluginToken(id, scopes)}
}

func pluginTarget(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		port = defaultPort
	}
	return "HOLYARRS_PLUGIN_TARGET=http://127.0.0.1:" + port
}

// newPluginEngine builds the Plugin Engine. Off Linux it has no runner, so
// installs and starts refuse with "plugins run only on Linux".
func newPluginEngine(c config) *plugins.Engine {
	run, limits, err := plugins.DefaultRunner()
	if err != nil {
		run = nil
	}
	noLock := run != nil && !limits.FileLock
	noNet := run != nil && !limits.NetLock
	if noNet {
		log.Printf("plugins: this system cannot keep plugins to the websites they list (network lock: %s); plugins that list websites stay stopped unless the owner allows them on the Plugins page", limits.NetLockReason)
	}
	if noLock {
		log.Printf("plugins: this system cannot give plugins the file lock (Landlock: %s); plugins stay stopped unless the owner allows each one on the Plugins page", limits.FileLockReason)
	}
	keys := plugins.PinnedKeys()
	for _, k := range labPluginKeys() {
		b, err := base64.StdEncoding.DecodeString(k)
		if err != nil || len(b) != ed25519.PublicKeySize {
			log.Printf("plugins: pinned key %q is not an ed25519 public key; skipped", k)
			continue
		}
		keys = append(keys, ed25519.PublicKey(b))
	}
	return &plugins.Engine{Dir: filepath.Join(c.ConfigDir, "plugins"), ListURL: c.PluginListURL, Keys: keys, Runner: run, NoFileLock: noLock, FileLockReason: limits.FileLockReason,
		NoNetLock: noNet, NetLockReason: limits.NetLockReason,
		HTTP: &http.Client{Timeout: 5 * time.Minute, CheckRedirect: plugins.RedirectGuard(c.PluginListURL)}}
}

// tellPlugins hands a Library message to the plugins that asked for its
// event, as the webhook payload without the people who asked.
func tellPlugins(pe *plugins.Engine, m alerts.Message) {
	if pe == nil {
		return
	}
	if ev := alerts.PluginMomentEvents[m.Moment]; ev != "" {
		var id int64
		if m.Event != nil {
			id = m.Event.ID
		}
		pe.DeliverEvent(ev, id, alerts.PluginPayload(m))
	}
}
