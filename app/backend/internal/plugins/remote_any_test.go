package plugins_test

import (
	"net/http"
	"testing"
)

// Remote access answers for a plugin only while the owner granted it
// network.forward.
func TestRemoteAccessNeedsForwardScope(t *testing.T) {
	for _, c := range []struct {
		name    string
		actions []string
		want    int
	}{
		{"forwarder", []string{"network.forward"}, http.StatusOK},
		{"other scopes only", []string{"library.read"}, http.StatusNotFound},
		{"no scopes", []string{}, http.StatusNotFound},
	} {
		t.Run(c.name, func(t *testing.T) {
			ls, pub := newListServer(t)
			e, _ := newEngine(t, ls, pub)
			m := mux(e)
			ls.publishWith("1.0.0", []byte("v100"+c.name), 2, func(man map[string]any) {
				man["min_app_version"] = "1.3.0"
				man["access"] = map[string]any{"hosts": []string{"controlplane.tailscale.com"}, "folders": []string{}, "actions": c.actions}
			})
			call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusOK)
			call(t, m, "GET", "/api/v1/plugins/tailscale/remote/status", c.want)
			call(t, m, "GET", "/api/v1/plugins/nope/remote/status", http.StatusNotFound)
		})
	}
}
