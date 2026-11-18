package outbound

import (
	"errors"
	"fmt"
	"net"
	"syscall"
	"time"
)

// ErrAddressRefused is the dial error for an address the policy refuses.
var ErrAddressRefused = errors.New("address refused by the network policy")

// ErrMetadataRefused is ErrAddressRefused for a cloud metadata address
// (MetadataIP), which every policy refuses, so the page need not suggest
// allowing the local network.
var ErrMetadataRefused = fmt.Errorf("%w: a cloud metadata address", ErrAddressRefused)

// Refusal gives the bare refusal err holds, without the address:
// ErrMetadataRefused or ErrAddressRefused.
func Refusal(err error) error {
	if errors.Is(err, ErrMetadataRefused) {
		return ErrMetadataRefused
	}
	return ErrAddressRefused
}

// PolicyDialer is a dialer that asks allow about every address it connects
// to, after DNS, and refuses the ones it says no to. When the app proxy is
// on, calls go through it (see Dialer). Alerts use it too.
func PolicyDialer(allow func(net.IP) bool) *Dialer {
	return &Dialer{Dialer: *DirectDialer(allow), allow: allow}
}

// DirectDialer is PolicyDialer without the app proxy, for dialing a proxy
// itself.
func DirectDialer(allow func(net.IP) bool) *net.Dialer {
	return &net.Dialer{Timeout: 10 * time.Second, Control: func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		ip := net.ParseIP(host)
		if ip != nil && MetadataIP(ip) {
			return ErrMetadataRefused
		}
		if ip == nil || !allow(ip) {
			if IsFakeIP(ip) {
				return errFakeIPAddr
			}
			return ErrAddressRefused
		}
		return nil
	}}
}

// nat64 is the well-known NAT64 prefix: the last 32 bits are an IPv4
// address, reached through the network's NAT64 gateway.
var nat64 = func() *net.IPNet { _, n, _ := net.ParseCIDR("64:ff9b::/96"); return n }()

// nat64IPv4 gives the IPv4 address a 64:ff9b::/96 address carries, or nil.
func nat64IPv4(ip net.IP) net.IP {
	if ip.To4() != nil || len(ip) != net.IPv6len || !nat64.Contains(ip) {
		return nil
	}
	return net.IPv4(ip[12], ip[13], ip[14], ip[15]).To4()
}

// metadataNets are where cloud metadata, credential and host services
// answer, refused even where the local network is allowed. In 169.254.0.0/16
// whole /24s are refused so near neighbours stay out: .169 (the metadata
// service of AWS, GCP, Azure, Oracle, OpenStack and most others, plus AWS
// DNS and time), .170 (AWS ECS and EKS credentials), .42 (Scaleway), and .0
// and .255, which the link-local standard keeps off ordinary hosts. Every
// IPv6 link-local address stays refused (OpenStack answers on
// fe80::a9fe:a9fe), as do the IPv6 metadata services of AWS, GCP, Oracle,
// Linode and Scaleway, Alibaba's 100.100.100.200 and Azure's WireServer
// 168.63.129.16, a public address. The rest of 169.254.0.0/16 is the local
// network: rootless Podman, for one, gives the host as 169.254.1.2.
var metadataNets = func() []*net.IPNet {
	var out []*net.IPNet
	for _, c := range []string{"169.254.0.0/24", "169.254.42.0/24", "169.254.169.0/24", "169.254.170.0/24", "169.254.255.0/24",
		"fe80::/10", "fd00:ec2::/64", "fd20:ce::254/128", "fd00:c1::a9fe:a9fe/128", "fd00:a9fe:a9fe::1/128", "fd00:42::42/128",
		"100.100.100.200/32", "168.63.129.16/32"} {
		_, n, _ := net.ParseCIDR(c)
		out = append(out, n)
	}
	return out
}()

// MetadataIP says ip is a cloud metadata address (metadataNets), also when
// written as an IPv4-mapped IPv6 address or carried in a NAT64 address.
func MetadataIP(ip net.IP) bool {
	if v4 := nat64IPv4(ip); v4 != nil {
		ip = v4
	}
	for _, n := range metadataNets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// NotMetadata allows the local network, link-local IPv4 included, but never
// a cloud metadata address (metadataNets). It also refuses unspecified,
// multicast and broadcast addresses, the 64:ff9b:1::/48 and 6to4 ranges
// (which can carry a metadata address inside) and 192.0.0.0/24. A NAT64
// address (64:ff9b::/96) is judged by the IPv4 address it carries. It is the
// policy for anything that may reach the local network.
func NotMetadata(ip net.IP) bool {
	if v4 := nat64IPv4(ip); v4 != nil {
		ip = v4
	}
	if ip == nil || ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() || ip.IsMulticast() || ip.IsInterfaceLocalMulticast() || ip.Equal(net.IPv4bcast) {
		return false
	}
	for _, n := range oddRanges {
		if n.Contains(ip) && !ip.Equal(net.IPv6loopback) {
			return false
		}
	}
	return !MetadataIP(ip)
}

// oddRanges are refused even where the local network is allowed, including
// the IPv4-compatible (::/96) and SIIT (::ffff:0:0:0/96) forms that carry an
// IPv4 address in the low bits; ::1 is still loopback.
var oddRanges = func() []*net.IPNet {
	var out []*net.IPNet
	for _, c := range []string{"64:ff9b:1::/48", "2002::/16", "192.0.0.0/24", "::/96", "::ffff:0:0:0/96"} {
		_, n, _ := net.ParseCIDR(c)
		out = append(out, n)
	}
	return out
}()

// blocked are ranges refused besides the ones net.IP names: CGNAT (also
// Tailscale), "this network", benchmarking, reserved, and the IPv6 ranges
// that carry an IPv4 address inside (local-use NAT64 and 6to4), which could
// reach a local IPv4 host; also the old site-local fec0::/10 and the old
// IPv4-compatible ::/96 and SIIT ::ffff:0:0:0/96 (an IPv4 address in the low
// bits), and 192.0.0.0/24 (protocol assignments, once Oracle's metadata).
// Well-known NAT64 (64:ff9b::/96) is judged by the IPv4 address it carries.
var blocked = func() []*net.IPNet {
	var out []*net.IPNet
	for _, c := range []string{"100.64.0.0/10", "0.0.0.0/8", "198.18.0.0/15", "240.0.0.0/4", "192.0.0.0/24", "64:ff9b:1::/48", "2002::/16", "fec0::/10", "::/96", "::ffff:0:0:0/96"} {
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
// and 6to4 ranges, and cloud metadata addresses (MetadataIP). A NAT64
// address is judged by the IPv4 address it carries. The
// check runs on each address as it is dialled, after DNS and on every
// redirect, so a name that later resolves to a local address is refused too.
func PublicOnly(ip net.IP) bool {
	if v4 := nat64IPv4(ip); v4 != nil {
		ip = v4
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	return !(ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() || ip.Equal(net.IPv4bcast) || inBlocked(ip) || MetadataIP(ip))
}

func inBlocked(ip net.IP) bool {
	for _, n := range blocked {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// SelfCallHeader marks a call HolyArrs sends out to an address someone else
// chose, such as a hook. A request that carries it never skips sign-in on
// the local network, so a hook pointed back at HolyArrs gets no owner rights.
const SelfCallHeader = "X-Holyarrs-Outgoing"
