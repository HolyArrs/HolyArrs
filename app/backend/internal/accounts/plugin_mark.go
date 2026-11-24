package accounts

import (
	"crypto/rand"
	"crypto/subtle"
	"net"
	"net/http"
	"strings"
	"sync"
)

// Headers a forwarding plugin (Tailscale, or one granted network.forward)
// sets on visits it hands in. The plugin strips every visitor copy (all
// x-holyarrs- headers) first; HolyArrs still believes them only when the
// mark matches.
const (
	PluginPeerHeader = "X-Holyarrs-Tailscale-Addr" // the peer IP (WhoIs)
	PluginNameHeader = "X-Holyarrs-Tailscale-Name" // the plugin's own outside name
)

type markEntry struct {
	mark string
	name string // the outside name its first marked visit gave; later ones must match
}

var (
	markMu sync.Mutex
	marks  = map[string]*markEntry{} // plugin id -> its current start
)

// IssuePluginMark makes a fresh mark for one start of a forwarding plugin,
// handed to it as HOLYARRS_PLUGIN_MARK. It lives only in memory, is bound to
// that plugin and replaces the mark of its last start.
func IssuePluginMark(id string) string {
	m := rand.Text()
	markMu.Lock()
	marks[id] = &markEntry{mark: m}
	markMu.Unlock()
	return m
}

// RevokePluginMark ends id's mark (stop, block, remove).
func RevokePluginMark(id string) {
	markMu.Lock()
	delete(marks, id)
	markMu.Unlock()
}

// markOwner is the plugin whose current mark r carries, compared in
// constant time, or "".
func markOwner(r *http.Request) string {
	got := r.Header.Get(PluginProxyHeader)
	if got == "" {
		return ""
	}
	markMu.Lock()
	defer markMu.Unlock()
	owner := ""
	for id, m := range marks {
		if subtle.ConstantTimeCompare([]byte(got), []byte(m.mark)) == 1 {
			owner = id
		}
	}
	return owner
}

// pluginMarked says whether r carries a live plugin's mark. A marked visit
// is still a visit from outside: it never counts as local.
func pluginMarked(r *http.Request) bool { return markOwner(r) != "" }

// pluginPeer is the peer IP of a marked visit, or "".
func pluginPeer(r *http.Request) string {
	if !pluginMarked(r) {
		return ""
	}
	if ip := net.ParseIP(strings.TrimSpace(r.Header.Get(PluginPeerHeader))); ip != nil {
		return ip.String()
	}
	return ""
}

// pluginHost says whether r is a marked visit whose Host is the plugin's own
// outside name.
func pluginHost(r *http.Request) bool {
	name := normHost(r.Header.Get(PluginNameHeader))
	id := markOwner(r)
	if name == "" || id == "" || normHost(r.Host) != name {
		return false
	}
	markMu.Lock()
	defer markMu.Unlock()
	e := marks[id]
	if e == nil {
		return false
	}
	if e.name == "" {
		e.name = name // pinned for this start
	}
	return e.name == name
}

// setupRoute is every first-run route: public only while there is no owner.
func setupRoute(r *http.Request) bool {
	return r.URL.Path == "/api/v1/auth/setup" || strings.HasPrefix(r.URL.Path, "/api/v1/auth/setup/")
}
