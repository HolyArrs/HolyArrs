package upstream

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

// Security L1: with BlockPrivate on, the dialers refuse loopback, private,
// link-local and unspecified addresses and allow public ones.
func TestPublicOnlyDial(t *testing.T) {
	BlockPrivate.Store(true)
	defer BlockPrivate.Store(false)
	for _, a := range []string{"127.0.0.1:80", "10.0.0.5:443", "192.168.1.1:443", "169.254.169.254:80", "[::1]:443", "0.0.0.0:80", "[fe80::1]:443"} {
		if err := publicOnly("tcp", a, nil); !errors.Is(err, ErrBlockedAddress) {
			t.Errorf("%s allowed", a)
		}
	}
	for _, a := range []string{"1.1.1.1:443", "[2606:4700::1111]:443"} {
		if err := publicOnly("tcp", a, nil); err != nil {
			t.Errorf("%s blocked: %v", a, err)
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	c := &http.Client{Transport: PublicTransport()}
	if _, err := c.Get(srv.URL); !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("loopback fetched: %v", err)
	}
	BlockPrivate.Store(false)
	if resp, err := c.Get(srv.URL); err != nil {
		t.Fatalf("off: %v", err)
	} else {
		resp.Body.Close()
	}
}

// Security 17: shared CGNAT (Tailscale peers) and reserved IPv4 ranges are
// not public.
func TestPublicAddrBlocksCGNAT(t *testing.T) {
	for _, a := range []string{"100.87.70.86", "100.64.0.1", "0.1.2.3", "198.18.0.1", "240.0.0.1", "::ffff:100.100.1.1"} {
		if PublicAddr(netip.MustParseAddr(a)) {
			t.Errorf("%s public", a)
		}
	}
	for _, a := range []string{"100.63.255.255", "100.128.0.1", "8.8.8.8"} {
		if !PublicAddr(netip.MustParseAddr(a)) {
			t.Errorf("%s blocked", a)
		}
	}
}
