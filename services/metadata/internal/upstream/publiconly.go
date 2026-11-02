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

// PublicAddr reports an address that may be dialed for an outside source.
func PublicAddr(ip netip.Addr) bool {
	ip = ip.Unmap()
	return ip.IsValid() && !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() &&
		!ip.IsUnspecified() && !ip.IsMulticast() && !ip.IsInterfaceLocalMulticast()
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
