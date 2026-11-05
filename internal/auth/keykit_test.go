package auth

import (
	"net/http"
	"strings"
	"testing"
)

const keyKitRoute = "POST /api/v1/system/key-kit"

// The key kit is the master key itself: the owner only, with a fresh local
// password (and two-factor code when enrolled), never a read key, never a
// stale session, never an admin.
func TestKeyKitRoutePolicy(t *testing.T) {
	if need, ok := RoutePerms[keyKitRoute]; !ok || need != needOwner {
		t.Fatalf("RoutePerms[%q] = %q, %v; want owner only", keyKitRoute, need, ok)
	}
	if !riskyRoutes[keyKitRoute] {
		t.Fatalf("%s must ask for a fresh local password", keyKitRoute)
	}
	if ReadRoutes[keyKitRoute] {
		t.Fatalf("%s must not be open to read keys", keyKitRoute)
	}
	if sc := RouteScope(keyKitRoute); sc == ScopeRead || sc == ScopeHealthRead || sc == ScopeWrite {
		t.Fatalf("%s scope %q is too wide", keyKitRoute, sc)
	}

	owner := Who{UserID: 1, Via: "session", Role: RoleOwner, Perms: DefaultPerms(RoleOwner)}
	if code, body := gateCode(keyKitRoute, owner); code != http.StatusForbidden || !strings.Contains(body, `"reauth"`) {
		t.Errorf("stale owner session: %d %s; want 403 reauth", code, body)
	}
	fresh := owner
	fresh.LocalRecent = true
	if code, body := gateCode(keyKitRoute, fresh); code != http.StatusTeapot {
		t.Errorf("fresh owner session refused: %d %s", code, body)
	}
	admin := Who{UserID: 2, Via: "session", Role: RoleAdmin, Perms: DefaultPerms(RoleAdmin), LocalRecent: true, TwoFactor: true}
	if code, body := gateCode(keyKitRoute, admin); code != http.StatusForbidden {
		t.Errorf("fresh admin session reached the key kit: %d %s", code, body)
	}
	readKey := Who{UserID: 1, Via: "api_key", Role: RoleOwner, Perms: DefaultPerms(RoleOwner), Scopes: []string{ScopeRead}}
	if code, body := gateCode(keyKitRoute, readKey); code != http.StatusForbidden {
		t.Errorf("read API key reached the key kit: %d %s", code, body)
	}
	trusted := Who{UserID: 1, Via: "local_network", Role: RoleOwner, Perms: DefaultPerms(RoleOwner)}
	if code, body := gateCode(keyKitRoute, trusted); code != http.StatusForbidden {
		t.Errorf("trusted-network skip reached the key kit: %d %s", code, body)
	}
}
