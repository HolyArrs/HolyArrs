package main

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// TestEveryServedRouteMappedOrClosedForPlugins walks every route the server
// registers: each is either given a plugin scope or closed, never both, so a
// new route can't be reached by a plugin until someone decides.
func TestEveryServedRouteMappedOrClosedForPlugins(t *testing.T) {
	seen := map[string]bool{}
	for _, dir := range routePackages {
		rs, err := webserver.RegisteredRoutes(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range rs {
			seen[r] = true
		}
	}
	for r := range accounts.RoutePerms {
		seen[r] = true
	}
	if len(seen) < 500 {
		t.Fatalf("found only %d routes", len(seen))
	}
	for r := range seen {
		_, mapped := accounts.PluginScopes[r]
		if mapped == accounts.PluginClosed[r] {
			t.Errorf("%s: put it in exactly one of routes or closed in internal/accounts/plugin_scopes.json", r)
		}
	}
	for r := range accounts.PluginScopes {
		if !seen[r] {
			t.Errorf("%s: mapped for plugins but no longer served", r)
		}
	}
}
