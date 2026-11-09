package accounts

import (
	"net/http"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

const keyKitRoute = "POST /api/v1/system/key-kit"

// The key kit is the master key itself: the owner only, with a fresh local
// password (and two-factor code when enrolled), never a read key, never a
// stale session, never an admin.
func TestKeyKitRoutePolicy(t *testing.T) {
	if need, ok := RoutePerms[keyKitRoute]; !ok || need != webserver.NeedOwner {
		t.Fatalf("RoutePerms[%q] = %q, %v; want owner only", keyKitRoute, need, ok)
	}
	if !riskyRoutes[keyKitRoute] {
		t.Fatalf("%s must ask for a fresh local password", keyKitRoute)
	}
	if ReadRoutes[keyKitRoute] {
		t.Fatalf("%s must not be open to read keys", keyKitRoute)
	}
	if sc := RouteScope(keyKitRoute); sc == webserver.ScopeRead || sc == webserver.ScopeHealthRead || sc == webserver.ScopeWrite {
		t.Fatalf("%s scope %q is too wide", keyKitRoute, sc)
	}

	owner := webserver.Caller{UserID: 1, Via: "session", Role: webserver.RoleOwner, Perms: webserver.DefaultPerms(webserver.RoleOwner)}
	if code, body := gateCode(keyKitRoute, owner); code != http.StatusForbidden || !strings.Contains(body, `"reauth"`) {
		t.Errorf("stale owner session: %d %s; want 403 reauth", code, body)
	}
	fresh := owner
	fresh.LocalRecent = true
	if code, body := gateCode(keyKitRoute, fresh); code != http.StatusTeapot {
		t.Errorf("fresh owner session refused: %d %s", code, body)
	}
	admin := webserver.Caller{UserID: 2, Via: "session", Role: webserver.RoleAdmin, Perms: webserver.DefaultPerms(webserver.RoleAdmin), LocalRecent: true, TwoFactor: true}
	if code, body := gateCode(keyKitRoute, admin); code != http.StatusForbidden {
		t.Errorf("fresh admin session reached the key kit: %d %s", code, body)
	}
	readKey := webserver.Caller{UserID: 1, Via: "api_key", Role: webserver.RoleOwner, Perms: webserver.DefaultPerms(webserver.RoleOwner), Scopes: []string{webserver.ScopeRead}}
	if code, body := gateCode(keyKitRoute, readKey); code != http.StatusForbidden {
		t.Errorf("read API key reached the key kit: %d %s", code, body)
	}
	trusted := webserver.Caller{UserID: 1, Via: "local_network", Role: webserver.RoleOwner, Perms: webserver.DefaultPerms(webserver.RoleOwner)}
	if code, body := gateCode(keyKitRoute, trusted); code != http.StatusForbidden {
		t.Errorf("trusted-network skip reached the key kit: %d %s", code, body)
	}
}
