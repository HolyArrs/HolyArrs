package plugins

import (
	"net"
	"net/netip"
	"strings"
)

// maxHosts is how many websites a manifest may list; maxOwnerHosts is how
// many the owner may add (a self-hosted Headscale, for example).
const (
	maxHosts      = 100
	maxOwnerHosts = 100
)

// normalizeHost lowercases a name and drops one trailing dot. It returns ""
// for anything that is not a plain ASCII DNS name; international names must
// be written in their xn-- form.
func normalizeHost(h string) string {
	h = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(h)), ".")
	if h == "" || len(h) > 253 {
		return ""
	}
	a := h
	for _, l := range strings.Split(a, ".") {
		if l == "" || len(l) > 63 {
			return ""
		}
		for _, c := range l {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
				return ""
			}
		}
	}
	return a
}

// isIPLiteral is true for every way of writing an address instead of a name:
// v4, v6, bracketed, integer, octal or hex forms, and any name whose last
// label is all digits or hex (inet_aton accepts those).
func isIPLiteral(h string) bool {
	h = strings.TrimSuffix(strings.TrimSpace(h), ".")
	if strings.HasPrefix(h, "[") || strings.Contains(h, ":") {
		return true
	}
	if _, err := netip.ParseAddr(h); err == nil {
		return true
	}
	labels := strings.Split(h, ".")
	last := strings.ToLower(labels[len(labels)-1])
	if last == "" {
		return true
	}
	if strings.HasPrefix(last, "0x") {
		return true
	}
	for _, c := range last {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// validHostPattern checks one manifest host: a plain name, or "*." as the
// leftmost label with at least two labels after it; no IPs, ports or paths.
func validHostPattern(p string) bool {
	if p != strings.ToLower(p) || strings.ContainsAny(p, "/:@ ") || isIPLiteral(strings.TrimPrefix(p, "*.")) {
		return false
	}
	rest := strings.TrimPrefix(p, "*.")
	if strings.Contains(rest, "*") || normalizeHost(rest) != rest {
		return false
	}
	if rest != p && strings.Count(rest, ".") < 1 {
		return false
	}
	return true
}

// matchHost reports whether name is allowed by one of the patterns. A pattern
// "*.x.y" matches one or more labels under x.y, never x.y itself.
func matchHost(patterns []string, name string) bool {
	n := normalizeHost(name)
	if n == "" || isIPLiteral(n) {
		return false
	}
	for _, p := range patterns {
		if rest, ok := strings.CutPrefix(p, "*."); ok {
			if strings.HasSuffix(n, "."+rest) {
				return true
			}
			continue
		}
		if n == p {
			return true
		}
	}
	return false
}

var badRanges = func() []netip.Prefix {
	var out []netip.Prefix
	for _, s := range []string{
		"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12",
		"192.0.0.0/24", "192.168.0.0/16", "198.18.0.0/15", "224.0.0.0/4", "240.0.0.0/4",
		"::/128", "::1/128", "fc00::/7", "fe80::/10", "ff00::/8", "100::/64", "2001:db8::/32",
		// Old forms that can still carry a private v4 address over a tunnel:
		// IPv4-compatible, SIIT, site-local, Teredo and the 6to4 relay.
		"::/96", "::ffff:0:0:0/96", "fec0::/10", "2001::/32", "192.88.99.0/24",
	} {
		out = append(out, netip.MustParsePrefix(s))
	}
	return out
}()

// alwaysBad are ranges no host may reach, even one the owner added.
var alwaysBad = func() []netip.Prefix {
	var out []netip.Prefix
	for _, s := range []string{"0.0.0.0/8", "127.0.0.0/8", "169.254.0.0/16", "224.0.0.0/4", "240.0.0.0/4", "::/128", "::1/128", "fe80::/10", "ff00::/8",
		"::/96", "::ffff:0:0:0/96", "fec0::/10", "2001::/32", "192.88.99.0/24"} {
		out = append(out, netip.MustParsePrefix(s))
	}
	return out
}()

// embedded4 returns the v4 address inside a v4-mapped, 6to4 or NAT64 form.
func embedded4(a netip.Addr) (netip.Addr, bool) {
	if a.Is4In6() {
		return a.Unmap(), true
	}
	if !a.Is6() {
		return netip.Addr{}, false
	}
	b := a.As16()
	if b[0] == 0x20 && b[1] == 0x02 { // 6to4 2002:V4ADDR::/48
		return netip.AddrFrom4([4]byte{b[2], b[3], b[4], b[5]}), true
	}
	if netip.MustParsePrefix("64:ff9b::/96").Contains(a) || netip.MustParsePrefix("64:ff9b:1::/48").Contains(a) {
		return netip.AddrFrom4([4]byte{b[12], b[13], b[14], b[15]}), true
	}
	return netip.Addr{}, false
}

// safeAddr reports whether a plugin may be connected to ip. allowPrivate
// lets owner-added hosts reach LAN ranges; loopback, link-local (and so
// cloud metadata), multicast and unspecified are never allowed. own are the
// host's own interface addresses, always refused.
func safeAddr(ip netip.Addr, allowPrivate bool, own []netip.Addr) bool {
	if !ip.IsValid() {
		return false
	}
	ip = ip.WithZone("")
	if v4, ok := embedded4(ip); ok {
		if !safeAddr(v4, allowPrivate, own) {
			return false
		}
		if ip.Is4In6() {
			return true
		}
	}
	for _, o := range own {
		if o.Unmap() == ip.Unmap() {
			return false
		}
	}
	list := badRanges
	if allowPrivate {
		list = alwaysBad
	}
	for _, p := range list {
		if p.Contains(ip) {
			return false
		}
	}
	return true
}

// ownAddrs lists this host's interface addresses.
func ownAddrs() []netip.Addr {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	var out []netip.Addr
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok {
			if ip, ok := netip.AddrFromSlice(n.IP); ok {
				out = append(out, ip.Unmap())
			}
		}
	}
	return out
}

// ParseOwnerHost checks one owner-added host, "name" or "name:port".
func ParseOwnerHost(s string) (host string, port int, ok bool) {
	s = strings.TrimSpace(s)
	h, p := s, ""
	if i := strings.LastIndexByte(s, ':'); i >= 0 {
		h, p = s[:i], s[i+1:]
		if p == "" {
			return "", 0, false
		}
	}
	if isIPLiteral(h) {
		return "", 0, false
	}
	n := normalizeHost(h)
	if n == "" || !strings.Contains(n, ".") {
		return "", 0, false
	}
	if p != "" {
		v := 0
		for _, c := range p {
			if c < '0' || c > '9' {
				return "", 0, false
			}
			v = v*10 + int(c-'0')
			if v > 65535 {
				return "", 0, false
			}
		}
		if v == 0 {
			return "", 0, false
		}
		port = v
	}
	return n, port, true
}
