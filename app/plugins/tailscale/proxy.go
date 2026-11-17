package main

import (
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

// MarkHeader carries the per-start secret on every visit handed to HolyArrs.
// HolyArrs knows it as accounts.PluginProxyHeader and always asks for sign-in
// when it is there.
const MarkHeader = "X-Holyarrs-Plugin-Proxy"

// Identity headers: information for HolyArrs' screens only, never sign-in.
const (
	UserHeader   = "X-Holyarrs-Tailscale-User"
	DeviceHeader = "X-Holyarrs-Tailscale-Device"
)

// Headers HolyArrs believes only with the mark: the tailnet peer IP (for
// per-visitor sign-in limits) and this device's ts.net name (so the
// visitor's Host passes HolyArrs' host check). Neither makes a visit local.
const (
	AddrHeader = "X-Holyarrs-Tailscale-Addr"
	NameHeader = "X-Holyarrs-Tailscale-Name"
)

// stripPrefixes are headers a visitor could write to pose as a proxy, a
// signed-in person or HolyArrs itself. All are removed before the visit
// goes on.
var stripPrefixes = []string{"x-forwarded-", "x-holyarrs-", "remote-", "tailscale-", "x-original-", "x-auth-request-", "x-authentik-"}

var stripExact = map[string]bool{
	"forwarded": true, "x-real-ip": true, "x-client-ip": true, "true-client-ip": true,
	"cf-connecting-ip": true, "x-cluster-client-ip": true, "client-ip": true,
}

// stripHeaders removes every forwarding and identity header.
func stripHeaders(h http.Header) {
	for k := range h {
		l := strings.ToLower(k)
		if stripExact[l] {
			delete(h, k)
			continue
		}
		for _, p := range stripPrefixes {
			if strings.HasPrefix(l, p) {
				delete(h, k)
				break
			}
		}
	}
}

// whoFunc looks up the tailnet person and device behind a peer address. It
// may return empty strings.
type whoFunc func(r *http.Request) (user, device string)

// newProxy hands visits to HolyArrs at target with the headers cleaned and
// the mark set. The visitor's Host is kept; name gives this device's ts.net
// name and may be nil.
func newProxy(target *url.URL, mark string, who whoFunc, name func() string) http.Handler {
	return &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			// pr.Out is already free of hop-by-hop headers; X-Forwarded-For
			// is only added by SetXForwarded, which is never called.
			stripHeaders(pr.Out.Header)
			pr.SetURL(target)
			pr.Out.Host = pr.In.Host
			pr.Out.Header.Set(MarkHeader, mark)
			if h, _, err := net.SplitHostPort(pr.In.RemoteAddr); err == nil && net.ParseIP(h) != nil {
				pr.Out.Header.Set(AddrHeader, h)
			}
			if name != nil {
				if n := name(); n != "" {
					pr.Out.Header.Set(NameHeader, n)
				}
			}
			if who != nil {
				if u, d := who(pr.In); u != "" || d != "" {
					pr.Out.Header.Set(UserHeader, u)
					pr.Out.Header.Set(DeviceHeader, d)
				}
			}
		},
	}
}
