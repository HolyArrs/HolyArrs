package plugins_test

import (
	"net/http"
	"slices"
	"testing"
)

// TestReachHosts: a plugin's reach is its approved hosts plus the owner's,
// and changes when the owner adds one, which a bot-check pin relies on.
func TestReachHosts(t *testing.T) {
	ls, pub := newListServer(t)
	e, _ := newEngine(t, ls, pub)
	m := mux(e)
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusOK)
	before := e.ReachHosts("tailscale")
	callBody(t, m, "PUT", "/api/v1/plugins/tailscale/owner-hosts", `{"hosts":["fs.home.arpa:8191"]}`, http.StatusOK)
	after := e.ReachHosts("tailscale")
	if !slices.Contains(after, "fs.home.arpa:8191") || slices.Equal(before, after) || !slices.IsSorted(after) {
		t.Fatalf("before %v, after %v", before, after)
	}
	if e.ReachHosts("missing") != nil {
		t.Fatal("a missing plugin reaches hosts")
	}
}
