package accounts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"slices"
	"strings"
	"time"
)

// keyLocalNets holds the networks where sign-in may be skipped, as a JSON
// list of CIDRs.
const keyLocalNets = "local_networks"

// maxLocalNets caps the networks the owner may list.
const maxLocalNets = 32

// containerNets are networks Docker, Podman and Docker Desktop hand out by
// default. Behind them every browser can show up as one gateway address.
var containerNets = func() []*net.IPNet {
	var out []*net.IPNet
	for _, s := range []string{"172.17.0.0/16", "10.88.0.0/16", "10.0.2.0/24", "192.168.65.0/24"} {
		_, n, _ := net.ParseCIDR(s)
		out = append(out, n)
	}
	return out
}()

// localNets is the owner's list as typed (cleaned up) and as parsed.
type localNets struct {
	list []string
	nets []*net.IPNet
}

// parseNet reads one CIDR or single address; an address is its own network.
func parseNet(e string) (*net.IPNet, error) {
	if !strings.Contains(e, "/") {
		ip := net.ParseIP(strings.Trim(e, "[]"))
		if ip == nil {
			return nil, errors.New("not an address")
		}
		if v4 := ip.To4(); v4 != nil {
			return &net.IPNet{IP: v4, Mask: net.CIDRMask(32, 32)}, nil
		}
		return &net.IPNet{IP: ip, Mask: net.CIDRMask(128, 128)}, nil
	}
	_, n, err := net.ParseCIDR(e)
	if err != nil {
		return nil, err
	}
	if ones, _ := n.Mask.Size(); ones == 0 {
		return nil, errors.New("that is every address")
	}
	return n, nil
}

// parseLocalNets cleans up the owner's list: spaces trimmed, empty entries
// and repeats dropped. It names the first entry it cannot read.
func parseLocalNets(in []string) (*localNets, error) {
	out := &localNets{list: []string{}}
	seen := map[string]bool{}
	for _, e := range in {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		n, err := parseNet(e)
		if err != nil {
			return nil, fmt.Errorf("%q is not a network: use one like 192.168.1.0/24 or a single address like 192.168.1.5", truncate(e, 60))
		}
		if tooWide(n) {
			return nil, fmt.Errorf("%q is too wide: list your home network, like 192.168.1.0/24", truncate(e, 60))
		}
		if s := n.String(); !seen[s] {
			seen[s] = true
			out.list = append(out.list, s)
			out.nets = append(out.nets, n)
		}
	}
	if len(out.list) > maxLocalNets {
		return nil, fmt.Errorf("list at most %d networks", maxLocalNets)
	}
	return out, nil
}

// loadLocalNets reads the saved list. Entries it cannot read are skipped,
// so a damaged value trusts less, never more.
func (a *Auth) loadLocalNets(ctx context.Context) *localNets {
	v := a.signInSetting(ctx, keyLocalNets)
	var in []string
	if err := json.Unmarshal([]byte(v), &in); err != nil {
		in = strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == '\n' || r == ' ' })
	}
	out := &localNets{list: []string{}}
	for _, e := range in {
		if n, err := parseNet(strings.TrimSpace(e)); err == nil && !tooWide(n) && len(out.list) < maxLocalNets {
			out.list = append(out.list, n.String())
			out.nets = append(out.nets, n)
		} else if strings.TrimSpace(e) != "" {
			log.Printf("auth: ignoring local network %q", e)
		}
	}
	return out
}

// isLocal reports whether the peer address is inside a network the owner
// listed for skipping sign-in.
func (a *Auth) isLocal(remote string) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	l := a.localNets.Load()
	if ip == nil || l == nil {
		return false
	}
	for _, n := range l.nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// homeRanges are what "Add usual home ranges" adds. Loopback (127.0.0.0/8)
// and link-local (169.254.0.0/16, fe80::/10) are never in it, and the
// Tailscale and carrier range (cgnatRange) only when asked for.
var homeRanges = []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "fc00::/7"}

const cgnatRange = "100.64.0.0/10"

// withHomeRanges adds the usual home ranges when home, and cgnatRange when
// cgnat, to list after the entries already there.
func withHomeRanges(list []string, home, cgnat bool) []string {
	out := append([]string{}, list...)
	var add []string
	if home {
		add = append(add, homeRanges...)
	}
	if cgnat {
		add = append(add, cgnatRange)
	}
	for _, r := range add {
		if !slices.Contains(out, r) {
			out = append(out, r)
		}
	}
	return out
}

// gatewayLike says whether an address looks like a container gateway:
// 172.x.0.1, 10.88.0.1 or 10.89.x.1 bridges, 10.0.2.2 (rootless) or Docker Desktop's
// 192.168.65.x.
func gatewayLike(ip net.IP) bool {
	v4 := ip.To4()
	if v4 == nil {
		return false
	}
	switch {
	case v4[0] == 192 && v4[1] == 168 && v4[2] == 65:
		return true
	case v4[0] == 10 && v4[1] == 0 && v4[2] == 2 && v4[3] == 2:
		return true
	case (v4[0] == 172 && v4[1]&0xf0 == 16 || v4[0] == 10 && v4[1] == 88) && v4[2] == 0 && v4[3] == 1:
		return true
	case v4[0] == 10 && v4[1] == 89 && v4[3] == 1: // Podman user networks
		return true
	}
	return false
}

// gatewayNets lists the entries that look like a container network, or
// that hold self (the address this browser shows up as) when self looks
// like a gateway. It only feeds a warning; nothing is refused.
func gatewayNets(list []string, self string) []string {
	out := []string{}
	me := net.ParseIP(strings.Trim(self, "[]"))
	for _, e := range list {
		n, err := parseNet(e)
		if err != nil {
			continue
		}
		ones, bits := n.Mask.Size()
		hit := me != nil && gatewayLike(me) && n.Contains(me)
		// A /16 or narrower entry that holds its own .1 gateway, like a
		// compose network 172.18.0.0/16 or the address 172.18.0.1.
		if v4 := n.IP.To4(); v4 != nil && ones-(bits-32) >= 16 {
			gw := net.IPv4(v4[0], v4[1], v4[2], 1)
			if gatewayLike(gw) && n.Contains(gw) {
				hit = true
			}
		}
		for _, c := range containerNets {
			if cOnes, cBits := c.Mask.Size(); cBits == bits && ones >= cOnes && c.Contains(n.IP) {
				hit = true
			}
		}
		if hit {
			out = append(out, e)
		}
	}
	return out
}

// seenAddress is the address the local-network check reads for this
// request: the forwarded client behind a trusted proxy, otherwise the peer.
// When the forwarded address cannot be trusted (see clientAddr) the skip
// never applies, so it returns "" and untrusted true.
func (a *Auth) seenAddress(r *http.Request) (addr string, untrusted bool) {
	addr, ok := a.clientAddr(r)
	if !ok {
		return "", true
	}
	return addr, false
}

// clientAddr is the one place that decides who the caller is. It is used for
// the local-network skip and for every sign-in limit key.
//
// The rule, so that a header the visitor wrote can never pick the address:
//   - No forwarding header: the peer address, trusted.
//   - Headers from a peer that is not a trusted proxy: ignored; the peer
//     address, not trusted for the skip.
//   - From a trusted proxy: every X-Forwarded-For line is read (HAProxy may
//     add its own line after the visitor's), and the entries are walked from
//     the right, skipping trusted proxies; the first other address is the
//     client. X-Real-IP is read too, unless it names a trusted proxy.
//   - If both headers give a client and they differ, one was written by the
//     visitor and we cannot tell which (nginx with only X-Real-IP passes the
//     visitor's X-Forwarded-For through; other proxies pass the visitor's
//     X-Real-IP through; Cloudflare in front of such an nginx makes them
//     differ on every request). Then the X-Real-IP client is the limit key:
//     the trusted proxy sets it, while the visitor can pick X-Forwarded-For
//     in that setup, so keying on it would let one visitor dodge the limit.
//     It is not trusted for the skip.
//   - A Forwarded header, or an entry that is not an address, makes the
//     result not trusted for the skip.
//
// The returned address is always usable as a limit key; ok says whether it
// may be used for the local-network skip.
func (a *Auth) clientAddr(r *http.Request) (addr string, ok bool) {
	addr, ok, _ = a.clientAddrUnsure(r)
	return addr, ok
}

// clientAddrUnsure is clientAddr plus unsure: true when the two proxy headers
// disagree, so the address is one of them and may be written by the visitor.
// It stays a fine per-visitor limit key but must never stand for a known place.
func (a *Auth) clientAddrUnsure(r *http.Request) (addr string, ok, unsure bool) {
	addr, _, ok, unsure = a.clientAddrBoth(r)
	return addr, ok, unsure
}

// clientAddrBoth is clientAddrUnsure plus other: when the two proxy headers
// disagree, the X-Forwarded-For address too. Either may be the visitor's, so
// limits count a failure under both.
func (a *Auth) clientAddrBoth(r *http.Request) (addr, other string, ok, unsure bool) {
	peer, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		peer = r.RemoteAddr
	}
	peer = strings.Trim(peer, "[]")
	xffs := r.Header.Values("X-Forwarded-For")
	real := strings.TrimSpace(r.Header.Get("X-Real-IP"))
	fwd := r.Header.Get("Forwarded")
	if len(xffs) == 0 && real == "" && fwd == "" {
		return peer, "", true, false
	}
	pip := net.ParseIP(peer)
	if pip == nil || !a.trustedProxy(pip) {
		a.warnUntrustedProxy(pip)
		return peer, "", false, false
	}
	ok = fwd == ""
	fromXFF := ""
	var entries []string
	for _, line := range xffs {
		for _, e := range strings.Split(line, ",") {
			if e = strings.TrimSpace(e); e != "" {
				entries = append(entries, e)
			}
		}
	}
	for i := len(entries) - 1; i >= 0; i-- {
		ip := parseHostIP(entries[i])
		if ip == nil {
			return peer, "", false, false
		}
		fromXFF = ip.String()
		if !a.trustedProxy(ip) {
			break
		}
		// Every entry so far is a trusted proxy: the left-most one stands.
	}
	fromReal := ""
	if real != "" {
		if ip := parseHostIP(real); ip == nil {
			ok = false
		} else if !a.trustedProxy(ip) || fromXFF == "" {
			fromReal = ip.String()
		}
	}
	switch {
	case fromXFF == "" && fromReal == "":
		return peer, "", false, false
	case fromXFF == "":
		return fromReal, "", ok, false
	case fromReal == "" || fromReal == fromXFF:
		return fromXFF, "", ok, false
	default:
		return fromReal, fromXFF, false, true
	}
}

// warnUntrustedProxy logs, at most once per MaxBackoff, that forwarding
// headers from a loopback or private peer were ignored: most likely a reverse
// proxy that is not set as trusted, so every visitor shares its sign-in limit.
func (a *Auth) warnUntrustedProxy(peer net.IP) {
	if peer == nil || !peer.IsLoopback() && !peer.IsPrivate() {
		return
	}
	now := time.Now()
	if a.now != nil {
		now = a.now()
	}
	a.noteProxyPeer(peer, now)
	last := a.proxyWarned.Load()
	if last != 0 && now.Sub(time.Unix(0, last)) < MaxBackoff || !a.proxyWarned.CompareAndSwap(last, now.UnixNano()) {
		return
	}
	log.Printf("auth: X-Forwarded-For from %s ignored; every visitor shares one sign-in limit. Set %s=%s", peer, EnvTrustedProxies, peer)
}

// noteProxyPeer remembers the private peer that sends forwarding headers
// without being trusted: most likely the owner's reverse proxy. It sticks
// while that peer keeps sending them, so another peer can only take its place
// once it has been quiet for MaxBackoff; a visitor writing the headers once
// cannot push the real proxy out.
func (a *Auth) noteProxyPeer(peer net.IP, now time.Time) {
	s := peer.String()
	a.proxyMu.Lock()
	defer a.proxyMu.Unlock()
	if a.proxyPeer == "" || a.proxyPeer == s || now.Sub(a.proxyPeerAt) > MaxBackoff {
		a.proxyPeer, a.proxyPeerAt = s, now
	}
}

// seenProxy says whether ip is the untrusted proxy noteProxyPeer recorded.
func (a *Auth) seenProxy(ip net.IP) bool {
	if ip == nil {
		return false
	}
	a.proxyMu.Lock()
	defer a.proxyMu.Unlock()
	return a.proxyPeer != "" && a.proxyPeer == ip.String()
}

// parseHostIP reads an address that may carry a port or brackets.
func parseHostIP(s string) net.IP {
	if h, _, err := net.SplitHostPort(s); err == nil {
		s = h
	}
	return net.ParseIP(strings.Trim(s, "[]"))
}
