package accounts

import (
	"crypto/rand"
	"crypto/subtle"
	"net"
	"net/http"
	"strings"
	"sync"
)

// Headers the Tailscale plugin sets on visits it hands in. The plugin strips
// every visitor copy (all x-holyarrs- headers) first; HolyArrs still believes
// them only when the mark matches.
const (
	PluginPeerHeader = "X-Holyarrs-Tailscale-Addr" // the tailnet peer IP (WhoIs)
	PluginNameHeader = "X-Holyarrs-Tailscale-Name" // the plugin's own ts.net name
)

var (
	markOnce sync.Once
	markVal  string
)

// PluginMark is the random per-start token handed to the Tailscale plugin as
// HOLYARRS_PLUGIN_MARK. It lives only in memory.
func PluginMark() string {
	markOnce.Do(func() { markVal = rand.Text() })
	return markVal
}

// pluginMarked says whether r carries the current mark, compared in constant
// time. A marked visit is still a visit from outside: it never counts as local.
func pluginMarked(r *http.Request) bool {
	got := r.Header.Get(PluginProxyHeader)
	return got != "" && subtle.ConstantTimeCompare([]byte(got), []byte(PluginMark())) == 1
}

// pluginPeer is the tailnet peer IP of a marked visit, or "".
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
// ts.net name.
func pluginHost(r *http.Request) bool {
	name := normHost(r.Header.Get(PluginNameHeader))
	return name != "" && pluginMarked(r) && normHost(r.Host) == name
}

// setupRoute is every first-run route: public only while there is no owner.
func setupRoute(r *http.Request) bool {
	return r.URL.Path == "/api/v1/auth/setup" || strings.HasPrefix(r.URL.Path, "/api/v1/auth/setup/")
}
