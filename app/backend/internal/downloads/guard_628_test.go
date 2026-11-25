package downloads

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
)

// a Downloader on a Docker bridge (172.16.0.0/12) is reached only with the
// local network switch on; cloud metadata and other blocked ranges stay refused
// even with it on.
func TestDockerBridgeAndMetadataPolicy(t *testing.T) {
	for _, c := range []struct {
		ip               string
		local, withLocal bool
	}{
		{"172.26.0.10", false, true},      // user-defined Docker bridge (the report)
		{"172.17.0.2", false, true},       // default docker0
		{"10.88.0.5", false, true},        // Podman default
		{"192.168.1.20", false, true},     // Synology/Unraid/TrueNAS host or macvlan
		{"127.0.0.1", false, true},        // same container
		{"169.254.169.254", false, false}, // cloud metadata
		{"fd00:ec2::254", false, false},   // AWS IPv6 metadata
		{"100.100.100.200", false, false}, // Alibaba metadata
		{"0.0.0.0", false, false},
		{"8.8.8.8", true, true},
	} {
		ip := net.ParseIP(c.ip)
		if got := outbound.PublicOnly(ip); got != c.local {
			t.Errorf("%s switch off: allowed=%v, want %v", c.ip, got, c.local)
		}
		if got := outbound.NotMetadata(ip); got != c.withLocal {
			t.Errorf("%s switch on: allowed=%v, want %v", c.ip, got, c.withLocal)
		}
	}
	// A name is checked by the address it resolves to, at dial time.
	_, err := Client("http://localhost:1", false).Get("http://localhost:1/")
	if err == nil || !errors.Is(err, outbound.ErrAddressRefused) {
		t.Fatalf("a name resolving to a local address must be refused with the switch off, got %v", err)
	}
	_, err = Client("http://169.254.169.254", true).Do(mustReq(t, "http://169.254.169.254/latest/meta-data/"))
	if err == nil || !errors.Is(err, outbound.ErrAddressRefused) {
		t.Fatalf("metadata must stay refused with the switch on, got %v", err)
	}
}

func mustReq(t *testing.T, u string) *http.Request {
	r, err := http.NewRequestWithContext(context.Background(), http.MethodGet, u, nil)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
