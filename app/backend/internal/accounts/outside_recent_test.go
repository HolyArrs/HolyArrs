package accounts

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
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
// jellyfinOwner makes the owner "boss" through Jellyfin first-run setup (no
// HolyArrs password), finishes the wizard, and returns that session and a
// fresh Jellyfin sign-in maker for any of the fake server's people.
func jellyfinOwner(t *testing.T, f *fixture) (person, func(name string) person, string) {
	t.Helper()
	ms := fakeSetupServer(t)
	in := func(path, name string, want int) person {
		b, _ := json.Marshal(map[string]string{"url": ms.URL, "name": name, "password": "right"})
		c := client(t)
		resp, out := call(t, c, "POST", f.srv.URL+path, string(b), nil)
		if resp.StatusCode != want {
			t.Fatalf("jellyfin %s %s: %d %s", path, name, resp.StatusCode, out)
		}
		var v struct {
			CSRF string `json:"csrf"`
		}
		jsonOf(t, out, &v)
		return person{c, v.CSRF}
	}
	own := in("/api/v1/auth/setup/media-server/jellyfin", "boss", http.StatusCreated)
	if resp, out := own.do(t, f, http.MethodPost, "/api/v1/auth/setup/done", ``); resp.StatusCode != http.StatusOK {
		t.Fatalf("setup done: %d %s", resp.StatusCode, out)
	}
	return own, func(name string) person { return in("/api/v1/auth/media-server/jellyfin", name, http.StatusOK) }, ms.URL
}

func TestNoPasswordOwnerOutsideSignInCounts(t *testing.T) {
	f := gated(t, Options{})
	own, signIn, _ := jellyfinOwner(t, f)
	jellyfin := func() person { return signIn("boss") }
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

// ownerID is the id of the only owner.
func ownerID(t *testing.T, f *fixture) int64 {
	t.Helper()
	var id int64
	if err := f.s.Read().QueryRow("SELECT id FROM users WHERE role = 'owner'").Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// An API key of a no-password owner never sets the first password, and the
// fresh outside sign-in that may set it is still refused once it is old.
// route: POST /api/v1/auth/password
func TestFirstPasswordNeedsFreshProof(t *testing.T) {
	f := gated(t, Options{})
	_, signIn, _ := jellyfinOwner(t, f)
	fresh := signIn("boss")
	_, out := fresh.do(t, f, http.MethodPost, "/api/v1/auth/keys", `{"label":"script","scopes":["all"]}`)
	var k struct {
		Key string `json:"key"`
	}
	jsonOf(t, out, &k)
	if k.Key == "" {
		t.Fatalf("no key: %s", out)
	}
	if resp, out := call(t, &http.Client{}, "POST", f.srv.URL+"/api/v1/auth/password", `{"new":"`+pw+`"}`, map[string]string{APIKeyHeader: k.Key}); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("API key set the first password: %d %s", resp.StatusCode, out)
	}
	f.clock.add(localRecentFor + time.Minute)
	if resp, out := fresh.do(t, f, http.MethodPost, "/api/v1/auth/password", `{"new":"`+pw+`"}`); resp.StatusCode != http.StatusForbidden || !strings.Contains(out, `"reauth"`) {
		t.Fatalf("old sign-in set the first password: %d %s", resp.StatusCode, out)
	}
	again := signIn("boss")
	if resp, out := again.do(t, f, http.MethodPost, "/api/v1/auth/password", `{"new":"`+pw+`"}`); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("fresh sign-in could not set the first password: %d %s", resp.StatusCode, out)
	}
}

// A no-password admin who must turn on two-factor gets no pass from a fresh
// Jellyfin sign-in, and /auth/me says so.
// route: PUT /api/v1/auth/settings
func TestNoPasswordAdminMustEnrollRefused(t *testing.T) {
	f := gated(t, Options{})
	own, signIn, msURL := jellyfinOwner(t, f)
	if resp, out := own.do(t, f, http.MethodPut, "/api/v1/auth/settings", `{"jellyfin":{"on":true,"url":"`+msURL+`","join":"everyone"},"confirm_everyone":true}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("jellyfin on: %d %s", resp.StatusCode, out)
	}
	signIn("mum")
	var mumID int64
	if err := f.s.Read().QueryRow("SELECT id FROM users WHERE role = 'member'").Scan(&mumID); err != nil {
		t.Fatal(err)
	}
	if resp, out := own.do(t, f, http.MethodPut, "/api/v1/users/"+strconv.FormatInt(mumID, 10), `{"role":"admin"}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("make admin: %d %s", resp.StatusCode, out)
	}
	if resp, out := own.do(t, f, http.MethodPut, "/api/v1/auth/settings", `{"require_two_factor_admins":true}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("require two-factor: %d %s", resp.StatusCode, out)
	}
	mum := signIn("mum")
	if _, me := mum.do(t, f, http.MethodGet, "/api/v1/auth/me", ""); !strings.Contains(me, `"must_two_factor":true`) || !strings.Contains(me, `"recent_proof":false`) {
		t.Fatalf("me: %s", me)
	}
	if resp, out := mum.do(t, f, http.MethodPut, "/api/v1/auth/settings", `{"require_two_factor_admins":false}`); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("admin who must enroll changed sign-in settings: %d %s", resp.StatusCode, out)
	}
	if _, me := own.do(t, f, http.MethodGet, "/api/v1/auth/me", ""); !strings.Contains(me, `"recent_proof":true`) {
		t.Fatalf("owner me: %s", me)
	}
}

// A home visit mapped to a no-password owner, and an OIDC session of that
// owner, never pass.
func TestNoPasswordOwnerOtherWaysRefused(t *testing.T) {
	f := gated(t, Options{})
	own, signIn, _ := jellyfinOwner(t, f)
	localHome(t, f, own, `,"local_person":`+strconv.FormatInt(ownerID(t, f), 10))
	v, me := localVisitor(t, f)
	if !strings.Contains(me, `"via":"local_network"`) {
		t.Fatalf("not a home visit: %s", me)
	}
	if resp, out := v.do(t, f, http.MethodPut, "/api/v1/auth/settings", `{"require_two_factor_admins":false}`); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("home visit changed sign-in settings: %d %s", resp.StatusCode, out)
	}
	if resp, out := v.do(t, f, http.MethodPost, "/api/v1/auth/password", `{"new":"`+pw+`"}`); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("home visit set the first password: %d %s", resp.StatusCode, out)
	}
	// A fresh session whose way in is single sign-on, not Plex, Jellyfin or Emby.
	for _, method := range []string{"oidc", "forward_auth"} {
		p := signIn("boss")
		setSessionAssurance(t, f, "UPDATE sessions SET auth_method = ? WHERE user_id = ? AND created_at = (SELECT max(created_at) FROM sessions)", method, ownerID(t, f))
		if signInSettings(t, f, p) {
			t.Fatalf("a fresh %s session of a no-password owner passed", method)
		}
	}
}

// A no-password owner with a fresh Jellyfin sign-in can link Plex from My
// account: the check inside the write takes the same proof.
// route: POST /api/v1/auth/plex/my-link/finish
func TestNoPasswordOwnerLinksPlex(t *testing.T) {
	p := newPlexPeople(t)
	f := gated(t, Options{PlexURL: p.srv.URL, PlexAppURL: "https://app.example.test"})
	_, signIn, _ := jellyfinOwner(t, f)
	fresh := signIn("boss")
	if resp, out := fresh.do(t, f, http.MethodPost, "/api/v1/auth/plex/my-link/finish", p.start(t, f, fresh, anaPlex)); resp.StatusCode != http.StatusOK {
		t.Fatalf("my-link finish: %d %s", resp.StatusCode, out)
	}
	stale := signIn("boss")
	pin := p.start(t, f, stale, anaPlexNew)
	f.clock.add(localRecentFor + time.Minute)
	if resp, out := stale.do(t, f, http.MethodPost, "/api/v1/auth/plex/my-link/finish", pin); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("stale my-link finish: %d %s", resp.StatusCode, out)
	}
}
