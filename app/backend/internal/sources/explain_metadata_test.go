package sources

import (
	"context"
	"strings"
	"testing"
)

// TestExplainMetadataSource: a Source at a cloud metadata address says so
// when tested, with or without the local network, instead of asking to
// tick "May reach the local network".
func TestExplainMetadataSource(t *testing.T) {
	for _, local := range []bool{true, false} {
		for _, host := range []string{"169.254.169.254", "[fd00:ec2::254]", "168.63.129.16"} {
			_, err := New(Config{URL: "http://" + host + "/api", Protocol: "torrent", AllowLocal: local}).Caps(context.Background())
			got := Explain(err)
			if !strings.Contains(got, "cloud metadata address") || strings.Contains(got, "May reach the local network") {
				t.Errorf("local=%v %s: %q", local, host, got)
			}
		}
	}
	_, err := New(Config{URL: "http://192.168.1.5/api", Protocol: "torrent"}).Caps(context.Background())
	if got := Explain(err); !strings.Contains(got, "Reach the local network") {
		t.Errorf("local address without the switch: %q", got)
	}
}
