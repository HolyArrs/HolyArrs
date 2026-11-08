package outbound

import (
	"errors"
	"net"
	"syscall"
	"time"
)

// ErrAddressRefused is the dial error for an address the policy refuses.
var ErrAddressRefused = errors.New("address refused by the network policy")

// PolicyDialer is a dialer that asks allow about every address it connects
// to, after DNS, and refuses the ones it says no to. Alerts use it too.
func PolicyDialer(allow func(net.IP) bool) *net.Dialer {
	return &net.Dialer{Timeout: 10 * time.Second, Control: func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		if ip := net.ParseIP(host); ip == nil || !allow(ip) {
			return ErrAddressRefused
		}
		return nil
	}}
}

// metadataAddrs are cloud metadata services outside 169.254.0.0/16: the
// AWS IPv6 one and Alibaba's.
var metadataAddrs = []net.IP{net.ParseIP("fd00:ec2::254"), net.ParseIP("100.100.100.200")}

// NotMetadata allows the local network but never link-local addresses
// (169.254.0.0/16, where cloud metadata services answer, and fe80::/10) or
// the other metadata addresses. It also refuses unspecified, multicast and
// broadcast addresses, NAT64 and 6to4 ranges (which can carry a metadata address
// inside) and 192.0.0.0/24. It is the policy for anything that may reach
// the local network.
func NotMetadata(ip net.IP) bool {
	if ip == nil || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() || ip.IsMulticast() || ip.IsInterfaceLocalMulticast() || ip.Equal(net.IPv4bcast) {
		return false
	}
	for _, n := range oddRanges {
		if n.Contains(ip) {
			return false
		}
	}
	for _, m := range metadataAddrs {
		if m.Equal(ip) {
			return false
		}
	}
	return true
}

// oddRanges are refused even where the local network is allowed.
var oddRanges = func() []*net.IPNet {
	var out []*net.IPNet
	for _, c := range []string{"64:ff9b::/96", "64:ff9b:1::/48", "2002::/16", "192.0.0.0/24"} {
		_, n, _ := net.ParseCIDR(c)
		out = append(out, n)
	}
	return out
}()

// blocked are ranges refused besides the ones net.IP names: CGNAT (also
// Tailscale), "this network", benchmarking, reserved, and the IPv6 ranges
// that carry an IPv4 address inside (NAT64 and 6to4), which could reach a
// local IPv4 host; also the old site-local fec0::/10 and the old IPv4-compatible
// ::/96 (an IPv4 address in the low bits).
var blocked = func() []*net.IPNet {
	var out []*net.IPNet
	for _, c := range []string{"100.64.0.0/10", "0.0.0.0/8", "198.18.0.0/15", "240.0.0.0/4", "64:ff9b::/96", "64:ff9b:1::/48", "2002::/16", "fec0::/10", "::/96"} {
		_, n, _ := net.ParseCIDR(c)
		out = append(out, n)
	}
	return out
}()

// MetadataServiceLocal is the address policy for a lab metadata server
// (HOLYARRS_METADATA_SERVICE_ALLOW_LOCAL): local networks are allowed, but
// never cloud metadata, unspecified, multicast or broadcast addresses.
func MetadataServiceLocal(ip net.IP) bool {
	return ip != nil && NotMetadata(ip)
}

// PublicOnly refuses addresses on this machine or a local network: loopback,
// private, link-local, CGNAT, unspecified, multicast, broadcast, reserved
// and NAT64/6to4 ranges. The
// check runs on each address as it is dialled, after DNS and on every
// redirect, so a name that later resolves to a local address is refused too.
func PublicOnly(ip net.IP) bool {
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	return !(ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() || ip.Equal(net.IPv4bcast) || inBlocked(ip))
}

func inBlocked(ip net.IP) bool {
	for _, n := range blocked {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
