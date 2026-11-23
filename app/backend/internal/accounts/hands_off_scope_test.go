package accounts

import (
	"context"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// The hands-off switch takes the same scope as the part switches: a key
// with only the read or write scope cannot flip it.
func TestHandsOffSwitchScope(t *testing.T) {
	const put = "PUT /api/v1/settings/hands-off"
	if got, want := RouteScope(put), RouteScope("POST /api/v1/parts/{part}/off"); got != want {
		t.Fatalf("%s needs %q, part switches %q", put, got, want)
	}
	cases := []struct {
		scopes []string
		ok     bool
	}{
		{[]string{webserver.ScopeRead}, false},
		{[]string{webserver.ScopeWrite}, false},
		{[]string{webserver.ScopeRead, webserver.ScopeWrite}, false},
		{[]string{webserver.ScopeControl}, true},
	}
	for _, tc := range cases {
		key := webserver.Caller{Via: "api_key", Role: webserver.RoleOwner, Scopes: tc.scopes}
		if got := key.HasScope(RouteScope(put)); got != tc.ok {
			t.Errorf("key with %v: allowed %v, want %v", tc.scopes, got, tc.ok)
		}
	}
}

// A key that can flip the hands-off switch can also read it.
func TestHandsOffReadByControlKey(t *testing.T) {
	const get = "GET /api/v1/settings/hands-off"
	for _, tc := range []struct {
		scopes []string
		ok     bool
	}{
		{[]string{webserver.ScopeRead}, false},
		{[]string{webserver.ScopeControl}, true},
		{[]string{webserver.ScopeAdmin}, true},
	} {
		key := webserver.Caller{Via: "api_key", Role: webserver.RoleOwner, Scopes: tc.scopes}
		if got := routeScopeOK(key, get); got != tc.ok {
			t.Errorf("key with %v: allowed %v, want %v", tc.scopes, got, tc.ok)
		}
	}
}

// route: GET /api/v1/auth/me
// Every signed-in person learns whether hands-off mode is on, so the add
// sheets and title pages can follow it.
func TestMeSaysHandsOff(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	ana := join(t, f, invite(t, f, own, `{"role":"member"}`), "ana")
	if _, me := ana.do(t, f, "GET", "/api/v1/auth/me", ""); !strings.Contains(me, `"hands_off":false`) {
		t.Fatalf("me before: %s", me)
	}
	if err := parts.SetHandsOff(context.Background(), f.s, nil, true, domain.Actor{}); err != nil {
		t.Fatal(err)
	}
	if _, me := ana.do(t, f, "GET", "/api/v1/auth/me", ""); !strings.Contains(me, `"hands_off":true`) {
		t.Fatalf("me after: %s", me)
	}
}
