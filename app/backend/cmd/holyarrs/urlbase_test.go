package main

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// The whole app under /holyarrs: pages, files, the API, sign-in redirects
// and the health check live under the base; outside it only the root
// /healthz and /readyz answer.
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
	// The health check always calls the root, so /healthz and /readyz also
	// answer there under a base set by flag; nothing else does.
	for _, p := range []string{"/healthz", "/readyz"} {
		if r, _ := get(p); r.StatusCode != 200 {
			t.Errorf("%s at the root under a base: %d, want 200", p, r.StatusCode)
		}
	}
	for _, p := range []string{"/login", "/api/v1/auth/state", "/healthzx", "/healthz/"} {
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

// A base saved in Settings > This app is used from the next start; the
// HOLYARRS_URL_BASE variable wins over it (B3, must be 100%).
func TestSavedURLBaseAppliesAtNextStartAndEnvWins(t *testing.T) {
	dir := t.TempDir()
	s, err := database.Open(dir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	now := database.FormatTime(time.Now())
	if err := s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('this_app', 'url_base', '"/saved"', ?, '"test"', ?, '"test"')`, now, now)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	status := func(addr, p string) int {
		t.Helper()
		cl := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		r, err := cl.Get("http://" + addr + p)
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		return r.StatusCode
	}

	// The saved base is used at start.
	c, err := parseFlags([]string{"-config", dir}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	addr, _, _, stop := runApp(t, c)
	if got := status(addr, "/saved/api/v1/auth/state"); got != 200 {
		t.Fatalf("saved base: %d", got)
	}
	if got := status(addr, "/api/v1/auth/state"); got != 404 {
		t.Fatalf("root under a saved base: %d", got)
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}

	// The variable wins over the saved base.
	t.Setenv("HOLYARRS_URL_BASE", "/fromenv")
	c, err = parseFlags([]string{"-config", dir}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	addr, _, _, _ = runApp(t, c)
	if got := status(addr, "/fromenv/api/v1/auth/state"); got != 200 {
		t.Fatalf("variable base: %d", got)
	}
	if got := status(addr, "/saved/api/v1/auth/state"); got != 404 {
		t.Fatalf("saved base while the variable is set: %d", got)
	}
}
