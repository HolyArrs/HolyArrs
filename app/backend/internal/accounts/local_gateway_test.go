package accounts

import (
	"net"
	"net/http/httptest"
	"strings"
	"testing"
)

// The container's own default gateway (what the container sees when someone
// opens the host's LAN IP) counts as local while the skip is on, even when
// the owner's list does not hold it. Other bridge addresses do not, and
// forwarding headers from the gateway still need it to be a trusted proxy.
func TestSkipLocalOwnGateway(t *testing.T) {
	old := readGateways
	readGateways = func() []net.IP { return []net.IP{net.ParseIP("172.18.0.1")} }
	t.Cleanup(func() { readGateways = old; resetGateways() })
	resetGateways()
	nets, err := parseLocalNets([]string{"192.168.1.0/24"})
	if err != nil {
		t.Fatal(err)
	}
	try := func(a *Auth, peer, xff string) bool {
		r := httptest.NewRequest("GET", "/api/v1/titles", nil)
		r.RemoteAddr, r.Host = peer+":5000", "192.168.1.9:6262"
		if xff != "" {
			r.Header.Set("X-Forwarded-For", xff)
		}
		return a.strictLocal(r)
	}
	a := setup(t, Options{}).a
	a.localNets.Store(nets)
	if try(a, "172.18.0.1", "") {
		t.Error("the gateway skipped sign-in with the skip off")
	}
	a.skipLocal.Store(true)
	if !try(a, "172.18.0.1", "") {
		t.Error("the container's own gateway did not skip")
	}
	if try(a, "172.17.0.1", "") {
		t.Error("another bridge address skipped sign-in")
	}
	b := setup(t, Options{}).a
	b.localNets.Store(nets)
	b.skipLocal.Store(true)
	if try(b, "172.18.0.1", "192.168.1.20") {
		t.Error("forwarding headers from an untrusted gateway skipped sign-in")
	}
}

// parseRoute reads the default gateways from /proc/net/route text.
func TestParseRouteGateways(t *testing.T) {
	in := "Iface\tDestination\tGateway \tFlags\tRefCnt\tUse\tMetric\tMask\t\tMTU\tWindow\tIRTT\n" +
		"eth0\t00000000\t010012AC\t0003\t0\t0\t0\t00000000\t0\t0\t0\n" +
		"eth0\t000012AC\t00000000\t0001\t0\t0\t0\t0000FFFF\t0\t0\t0\n"
	got := parseRoute(strings.NewReader(in))
	if len(got) != 1 || got[0].String() != "172.18.0.1" {
		t.Fatalf("parseRoute = %v", got)
	}
	in6 := "00000000000000000000000000000000 00 00000000000000000000000000000000 00 fe800000000000000000000000000001 00000400 00000001 00000000 00000003 eth0\n" +
		"fd000000000000000000000000000000 40 00000000000000000000000000000000 00 00000000000000000000000000000000 00000100 00000001 00000000 00000001 eth0\n"
	got6 := parseRoute6(strings.NewReader(in6))
	if len(got6) != 1 || got6[0].String() != "fe80::1" {
		t.Fatalf("parseRoute6 = %v", got6)
	}
}
