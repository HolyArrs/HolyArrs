package accounts

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// signInSettings tries the risky sign-in settings route and says whether it
// went through, failing on anything but a 200 or a 403 "reauth".
func signInSettings(t *testing.T, f *fixture, p person) bool {
	t.Helper()
	resp, out := p.do(t, f, http.MethodPut, "/api/v1/auth/settings", `{"require_two_factor_admins":false}`)
	switch {
	case resp.StatusCode == http.StatusOK:
		return true
	case resp.StatusCode == http.StatusForbidden && strings.Contains(out, `"reauth"`):
		return false
	}
	t.Fatalf("sign-in settings: %d %s", resp.StatusCode, out)
	return false
}

// importAsks says whether the member import route stops for a reauth.
func importAsks(t *testing.T, f *fixture, p person) bool {
	t.Helper()
	resp, out := p.do(t, f, http.MethodPost, "/api/v1/auth/import/jellyfin", `{"people":[]}`)
	return resp.StatusCode == http.StatusForbidden && strings.Contains(out, `"reauth"`)
}

// An owner made through Jellyfin has no HolyArrs password: a fresh Jellyfin
// sign-in is their proof for risky changes, a stale one is not, and once they
// set a password only that password counts.
func TestNoPasswordOwnerOutsideSignInCounts(t *testing.T) {
	ms := fakeSetupServer(t)
	f := gated(t, Options{})
	b, _ := json.Marshal(map[string]string{"url": ms.URL, "name": "boss", "password": "right"})
	made := false
	jellyfin := func() person {
		c := client(t)
		path, want := "/api/v1/auth/media-server/jellyfin", http.StatusOK
		if !made {
			path, want, made = "/api/v1/auth/setup/media-server/jellyfin", http.StatusCreated, true
		}
		resp, out := call(t, c, "POST", f.srv.URL+path, string(b), nil)
		if resp.StatusCode != want {
			t.Fatalf("jellyfin %s: %d %s", path, resp.StatusCode, out)
		}
		var v struct {
			CSRF string `json:"csrf"`
		}
		jsonOf(t, out, &v)
		return person{c, v.CSRF}
	}
	own := jellyfin()
	if resp, out := own.do(t, f, http.MethodPost, "/api/v1/auth/setup/done", ``); resp.StatusCode != http.StatusOK {
		t.Fatalf("setup done: %d %s", resp.StatusCode, out)
	}
	f.clock.add(localRecentFor + time.Minute)
	if signInSettings(t, f, own) || !importAsks(t, f, own) {
		t.Fatal("a stale Jellyfin sign-in passed without a password")
	}

	fresh := jellyfin()
	if who := meOf(t, f, fresh); !who.OutsideRecent || who.LocalRecent {
		t.Fatalf("fresh Jellyfin sign-in: %+v", who)
	}
	if !signInSettings(t, f, fresh) {
		t.Fatal("a fresh Jellyfin sign-in of a no-password owner was refused")
	}
	if importAsks(t, f, fresh) {
		t.Fatal("member import asked a fresh no-password owner for a password")
	}
	f.clock.add(localRecentFor + time.Minute)
	if signInSettings(t, f, fresh) {
		t.Fatal("a Jellyfin sign-in still counted after 10 minutes")
	}

	// With a password set, only the password counts.
	again := jellyfin()
	if resp, out := again.do(t, f, http.MethodPost, "/api/v1/auth/password", `{"new":"`+pw+`"}`); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("first password: %d %s", resp.StatusCode, out)
	}
	after := jellyfin()
	if who := meOf(t, f, after); !who.OutsideRecent {
		t.Fatalf("Jellyfin sign-in after the password: %+v", who)
	}
	if signInSettings(t, f, after) || !importAsks(t, f, after) {
		t.Fatal("a Jellyfin sign-in passed for an owner who has a password")
	}
}

// The outside proof needs a browser session of a person with no password and
// no unfinished required two-factor enrollment.
func TestRecentLocalOutsideLimits(t *testing.T) {
	ok := func(who webserver.Caller) bool {
		r := (&http.Request{}).WithContext(webserver.ContextWith(context.Background(), who))
		return RecentLocal(r)
	}
	if !ok(webserver.Caller{Via: "session", OutsideRecent: true, NoPassword: true}) {
		t.Fatal("no-password outside sign-in refused")
	}
	if ok(webserver.Caller{Via: "session", OutsideRecent: true}) {
		t.Fatal("outside sign-in passed for someone with a password")
	}
	if ok(webserver.Caller{Via: "session", NoPassword: true}) {
		t.Fatal("stale outside sign-in passed")
	}
	if ok(webserver.Caller{Via: "session", OutsideRecent: true, NoPassword: true, MustTwoFactor: true}) {
		t.Fatal("outside sign-in skipped a required two-factor enrollment")
	}
	if ok(webserver.Caller{Via: "local_network", OutsideRecent: true, NoPassword: true}) {
		t.Fatal("outside mark worked for a home-network visit")
	}
	if ok(webserver.Caller{Via: "api_key", OutsideRecent: true, NoPassword: true}) {
		t.Fatal("outside mark worked for an API key")
	}
	if !needsRecentLocal("PUT /api/v1/auth/settings", webserver.Caller{Via: "local_network", OutsideRecent: true, NoPassword: true}) {
		t.Fatal("a home-network visit reached sign-in settings")
	}
}
