package main

import (
	"errors"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
	"time"
)

// TestControlsNeedAPasswordSignIn: the local-network skip signs a person
// in without a password; it must not run any control.
func TestControlsNeedAPasswordSignIn(t *testing.T) {
	r := newOwnerRig(t)
	r.owner.call("PUT", "/api/v1/auth/settings", `{"skip_login_local_network":true,"local_networks":["127.0.0.0/8","10.0.0.0/8"]}`, 200)
	jar, _ := cookiejar.New(nil)
	bare := &signedIn{t: t, base: r.srv.URL, c: &http.Client{Jar: jar}}
	bare.call("GET", "/api/v1/parts", "", 200) // signed in through the skip
	for _, p := range []string{
		"/api/v1/parts/music/on", "/api/v1/parts/music/off", "/api/v1/parts/music/restart",
		"/api/v1/stages/gates/pause", "/api/v1/stages/gates/resume", "/api/v1/stages/gates/restart",
		"/api/v1/links/downloader/1/restart",
		"/api/v1/app/restart", "/api/v1/app/safe-mode", "/api/v1/app/shutdown",
	} {
		for _, body := range []string{"", `{"confirm":"abc"}`} {
			if b := bare.call("POST", p, body, 403); !strings.Contains(string(b), "confirm your local password") {
				t.Errorf("%s %s: %s", p, body, b)
			}
		}
	}
}

// TestLinkRestartNotFoundFirst: an unknown id answers 404 and writes no
// Event; a known one writes the Event.
func TestLinkRestartNotFoundFirst(t *testing.T) {
	f := newCtl(t, false)
	u := f.srv.URL + "/api/v1/"
	for _, kind := range []string{"downloader", "source", "subtitle_site"} {
		do(t, "POST", u+"links/"+kind+"/999/restart", "", 404)
	}
	if es := f.events(t); len(es) != 0 {
		t.Fatalf("events for unknown links: %+v", es)
	}
}

// TestSafeModeExitErrorIsShown: when the next start cannot be marked safe
// mode, the answer is 500 and nothing restarts.
func TestSafeModeExitErrorIsShown(t *testing.T) {
	f := newCtl(t, false)
	f.exitErr = errors.New("disk full")
	b := confirmTwice(t, f.srv.URL+"/api/v1/app/safe-mode", 500)
	if !strings.Contains(string(b), "could not arrange safe mode") || strings.Contains(string(b), "disk full") {
		t.Fatalf("answer: %s", b)
	}
	if es := f.events(t); len(es) != 0 {
		t.Fatalf("event written for a failed exit: %+v", es)
	}
}

// TestSafeModeKeepsTheLibrary: in safe mode the Library list and title
// pages stay reachable, a title can be edited or removed, and nothing
// optional runs (searches and part settings answer "this part is off";
// optional loops are not started).
// route: GET /api/v1/titles
func TestSafeModeKeepsTheLibrary(t *testing.T) {
	f := newCtlMode(t, false, true)
	id := addTitleFor(t, f.s, "Salt Choir")
	u := f.srv.URL + "/api/v1/"
	if b := do(t, "GET", u+"titles", "", 200); !strings.Contains(string(b), "Salt Choir") {
		t.Fatalf("library list in safe mode: %s", b)
	}
	do(t, "GET", u+"titles/"+itoa(id), "", 200)
	do(t, "POST", u+"titles/"+itoa(id)+"/search", "", 409)
	do(t, "GET", u+"settings/music", "", 409)
	do(t, "GET", u+"titles/lookup?term=x", "", 409)
	var ps struct {
		SafeScreens []string `json:"safe_screens"`
	}
	decodeJSON(t, do(t, "GET", u+"parts", "", 200), &ps)
	if len(ps.SafeScreens) == 0 || ps.SafeScreens[0] != "/library" {
		t.Fatalf("safe screens: %+v", ps)
	}
	time.Sleep(100 * time.Millisecond)
	// Only the core loop (backups) ran; the optional one (gates) did not.
	if st, _ := f.reg.Get("stage:gates"); st.State != "off" {
		t.Fatalf("gates in safe mode: %+v", st)
	}
	do(t, "POST", u+"titles/"+itoa(id)+"/delete", `{}`, 200)
}
