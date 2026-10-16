package main

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// The whole app under /holyarrs: pages, files, the API, sign-in redirects
// and the health check live under the base; nothing answers outside it.
func TestServesUnderURLBase(t *testing.T) {
	c, err := parseFlags([]string{"-config", t.TempDir(), "-url-base", "holyarrs/"}, io.Discard)
	if err != nil || c.URLBase != "/holyarrs" {
		t.Fatalf("flag: %q %v", c.URLBase, err)
	}
	if _, err := parseFlags([]string{"-url-base", "/a/../b"}, io.Discard); err == nil {
		t.Fatal("a bad base was taken")
	}
	addr, _, _, _ := runApp(t, c)
	base := "http://" + addr
	cl := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	get := func(p string) (*http.Response, string) {
		t.Helper()
		resp, err := cl.Get(base + p)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp, string(b)
	}
	if r, _ := get("/holyarrs/healthz"); r.StatusCode != 200 {
		t.Fatalf("healthz %d", r.StatusCode)
	}
	for _, p := range []string{"/healthz", "/login", "/api/v1/auth/state"} {
		if r, _ := get(p); r.StatusCode != 404 {
			t.Errorf("%s outside the base: %d", p, r.StatusCode)
		}
	}
	if r, _ := get("/holyarrs"); r.StatusCode != 301 || r.Header.Get("Location") != "/holyarrs/" {
		t.Fatalf("bare base: %d %q", r.StatusCode, r.Header.Get("Location"))
	}
	if r, _ := get("/holyarrs/library"); r.StatusCode != 303 || r.Header.Get("Location") != "/holyarrs/login" {
		t.Fatalf("sign-in redirect: %d %q", r.StatusCode, r.Header.Get("Location"))
	}
	r, page := get("/holyarrs/login")
	if r.StatusCode != 200 || !strings.Contains(page, `base: "/holyarrs"`) || !strings.Contains(page, `"/holyarrs/_app/immutable/`) {
		t.Fatalf("login page: %d", r.StatusCode)
	}
	if r, body := get("/holyarrs/api/v1/auth/state"); r.StatusCode != 200 || !strings.Contains(body, "needs_setup") {
		t.Fatalf("api: %d %s", r.StatusCode, body)
	}
}
