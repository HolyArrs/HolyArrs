package outbound

import (
	"errors"
	"net"
	"testing"
)

func TestPublicOnly(t *testing.T) {
	for addr, ok := range map[string]bool{
		"127.0.0.1": false, "10.1.2.3": false, "172.16.0.9": false, "192.168.1.1": false, "169.254.169.254": false,
		"100.64.0.1": false, "100.127.255.254": false, "0.0.0.0": false, "224.0.0.1": false, "255.255.255.255": false,
		"::1": false, "fe80::1": false, "fc00::1": false, "fd12::1": false, "::ffff:10.0.0.1": false, "::": false,
		"0.1.2.3": false, "198.18.0.1": false, "198.19.255.1": false, "240.0.0.1": false, "64:ff9b::a00:1": false,
		"64:ff9b:1::1": false, "2002:a00:1::1": false, "198.20.0.1": true, "fec0::1": false, "::a00:1": false,
		"8.8.8.8": true, "1.1.1.1": true, "100.128.0.1": true, "2001:4860:4860::8888": true,
	} {
		if got := PublicOnly(net.ParseIP(addr)); got != ok {
			t.Errorf("PublicOnly(%s) = %v, want %v", addr, got, ok)
		}
	}
}

// TestNotMetadataLinkLocal: ordinary link-local IPv4 (rootless Podman gives
// the host as 169.254.1.2) is the local network; cloud metadata services,
// their neighbours and every IPv6 link-local address stay refused, in every
// form that can carry them.
func TestNotMetadataLinkLocal(t *testing.T) {
	for addr, want := range map[string]bool{
		"169.254.1.2": true, "::ffff:169.254.1.2": true, "169.254.1.1": true, "169.254.100.5": true,
		"169.254.168.255": true, "169.254.171.0": true, "169.254.41.255": true, "169.254.43.0": true, "169.254.254.255": true,
		"192.168.1.2": true, "127.0.0.1": true, "::1": true, "10.0.0.5": true, "172.16.0.1": true, "fd12::1": true,
		"100.64.0.1": true, "8.8.8.8": true, "2001:4860:4860::8888": true, "fd00:ec3::254": true,
		"169.254.20.10": true, "169.254.25.10": true, "10.0.2.2": true, "192.168.65.254": true, "172.17.0.1": true,
		"100.100.100.100": true, "198.19.249.1": true, "fd7a:115c:a1e0::1": true, "fd20:1::5": true,
		"64:ff9b::808:808": true, "64:ff9b::a9fe:102": true,

		"fd20:ce::254": false, "fd00:c1::a9fe:a9fe": false, "fd00:a9fe:a9fe::1": false, "168.63.129.16": false,
		"::ffff:168.63.129.16": false, "::ffff:169.254.0.23": false, "::ffff:169.254.42.42": false, "::ffff:169.254.255.1": false,
		"::ffff:169.254.170.23": false, "::ffff:169.254.169.253": false, "64:ff9b::a83f:8110": false, "64:ff9b::a9fe:2a2a": false,

		"169.254.169.254": false, "169.254.169.253": false, "169.254.169.123": false, "169.254.169.0": false, "169.254.169.255": false,
		"169.254.170.2": false, "169.254.170.23": false, "169.254.170.0": false, "169.254.170.255": false,
		"169.254.0.23": false, "169.254.0.1": false, "169.254.42.42": false, "169.254.255.255": false, "169.254.255.1": false,
		"::ffff:169.254.169.254": false, "::ffff:169.254.170.2": false, "::ffff:100.100.100.200": false,
		"64:ff9b::a9fe:a9fe": false, "64:ff9b::a9fe:aa02": false, "64:ff9b:1::a9fe:a9fe": false, "64:ff9b::0:0": false,
		"2002:a9fe:a9fe::1": false, "2002:a9fe:102::1": false,
		"fe80::1": false, "fe80::a9fe:a9fe": false, "febf::1": false,
		"fd00:ec2::254": false, "fd00:ec2::23": false, "fd00:ec2::123": false, "fd00:ec2::253": false, "fd00:42::42": false,
		"100.100.100.200": false, "::a9fe:a9fe": false, "::ffff:0:a9fe:a9fe": false, "::ffff:0:a9fe:102": false, "64:ff9b::c000:c0": false,
	} {
		if got := NotMetadata(net.ParseIP(addr)); got != want {
			t.Errorf("NotMetadata(%s) = %v, want %v", addr, got, want)
		}
		if got := MetadataServiceLocal(net.ParseIP(addr)); got != want {
			t.Errorf("MetadataServiceLocal(%s) = %v, want %v", addr, got, want)
		}
	}
	// The stricter policy: no link-local address at all, no Azure WireServer,
	// and a NAT64 address judged by the IPv4 address it carries.
	for addr, want := range map[string]bool{
		"169.254.1.2": false, "169.254.169.254": false, "fe80::1": false, "168.63.129.16": false, "::ffff:168.63.129.16": false,
		"64:ff9b::a83f:8110": false, "64:ff9b::c0a8:101": false, "64:ff9b::a9fe:a9fe": false, "64:ff9b::7f00:1": false,
		"64:ff9b::808:808": true, "1.1.1.1": true, "198.18.0.5": false, "64:ff9b:1::808:808": false, "2002:808:808::1": false,
		"fd20:ce::254": false, "fd00:c1::a9fe:a9fe": false, "fd00:a9fe:a9fe::1": false,
		"192.0.0.192": false, "64:ff9b::c000:c0": false, "64:ff9b::6440:1": false, "64:ff9b::ffff:ffff": false, "64:ff9b::e000:1": false,
		"::a9fe:a9fe": false, "::ffff:0:a9fe:a9fe": false, "::1": false,
	} {
		if got := PublicOnly(net.ParseIP(addr)); got != want {
			t.Errorf("PublicOnly(%s) = %v, want %v", addr, got, want)
		}
	}
}

// TestPolicyDialerMetadataError: a refused metadata address gets its own
// error, so the page can say why even with the local network allowed; it
// still counts as a refused address.
func TestPolicyDialerMetadataError(t *testing.T) {
	for _, allow := range []func(net.IP) bool{NotMetadata, PublicOnly} {
		d := PolicyDialer(allow)
		for _, addr := range []string{"169.254.169.254:80", "[::ffff:168.63.129.16]:80", "[64:ff9b::a9fe:a9fe]:80", "[fd20:ce::254]:80"} {
			err := d.Control("tcp", addr, nil)
			if !errors.Is(err, ErrMetadataRefused) || !errors.Is(err, ErrAddressRefused) {
				t.Errorf("dial %s = %v, want the metadata refusal", addr, err)
			}
		}
	}
	err := PolicyDialer(PublicOnly).Control("tcp", "192.168.1.5:80", nil)
	if !errors.Is(err, ErrAddressRefused) || errors.Is(err, ErrMetadataRefused) {
		t.Errorf("local address under PublicOnly = %v, want the plain refusal", err)
	}
	if Refusal(&net.OpError{Op: "dial", Err: ErrMetadataRefused}) != ErrMetadataRefused || Refusal(&net.OpError{Op: "dial", Err: ErrAddressRefused}) != ErrAddressRefused {
		t.Error("Refusal lost which refusal it was")
	}
}

// TestPolicyDialerLinkLocal checks the dialer's own hook with the address
// it is handed after DNS: a name such as host.containers.internal that
// resolves to 169.254.1.2 passes, one that resolves to a metadata address
// does not. The machine has no link-local interface to listen on, so the
// real connection is shown on loopback under the same policy.
func TestPolicyDialerLinkLocal(t *testing.T) {
	d := PolicyDialer(NotMetadata)
	for addr, ok := range map[string]bool{
		"169.254.1.2:8112": true, "[::ffff:169.254.1.2]:8112": true,
		"169.254.169.254:80": false, "[::ffff:169.254.169.254]:80": false, "169.254.170.2:80": false,
		"[fe80::1]:80": false, "[fd00:ec2::254]:80": false,
	} {
		err := d.Control("tcp", addr, nil)
		if ok && err != nil {
			t.Errorf("dial %s refused: %v", addr, err)
		}
		if !ok && !errors.Is(err, ErrAddressRefused) {
			t.Errorf("dial %s = %v, want refused", addr, err)
		}
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	c, err := d.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("local listener not reachable under NotMetadata: %v", err)
	}
	c.Close()
}

func TestNotMetadataOddRanges(t *testing.T) {
	for addr, want := range map[string]bool{
		"0.0.0.0": false, "::": false, "224.0.0.1": false, "255.255.255.255": false,
		"64:ff9b::a9fe:a9fe": false, "192.0.0.192": false, "2002:a9fe:a9fe::": false,
		"192.168.1.2": true, "127.0.0.1": true, "10.0.0.5": true,
	} {
		if got := NotMetadata(net.ParseIP(addr)); got != want {
			t.Errorf("NotMetadata(%s) = %v, want %v", addr, got, want)
		}
	}
}
