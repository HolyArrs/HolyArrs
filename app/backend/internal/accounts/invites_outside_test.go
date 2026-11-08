package accounts

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// inviteOpenNow says whether the invite link still opens.
func inviteOpenNow(t *testing.T, f *fixture, tok string) bool {
	t.Helper()
	resp, _ := call(t, client(t), "GET", f.srv.URL+"/api/v1/auth/invites/"+tok, "", nil)
	return resp.StatusCode == 200
}

// route: POST /api/v1/auth/invites/{token}/media-server/{kind}
// Joining through an invite with Jellyfin or Emby: the invite is the
// permission (the join rule is nobody), the user gets the invite's role and
// is linked, and an account already linked leaves the invite unused.
func TestInviteJoinWithMediaServer(t *testing.T) {
	for _, kind := range []string{"jellyfin", "emby"} {
		t.Run(kind, func(t *testing.T) {
			var logouts atomic.Int32
			ms := fakeMediaServer(t, &logouts)
			f := gated(t, Options{})
			own := ownerOf(t, f)
			join := func(tok, name, pass string) (*http.Client, int, string) {
				c := client(t)
				b, _ := json.Marshal(map[string]string{"name": name, "password": pass})
				resp, out := call(t, c, "POST", f.srv.URL+"/api/v1/auth/invites/"+tok+"/media-server/"+kind, string(b), nil)
				return c, resp.StatusCode, out
			}
			tok := invite(t, f, own, `{"role":"member","perms":["ask","auto_approve"]}`)
			if _, code, _ := join(tok, "mum", "right"); code != 404 {
				t.Fatalf("while the way is off: %d", code)
			}
			if resp, out := own.do(t, f, "PUT", "/api/v1/auth/settings", `{"`+kind+`":{"on":true,"url":"`+ms.URL+`","join":"nobody"}}`); resp.StatusCode != 200 {
				t.Fatalf("way on: %d %s", resp.StatusCode, out)
			}
			if _, code, _ := join(tok, "mum", "wrong"); code != 401 {
				t.Fatalf("wrong password: %d", code)
			}
			if _, code, _ := join(tok, "off", "right"); code != 403 {
				t.Fatalf("turned off on the server: %d", code)
			}
			if !inviteOpenNow(t, f, tok) {
				t.Fatal("a failed join used the invite")
			}
			c, code, out := join(tok, "mum", "right")
			if code != 200 {
				t.Fatalf("join: %d %s", code, out)
			}
			_, me := call(t, c, "GET", f.srv.URL+"/api/v1/auth/me", "", nil)
			if !strings.Contains(me, `"name":"Mum"`) || !strings.Contains(me, `"role":"member"`) || !strings.Contains(me, `"perms":["ask","auto_approve"]`) {
				t.Fatalf("me: %s", me)
			}
			if inviteOpenNow(t, f, tok) {
				t.Fatal("invite still open after the join")
			}
			// The used invite does nothing more, even for someone new.
			if _, code, _ := join(tok, "kid", "right"); code != 404 {
				t.Fatalf("used invite: %d", code)
			}
			// The linked person signs in the usual way, though nobody may join.
			lc := client(t)
			if resp, _ := call(t, lc, "POST", f.srv.URL+"/api/v1/auth/media-server/"+kind, `{"name":"mum","password":"right"}`, nil); resp.StatusCode != 200 {
				t.Fatalf("linked sign-in: %d", resp.StatusCode)
			}
			// Already linked: a second invite is left unused.
			tok2 := invite(t, f, own, `{}`)
			_, code, out = join(tok2, "mum", "right")
			if code != 409 || !strings.Contains(out, `"code":"linked"`) {
				t.Fatalf("already linked: %d %s", code, out)
			}
			if !inviteOpenNow(t, f, tok2) {
				t.Fatal("already-linked join used the invite")
			}
			var n int
			_ = f.s.Read().QueryRow("SELECT count(*) FROM users").Scan(&n)
			if n != 2 {
				t.Fatalf("%d users, want owner and Mum", n)
			}
		})
	}
}

// An invite with an age limit makes a Kid however the person joins, and a
// server-side limit does not change what the invite gives.
func TestInviteJoinKeepsKidRating(t *testing.T) {
	var logouts atomic.Int32
	ms := fakeMediaServer(t, &logouts)
	f := gated(t, Options{})
	own := ownerOf(t, f)
	own.do(t, f, "PUT", "/api/v1/auth/settings", `{"jellyfin":{"on":true,"url":"`+ms.URL+`"}}`)
	tok := invite(t, f, own, `{"max_rating":"PG-13"}`)
	c := client(t)
	resp, out := call(t, c, "POST", f.srv.URL+"/api/v1/auth/invites/"+tok+"/media-server/jellyfin", `{"name":"kid","password":"right"}`, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("join: %d %s", resp.StatusCode, out)
	}
	if _, me := call(t, c, "GET", f.srv.URL+"/api/v1/auth/me", "", nil); !strings.Contains(me, `"max_rating":"PG-13"`) || !strings.Contains(me, `"perms":["ask"]`) {
		t.Fatalf("me: %s", me)
	}
}

// invitePlex runs the invite's Plex PIN steps on c and returns the finish answer.
func invitePlex(t *testing.T, f *fixture, p *fakePlex, c *http.Client, tok string) (int, string) {
	t.Helper()
	p.calls.Store(0)
	p.confirmed.Store(false)
	base := f.srv.URL + "/api/v1/auth/invites/" + tok + "/plex/"
	resp, out := call(t, c, "POST", base+"start", "", nil)
	if resp.StatusCode != 200 || !strings.Contains(out, "ABCD") {
		return resp.StatusCode, out
	}
	if resp, _ := call(t, c, "POST", base+"finish", `{"id":77}`, nil); resp.StatusCode != 202 {
		t.Fatalf("waiting: %d", resp.StatusCode)
	}
	f.clock.add(2 * time.Second)
	p.confirmed.Store(true)
	resp, out = call(t, c, "POST", base+"finish", `{"id":77}`, nil)
	return resp.StatusCode, out
}

// route: POST /api/v1/auth/invites/{token}/plex/start
// route: POST /api/v1/auth/invites/{token}/plex/finish
// Joining through an invite with Plex uses the same PIN steps; the join rule
// (nobody here, and a server the person does not share) does not block it.
func TestInviteJoinWithPlex(t *testing.T) {
	p := newFakePlex(t)
	p.restricted.Store(true) // the invite decides the limits, not Plex
	f := gated(t, Options{PlexURL: p.srv.URL, PlexAppURL: "https://app.example.test"})
	own := ownerOf(t, f)
	tok := invite(t, f, own, `{"role":"member"}`)
	if code, _ := invitePlex(t, f, p, client(t), tok); code != 404 {
		t.Fatalf("plex while off: %d", code)
	}
	own.do(t, f, "PUT", "/api/v1/auth/settings", `{"plex":{"on":true,"server_id":"other-server","join":"nobody"}}`)

	// A PIN started for an ordinary sign-in cannot finish an invite, and the
	// other way round; a PIN started for one invite cannot finish another.
	c := client(t)
	p.calls.Store(0)
	call(t, c, "POST", f.srv.URL+"/api/v1/auth/plex/start", "", nil)
	if resp, _ := call(t, c, "POST", f.srv.URL+"/api/v1/auth/invites/"+tok+"/plex/finish", `{"id":77}`, nil); resp.StatusCode != 404 {
		t.Fatalf("sign-in pin finished an invite: %d", resp.StatusCode)
	}
	other := invite(t, f, own, `{}`)
	p.calls.Store(0)
	call(t, c, "POST", f.srv.URL+"/api/v1/auth/invites/"+other+"/plex/start", "", nil)
	if resp, _ := call(t, c, "POST", f.srv.URL+"/api/v1/auth/invites/"+tok+"/plex/finish", `{"id":77}`, nil); resp.StatusCode != 404 {
		t.Fatalf("one invite's pin finished another: %d", resp.StatusCode)
	}
	if resp, _ := call(t, c, "POST", f.srv.URL+"/api/v1/auth/plex/finish", `{"id":77}`, nil); resp.StatusCode != 404 {
		t.Fatalf("invite pin signed in: %d", resp.StatusCode)
	}
	// Another browser cannot finish this one's pin (no pin cookie).
	p.calls.Store(0)
	call(t, c, "POST", f.srv.URL+"/api/v1/auth/invites/"+tok+"/plex/start", "", nil)
	if resp, _ := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/invites/"+tok+"/plex/finish", `{"id":77}`, nil); resp.StatusCode != 404 {
		t.Fatalf("pin finished without its cookie: %d", resp.StatusCode)
	}

	c = client(t)
	code, out := invitePlex(t, f, p, c, tok)
	if code != 200 {
		t.Fatalf("plex join: %d %s", code, out)
	}
	_, me := call(t, c, "GET", f.srv.URL+"/api/v1/auth/me", "", nil)
	if !strings.Contains(me, `"name":"kidplex"`) || !strings.Contains(me, `"role":"member"`) || !strings.Contains(me, `"max_rating":""`) {
		t.Fatalf("me: %s", me)
	}
	if inviteOpenNow(t, f, tok) {
		t.Fatal("invite still open after the Plex join")
	}
	// The pin is used up.
	if resp, _ := call(t, c, "POST", f.srv.URL+"/api/v1/auth/invites/"+tok+"/plex/finish", `{"id":77}`, nil); resp.StatusCode != 404 {
		t.Fatalf("pin reused: %d", resp.StatusCode)
	}
	// Already linked: the other invite stays unused.
	code, out = invitePlex(t, f, p, client(t), other)
	if code != 409 || !strings.Contains(out, "Sign in instead") {
		t.Fatalf("already linked: %d %s", code, out)
	}
	if !inviteOpenNow(t, f, other) {
		t.Fatal("already-linked Plex join used the invite")
	}
}

// Used, ended and wrong invites answer as the password join does, and wrong
// tokens count toward the same back-off.
func TestInviteJoinGoneInvites(t *testing.T) {
	var logouts atomic.Int32
	ms := fakeMediaServer(t, &logouts)
	p := newFakePlex(t)
	f := gated(t, Options{PlexURL: p.srv.URL})
	own := ownerOf(t, f)
	own.do(t, f, "PUT", "/api/v1/auth/settings", `{"plex":{"on":true},"emby":{"on":true,"url":"`+ms.URL+`"}}`)
	used := invite(t, f, own, `{}`)
	join(t, f, used, "ana")
	ended := invite(t, f, own, `{"days":1}`)
	f.clock.add(25 * time.Hour)
	for _, tok := range []string{used, ended, "wrong"} {
		f.clock.add(MaxBackoff + time.Minute) // each check starts with no wait from the last
		if resp, _ := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/invites/"+tok+"/media-server/emby", `{"name":"mum","password":"right"}`, nil); resp.StatusCode != 404 {
			t.Fatalf("%s invite, emby: %d", tok, resp.StatusCode)
		}
		if resp, _ := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/invites/"+tok+"/plex/start", "", nil); resp.StatusCode != 404 {
			t.Fatalf("%s invite, plex: %d", tok, resp.StatusCode)
		}
	}
	var n int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM user_links").Scan(&n)
	if n != 0 {
		t.Fatalf("%d links made from gone invites", n)
	}
	// Guessing through the new calls slows down like the old one.
	f.clock.add(MaxBackoff + time.Minute)
	c := client(t)
	var last int
	for range 8 {
		resp, _ := call(t, c, "POST", f.srv.URL+"/api/v1/auth/invites/guess/plex/start", "", nil)
		last = resp.StatusCode
		f.clock.add(10 * time.Millisecond)
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("guessing not slowed: %d", last)
	}
}
