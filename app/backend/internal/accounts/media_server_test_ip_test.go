package accounts

import (
	"net"
	"testing"
)

// TestBlockedTestIPLinkLocal: the address test allows the same local
// network the real media server connection does (169.254.1.2 is the host
// under rootless Podman) and refuses every cloud metadata address.
func TestBlockedTestIPLinkLocal(t *testing.T) {
	for addr, want := range map[string]bool{
		"169.254.1.2": false, "192.168.1.5": false, "127.0.0.1": false,
		"169.254.169.254": true, "169.254.170.2": true, "::ffff:169.254.169.254": true, "fe80::1": true,
		"fd00:ec2::254": true, "100.100.100.200": true, "0.0.0.0": true, "224.0.0.1": true,
		"168.63.129.16": true, "fd20:ce::254": true, "fd00:a9fe:a9fe::1": true,
		"2002:a9fe:a9fe::1": true, "::a9fe:a9fe": true, "::ffff:0:a9fe:a9fe": true, "::1": false,
	} {
		if got := blockedTestIP(net.ParseIP(addr)); got != want {
			t.Errorf("blockedTestIP(%s) = %v, want %v", addr, got, want)
		}
	}
}
