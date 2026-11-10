package sources

import (
	"errors"
	"strings"
	"testing"
)

// A Source that searches through Prowlarr or Jackett names that app when it cannot be reached.
func TestExplainViaNamesTheBridge(t *testing.T) {
	down := errors.New("dial tcp 192.168.1.36:9696: connect: connection refused")
	for addr, want := range map[string]string{
		"http://192.168.1.36:9696/12/api":                             "Prowlarr isn't answering",
		"http://192.168.1.36:9696/12":                                 "Prowlarr isn't answering",
		"http://jackett:9117/api/v2.0/indexers/nyaa/results/torznab/": "Jackett isn't answering",
		"https://indexer.example/api":                                 "HolyArrs could not reach the Source",
	} {
		if got := ExplainVia(addr, down); !strings.HasPrefix(got, want) {
			t.Errorf("%s: got %q, want it to start %q", addr, got, want)
		}
	}
}
