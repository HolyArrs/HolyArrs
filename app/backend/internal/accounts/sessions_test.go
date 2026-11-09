package accounts

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func signIn(t *testing.T, f *fixture, name string) person {
	t.Helper()
	c := client(t)
	resp, out := call(t, c, "POST", f.srv.URL+"/api/v1/auth/login", `{"name":"`+name+`","password":"`+pw+`"}`,
		map[string]string{"User-Agent": "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 Version/18.0 Mobile/15E148 Safari/604.1"})
	if resp.StatusCode != 200 {
		t.Fatalf("sign in: %d %s", resp.StatusCode, out)
	}
	var v struct {
		CSRF string `json:"csrf"`
	}
	jsonOf(t, out, &v)
	return person{c, v.CSRF}
}

type sessionList struct {
	Sessions []SessionView `json:"sessions"`
}

func sessionsOf(t *testing.T, f *fixture, p person, path string) []SessionView {
	t.Helper()
	resp, out := p.do(t, f, "GET", path, "")
	if resp.StatusCode != 200 {
		t.Fatalf("%s: %d %s", path, resp.StatusCode, out)
	}
	var v sessionList
	jsonOf(t, out, &v)
	return v.Sessions
}

// route: GET /api/v1/auth/sessions
// route: DELETE /api/v1/auth/sessions/{sid}
// route: DELETE /api/v1/auth/sessions
func TestMySessions(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	phone := signIn(t, f, "luis")
	third := signIn(t, f, "luis")
	ss := sessionsOf(t, f, own, "/api/v1/auth/sessions")
	if len(ss) != 3 {
		t.Fatalf("want 3 sessions, got %+v", ss)
	}
	current := 0
	for _, s := range ss {
		if s.Current {
			current++
		}
		if s.ID == "" || len(s.ID) < 16 {
			t.Fatalf("weak id %q", s.ID)
		}
	}
	if current != 1 {
		t.Fatalf("%d sessions marked current", current)
	}
	// The id rotates hourly; the session keeps its public id and is listed once.
	f.clock.add(RotateEvery + time.Minute)
	if after := sessionsOf(t, f, own, "/api/v1/auth/sessions"); len(after) != 3 {
		t.Fatalf("after rotation: %+v", after)
	}
	// End the phone's session from here.
	var phoneID string
	for _, s := range sessionsOf(t, f, phone, "/api/v1/auth/sessions") {
		if s.Current {
			phoneID = s.ID
			if s.Device != "Safari on iPhone" {
				t.Fatalf("device %q", s.Device)
			}
		}
	}
	if resp, _ := own.do(t, f, "DELETE", "/api/v1/auth/sessions/"+phoneID, ""); resp.StatusCode != 204 {
		t.Fatalf("end one: %d", resp.StatusCode)
	}
	if resp, _ := phone.do(t, f, "GET", "/api/v1/titles", ""); resp.StatusCode != 401 {
		t.Fatalf("ended session still works: %d", resp.StatusCode)
	}
	if resp, _ := own.do(t, f, "DELETE", "/api/v1/auth/sessions/"+phoneID, ""); resp.StatusCode != 404 {
		t.Fatalf("end again: %d", resp.StatusCode)
	}
	// Sign out the others: this one stays.
	if resp, out := own.do(t, f, "DELETE", "/api/v1/auth/sessions", ""); resp.StatusCode != 200 || !strings.Contains(out, `"ended":1`) {
		t.Fatalf("others: %d %s", resp.StatusCode, out)
	}
	if resp, _ := third.do(t, f, "GET", "/api/v1/titles", ""); resp.StatusCode != 401 {
		t.Fatalf("other session still works: %d", resp.StatusCode)
	}
	if resp, _ := own.do(t, f, "GET", "/api/v1/titles", ""); resp.StatusCode != 200 {
		t.Fatalf("this session ended: %d", resp.StatusCode)
	}
	// Without the CSRF header the change is refused.
	if resp, _ := call(t, own.c, "DELETE", f.srv.URL+"/api/v1/auth/sessions", "", nil); resp.StatusCode != 403 {
		t.Fatalf("no csrf: %d", resp.StatusCode)
	}
}

// route: GET /api/v1/users/{id}/sessions
// route: DELETE /api/v1/users/{id}/sessions/{sid}
// route: POST /api/v1/users/{id}/sessions/end-all
func TestUserSessionsForOwnerAndAdmins(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	ad := join(t, f, invite(t, f, own, `{"role":"admin"}`), "ana")
	kid := join(t, f, invite(t, f, own, `{"role":"member"}`), "kid")
	kid2 := signIn(t, f, "kid")
	ownerID, adminID, kidID := userID(t, f, "luis"), userID(t, f, "ana"), userID(t, f, "kid")
	if resp, _ := kid.do(t, f, "GET", fmt.Sprintf("/api/v1/users/%d/sessions", kidID), ""); resp.StatusCode != 403 {
		t.Fatalf("member reached the users routes: %d", resp.StatusCode)
	}
	if resp, _ := ad.do(t, f, "GET", fmt.Sprintf("/api/v1/users/%d/sessions", ownerID), ""); resp.StatusCode != 403 {
		t.Fatalf("admin saw the owner's sessions: %d", resp.StatusCode)
	}
	if resp, _ := ad.do(t, f, "POST", fmt.Sprintf("/api/v1/users/%d/sessions/end-all", ownerID), ""); resp.StatusCode != 403 {
		t.Fatalf("admin ended the owner's sessions: %d", resp.StatusCode)
	}
	if resp, _ := ad.do(t, f, "GET", "/api/v1/users/999/sessions", ""); resp.StatusCode != 404 {
		t.Fatalf("no such user: %d", resp.StatusCode)
	}
	ss := sessionsOf(t, f, ad, fmt.Sprintf("/api/v1/users/%d/sessions", kidID))
	if len(ss) != 2 || ss[0].Current || ss[1].Current {
		t.Fatalf("kid sessions: %+v", ss)
	}
	if resp, _ := ad.do(t, f, "DELETE", fmt.Sprintf("/api/v1/users/%d/sessions/%s", kidID, ss[0].ID), ""); resp.StatusCode != 204 {
		t.Fatalf("admin end one: %d", resp.StatusCode)
	}
	if resp, _ := ad.do(t, f, "DELETE", fmt.Sprintf("/api/v1/users/%d/sessions/%s", ownerID, ss[0].ID), ""); resp.StatusCode != 403 {
		t.Fatalf("admin end on owner: %d", resp.StatusCode)
	}
	if left := sessionsOf(t, f, own, fmt.Sprintf("/api/v1/users/%d/sessions", kidID)); len(left) != 1 {
		t.Fatalf("left: %+v", left)
	}
	if resp, out := own.do(t, f, "POST", fmt.Sprintf("/api/v1/users/%d/sessions/end-all", kidID), ""); resp.StatusCode != 200 || !strings.Contains(out, `"ended":1`) {
		t.Fatalf("owner ends all: %d %s", resp.StatusCode, out)
	}
	for _, p := range []person{kid, kid2} {
		if resp, _ := p.do(t, f, "GET", "/api/v1/auth/me", ""); resp.StatusCode != 401 {
			t.Fatalf("kid still signed in: %d", resp.StatusCode)
		}
	}
	// The owner may end an admin's sessions; the admin may see their own.
	if n := len(sessionsOf(t, f, ad, fmt.Sprintf("/api/v1/users/%d/sessions", adminID))); n != 1 {
		t.Fatalf("admin own: %d", n)
	}
	if resp, _ := own.do(t, f, "POST", fmt.Sprintf("/api/v1/users/%d/sessions/end-all", adminID), ""); resp.StatusCode != 200 {
		t.Fatalf("owner ends admin: %d", resp.StatusCode)
	}
	if resp, _ := ad.do(t, f, "GET", "/api/v1/auth/me", ""); resp.StatusCode != 401 {
		t.Fatalf("admin still signed in: %d", resp.StatusCode)
	}
}

func TestDeviceNames(t *testing.T) {
	for ua, want := range map[string]string{
		"": "Unknown device",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:140.0) Gecko/20100101 Firefox/140.0":                                      "Firefox on Windows",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0 Safari/537.36 Edg/140.0": "Edge on Windows",
		"Mozilla/5.0 (Linux; Android 15; Pixel 7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0 Mobile Safari/537.36":     "Chrome on Android",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 14_0) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/605.1.15":    "Safari on Mac",
		"curl/8.9.1":   "curl",
		"strange-tool": "Unknown device",
	} {
		if got := Device(ua); got != want {
			t.Errorf("Device(%q) = %q, want %q", ua, got, want)
		}
	}
}
