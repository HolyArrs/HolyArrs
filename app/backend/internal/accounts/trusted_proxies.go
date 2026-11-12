package accounts

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"slices"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// keyTrustedProxies holds the trusted proxies saved in Settings, as a JSON
// list. They are added to the ones from HOLYARRS_TRUSTED_PROXIES, which stay
// and cannot be removed in Settings.
const keyTrustedProxies = "trusted_proxies"

// maxTrustedProxies caps the saved list.
const maxTrustedProxies = 32

// proxyList is a list of proxies as shown and as parsed.
type proxyList struct {
	list []string
	nets []*net.IPNet
}

// tooWide says whether a network is wider than any home network or proxy
// group: shorter than /8 for IPv4, /7 for IPv6.
func tooWide(n *net.IPNet) bool {
	ones, bits := n.Mask.Size()
	if bits == 32 {
		return ones < 8
	}
	return ones < 7
}

// parseProxyList checks the proxies typed in Settings and names the first
// entry it cannot use.
func parseProxyList(in []string) (*proxyList, error) {
	out := &proxyList{list: []string{}}
	for _, e := range in {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		n, err := parseNet(e)
		if err != nil {
			return nil, fmt.Errorf("%q is not a proxy address: use one like 172.18.0.2 or a range like 172.18.0.0/16", truncate(e, 60))
		}
		if tooWide(n) {
			return nil, fmt.Errorf("%q is too wide: list only the addresses your proxy uses", truncate(e, 60))
		}
		if s := n.String(); !slices.Contains(out.list, s) {
			out.list = append(out.list, s)
			out.nets = append(out.nets, n)
		}
	}
	if len(out.list) > maxTrustedProxies {
		return nil, fmt.Errorf("list at most %d proxies", maxTrustedProxies)
	}
	return out, nil
}

// loadSavedProxies reads the saved list; entries it cannot use are skipped.
func (a *Auth) loadSavedProxies(ctx context.Context) {
	out := &proxyList{list: []string{}}
	var in []string
	if v := a.signInSetting(ctx, keyTrustedProxies); v != "" {
		if err := json.Unmarshal([]byte(v), &in); err != nil {
			log.Printf("auth: ignoring saved trusted proxies: %v", err)
		}
	}
	for _, e := range in {
		if n, err := parseNet(strings.TrimSpace(e)); err == nil && !tooWide(n) && len(out.list) < maxTrustedProxies {
			out.list = append(out.list, n.String())
			out.nets = append(out.nets, n)
		} else {
			log.Printf("auth: ignoring trusted proxy %q", e)
		}
	}
	a.savedProxies.Store(out)
}

// fixedProxyList is the proxies from the start-up options and
// HOLYARRS_TRUSTED_PROXIES, for the locked rows in Settings.
func (a *Auth) fixedProxyList() []string {
	out := []string{}
	for _, n := range a.proxies {
		out = append(out, n.String())
	}
	return out
}

// anyProxy says whether any trusted proxy is set, fixed or saved.
func (a *Auth) anyProxy() bool {
	if len(a.proxies) > 0 {
		return true
	}
	p := a.savedProxies.Load()
	return p != nil && len(p.nets) > 0
}

// checkProxyChange checks a new saved list. It is changed only in the browser
// with a password sign-in, like Allowed host names, and never so that the
// proxy this browser comes through stops being trusted: its forwarded
// address and https would no longer count, and every save would be refused.
func (a *Auth) checkProxyChange(r *http.Request, in []string) (*proxyList, int, string) {
	if who, ok := webserver.From(r.Context()); ok && who.Via != "session" {
		return nil, http.StatusForbidden, "change trusted proxies in the browser, signed in with your password"
	}
	l, err := parseProxyList(in)
	if err != nil {
		return nil, http.StatusBadRequest, err.Error()
	}
	peer := parseHostIP(r.RemoteAddr)
	if peer == nil || !anyForwardHeader(r) || !a.trustedProxy(peer) {
		return l, 0, ""
	}
	for _, n := range a.proxies {
		if n.Contains(peer) {
			return l, 0, ""
		}
	}
	for _, n := range l.nets {
		if n.Contains(peer) {
			return l, 0, ""
		}
	}
	return nil, http.StatusConflict, "you reach HolyArrs through the proxy at " + peer.String() + "; keep it in the list"
}
