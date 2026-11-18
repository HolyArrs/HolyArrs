package main

import (
	"net/http"
	"testing"
)

func TestPluginRoutesAreOwnerOnly(t *testing.T) {
	t.Parallel()
	r := newAskRig(t)
	for _, rt := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/plugins"},
		{http.MethodPost, "/api/v1/plugins/tailscale/install"},
		{http.MethodPost, "/api/v1/plugins/tailscale/start"},
		{http.MethodPost, "/api/v1/plugins/tailscale/stop"},
		{http.MethodDelete, "/api/v1/plugins/tailscale"},
		{http.MethodGet, "/api/v1/plugins/tailscale/status"},
		{http.MethodPost, "/api/v1/plugins/tailscale/signout"},
	} {
		r.ana.call(rt.method, rt.path, "", http.StatusForbidden)
	}
	// The rig has no Plugin Engine; the owner gets past the gate.
	r.owner.call(http.MethodGet, "/api/v1/plugins", "", http.StatusServiceUnavailable)
}
