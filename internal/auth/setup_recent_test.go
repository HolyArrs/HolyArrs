package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// newKey tries the risky "make an API key" route and says whether it went
// through, failing on anything but a 201 or a 403 "reauth".
func newKey(t *testing.T, f *fixture, p person) bool {
	t.Helper()
	resp, out := p.do(t, f, http.MethodPost, "/api/v1/auth/keys", `{"label":"script"}`)
	switch {
	case resp.StatusCode == http.StatusCreated:
		return true
	case resp.StatusCode == http.StatusForbidden && strings.Contains(out, `"reauth"`):
		return false
	}
	t.Fatalf("new API key: %d %s", resp.StatusCode, out)
	return false
}

// Luis, 2026-11-05 ("No check during setup"): the session first-run setup
// starts passes the recent-password check for an hour; nothing else does.
func TestSetupSessionCountsAsRecentForAnHour(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	if who := meOf(t, f, own); !who.SetupRecent || !who.LocalRecent {
		t.Fatalf("fresh setup session: setup %v local %v", who.SetupRecent, who.LocalRecent)
	}
	f.clock.add(localRecentFor + time.Minute)
	if who := meOf(t, f, own); !who.SetupRecent || who.LocalRecent {
		t.Fatalf("setup session at 11 min: setup %v local %v", who.SetupRecent, who.LocalRecent)
	}
	if !newKey(t, f, own) {
		t.Fatal("setup session asked for the password inside its hour")
	}

	// A normal password sign-in is never a setup session.
	signed := signIn(t, f, "luis")
	if who := meOf(t, f, signed); who.SetupRecent {
		t.Fatal("a password sign-in counted as setup")
	}
	// An invite join starts a session much like setup's; it is not one either.
	ana := join(t, f, invite(t, f, own, `{"role":"admin"}`), "ana")
	if who := meOf(t, f, ana); who.SetupRecent {
		t.Fatal("an invite join counted as setup")
	}
	f.clock.add(localRecentFor + time.Minute)
	if newKey(t, f, signed) {
		t.Fatal("a stale password sign-in made a key without the password")
	}
	if newKey(t, f, ana) {
		t.Fatal("a stale invite session made a key without the password")
	}
	if !newKey(t, f, own) {
		t.Fatal("setup session asked for the password at 22 min")
	}

	// The hour is over (the session has rotated by now, too).
	f.clock.add(setupRecentFor - 2*(localRecentFor+time.Minute))
	if who := meOf(t, f, own); who.SetupRecent {
		t.Fatal("setup session still counted at exactly one hour")
	}
	if newKey(t, f, own) {
		t.Fatal("setup session made a key after its hour")
	}
}

// A reauth replaces the setup session with an ordinary one.
func TestSetupMarkEndsWithReauth(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	resp, out := own.do(t, f, http.MethodPost, "/api/v1/auth/reauth", `{"password":"`+pw+`"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reauth: %d %s", resp.StatusCode, out)
	}
	var v struct {
		CSRF string `json:"csrf"`
	}
	jsonOf(t, out, &v)
	own.csrf = v.CSRF
	f.clock.add(localRecentFor + time.Minute)
	if who := meOf(t, f, own); who.SetupRecent || who.LocalRecent {
		t.Fatalf("after reauth: setup %v local %v", who.SetupRecent, who.LocalRecent)
	}
	if newKey(t, f, own) {
		t.Fatal("a stale reauth session made a key")
	}
}

// An owner made through Jellyfin has no password to confirm: the setup
// session lets the first-run steps through; a later Jellyfin sign-in does not.
func TestOutsideSetupSessionCountsAsRecent(t *testing.T) {
	ms := fakeSetupServer(t)
	f := gated(t, Options{})
	b, _ := json.Marshal(map[string]string{"url": ms.URL, "name": "boss", "password": "right"})
	c := client(t)
	resp, out := call(t, c, "POST", f.srv.URL+"/api/v1/auth/setup/media-server/jellyfin", string(b), nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("setup: %d %s", resp.StatusCode, out)
	}
	var v struct {
		CSRF string `json:"csrf"`
	}
	jsonOf(t, out, &v)
	own := person{c, v.CSRF}
	if who := meOf(t, f, own); !who.SetupRecent || who.LocalRecent || who.AuthMethod != ProviderJellyfin {
		t.Fatalf("outside setup session: %+v", who)
	}

	c2 := client(t)
	resp, out = call(t, c2, "POST", f.srv.URL+"/api/v1/auth/media-server/jellyfin", string(b), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("sign-in: %d %s", resp.StatusCode, out)
	}
	jsonOf(t, out, &v)
	later := person{c2, v.CSRF}
	if who := meOf(t, f, later); who.SetupRecent {
		t.Fatal("a later Jellyfin sign-in counted as setup")
	}
	if newKey(t, f, later) {
		t.Fatal("a Jellyfin sign-in made a key without a password")
	}
	if !newKey(t, f, own) {
		t.Fatal("the Jellyfin setup session asked for a password")
	}
	f.clock.add(setupRecentFor)
	if newKey(t, f, own) {
		t.Fatal("the Jellyfin setup session made a key after its hour")
	}
}

// A required two-factor enrollment still wins over the setup mark, and only
// a browser session counts.
func TestRecentLocalSetupMarkLimits(t *testing.T) {
	ok := func(who Who) bool {
		r := (&http.Request{}).WithContext(ContextWith(context.Background(), who))
		return RecentLocal(r)
	}
	if !ok(Who{Via: "session", SetupRecent: true}) {
		t.Fatal("setup session refused")
	}
	if ok(Who{Via: "session", SetupRecent: true, MustTwoFactor: true}) {
		t.Fatal("setup mark skipped a required two-factor enrollment")
	}
	if ok(Who{Via: "api_key", SetupRecent: true}) {
		t.Fatal("setup mark worked for an API key")
	}
}
