package upstream

import (
	"errors"
	"net"
	"net/http"
	"net/netip"
	"sync/atomic"
	"syscall"
	"time"
)

// BlockPrivate makes the outgoing dialers (the egress pool's and
// PublicTransport's) refuse loopback, private, link-local and unspecified
// addresses. serve turns it on; tests that dial local fakes leave it off.
var BlockPrivate atomic.Bool

// ErrBlockedAddress is a dial to an address that is not public.
var ErrBlockedAddress = errors.New("upstream: blocked address")

// publicOnly is a dialer Control that refuses non-public addresses while
// BlockPrivate is on.
func publicOnly(_, address string, _ syscall.RawConn) error {
	if !BlockPrivate.Load() {
		return nil
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return ErrBlockedAddress
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return ErrBlockedAddress
	}
	if !PublicAddr(ip) {
		return ErrBlockedAddress
	}
	return nil
}

// nonPublic are IPv4 ranges the standard checks miss: "this network",
// shared CGNAT space (Tailscale peers live there), benchmarking and reserved.
var nonPublic = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("240.0.0.0/4"),
}

// PublicAddr reports an address that may be dialed for an outside source.
func PublicAddr(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() || ip.IsMulticast() || ip.IsInterfaceLocalMulticast() {
		return false
	}
	for _, p := range nonPublic {
		if p.Contains(ip) {
			return false
		}
	}
	return true
}

// PublicTransport is the default transport with a dialer that refuses
// non-public addresses (while BlockPrivate is on), for picture downloads
// and their sources.
func PublicTransport() http.RoundTripper {
	t := http.DefaultTransport.(*http.Transport).Clone()
	d := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second, Control: publicOnly}
	t.DialContext = d.DialContext
	return t
}
