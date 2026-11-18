package sources

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// A Source that searches through Prowlarr or Jackett names that app when it cannot be reached.
func TestExplainViaNamesTheBridge(t *testing.T) {
	down := errors.New("dial tcp 192.168.1.36:9696: connect: connection refused")
	for addr, want := range map[string]string{
		"http://192.168.1.36:9696/12/api":                             "Prowlarr isn't answering",
		"http://192.168.1.36:9696/12":                                 "Prowlarr isn't answering",
		"http://jackett:9117/api/v2.0/indexers/nyaa/results/torznab/": "Jackett isn't answering",
		"https://indexer.example/api":                                 "Nothing answers on that port.",
	} {
		if got := ExplainVia(addr, down, nil); !strings.HasPrefix(got, want) {
			t.Errorf("%s: got %q, want it to start %q", addr, got, want)
		}
	}
}

func TestProbeDialKeepsTheSourceWay(t *testing.T) {
	if ProbeDial(domain.Source{ProxyURL: "http://proxy:3128"}) != nil {
		t.Fatal("a Source behind a proxy must not be probed directly")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if c, err := ProbeDial(domain.Source{})(context.Background(), "tcp", ln.Addr().String()); err == nil {
		c.Close()
		t.Fatal("the probe reached this machine without the local network allowed")
	}
	c, err := ProbeDial(domain.Source{AllowLocal: true})(context.Background(), "tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("local network allowed: %v", err)
	}
	c.Close()
}
