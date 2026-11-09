package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/backups"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// TestEveryRouteHasAPermission reads every route the program registers and
// checks accounts.RoutePerms names what it needs (or that it is public). A route
// missing from the table is owner-only, which is safe but usually a mistake.
func TestEveryRouteHasAPermission(t *testing.T) {
	t.Parallel()
	re := regexp.MustCompile(`m(?:ux)?\.Handle(?:Func)?\("([A-Z]+ [^"]*)"(\+RestorePath)?`)
	public := map[string]bool{
		"GET /healthz": true, "GET /readyz": true, "GET /api/v1/auth/state": true, "POST /api/v1/auth/setup": true, "POST /api/v1/auth/login": true,
		"POST /api/v1/auth/login/two-factor": true, "POST /api/v1/auth/plex/start": true, "POST /api/v1/auth/plex/finish": true,
		"POST /api/v1/auth/media-server/{kind}": true, "GET /api/v1/auth/invites/{token}": true, "POST /api/v1/auth/invites/{token}": true,
		"POST /api/v1/auth/setup/plex/start": true, "POST /api/v1/auth/setup/plex/finish": true,
		"POST /api/v1/auth/setup/media-server/{kind}": true, "GET /api/v1/auth/setup/discover": true,
		"POST /api/v1/auth/invites/{token}/plex/start": true, "POST /api/v1/auth/invites/{token}/plex/finish": true,
		"POST /api/v1/auth/invites/{token}/media-server/{kind}": true,
	}
	seen := 0
	for _, dir := range routePackages {
		files, _ := filepath.Glob(filepath.Join(dir, "*.go"))
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") || (dir == "../../internal/arrapi" && filepath.Base(f) != "routes_settings.go") || (dir == "." && filepath.Base(f) == "starting.go") {
				continue
			}
			b, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			for _, m := range re.FindAllStringSubmatch(string(b), -1) {
				p := m[1]
				if m[2] != "" {
					p += backups.RestorePath
				}
				seen++
				if _, ok := accounts.RoutePerms[p]; !ok && !public[p] {
					t.Errorf("%s: route %q has no entry in accounts.RoutePerms", f, p)
				}
			}
		}
	}
	if seen < 50 {
		t.Fatalf("found only %d routes; the pattern no longer matches the code", seen)
	}
}

// TestMemberIsRefusedEveryStaffRoute signs a member in to the real API and
// calls every route that needs more than being signed in: each one says 403.
// route: POST /api/v1/users/invites
// route: POST /api/v1/auth/setup
// route: POST /api/v1/auth/invites/{token}
func TestMemberIsRefusedEveryStaffRoute(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s, err := database.Open(dir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	v, err := secrets.Open(context.Background(), s, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	au := accounts.New(s, accounts.Options{Vault: v})
	srv := httptest.NewServer(buildHandlers(services{Store: s, Metadata: metadata.Lab(), Version: "test", Auth: au, Secrets: v, Live: webserver.NewLiveHub(webserver.LiveOptions{}),
		Backups: &backups.Manager{Store: s, Dir: dir, Version: "test"}, Restart: func() {}}).Handler())
	t.Cleanup(srv.Close)

	signed := func(method, path, body string, c *http.Client, csrf string) (int, string) {
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(accounts.CSRFHeader, csrf)
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	newClient := func() *http.Client {
		jar, _ := cookiejar.New(nil)
		return &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	csrfOf := func(body string) string {
		var v struct {
			CSRF string `json:"csrf"`
			Path string `json:"path"`
		}
		_ = json.Unmarshal([]byte(body), &v)
		return v.CSRF + v.Path
	}
	own := newClient()
	st, body := signed("POST", "/api/v1/auth/setup", `{"name":"luis","password":"a long test passphrase"}`, own, "")
	if st != 201 {
		t.Fatalf("setup %d %s", st, body)
	}
	ownCSRF := csrfOf(body)
	st, body = signed("POST", "/api/v1/users/invites", `{}`, own, ownCSRF)
	if st != 201 {
		t.Fatalf("invite %d %s", st, body)
	}
	link := csrfOf(body)
	mem := newClient()
	st, body = signed("POST", "/api/v1/auth/invites/"+strings.TrimPrefix(link, "/invite/"), `{"name":"ana","password":"a long test passphrase"}`, mem, "")
	if st != 201 {
		t.Fatalf("join %d %s", st, body)
	}
	memCSRF := csrfOf(body)

	fill := strings.NewReplacer("{id}", "1", "{name}", "x.db", "{token}", "x", "{kind}", "jellyfin")
	checked := 0
	for pattern, need := range accounts.RoutePerms {
		method, path, _ := strings.Cut(pattern, " ")
		// Members may ask for titles (Requests), so those routes are theirs.
		if need == "signed_in" || need == "self" || need == webserver.NeedPublic || need == webserver.PermAsk || path == "/" || path == "/login" {
			continue
		}
		// The arr API takes no session at all, only its own keys;
		// TestOldAppMemberKeyRefused checks a member's key gets 403.
		if accounts.ArrSide(path) != "" {
			continue
		}
		st, body := signed(method, fill.Replace(path), "{}", mem, memCSRF)
		if st != http.StatusForbidden {
			t.Errorf("member %s: %d %s", pattern, st, body)
		}
		checked++
	}
	if checked < 40 {
		t.Fatalf("checked only %d routes", checked)
	}
	// And what members may do still works.
	for _, p := range []string{"/api/v1/titles", "/api/v1/auth/me", "/api/v1/titles/lookup?term=x"} {
		if st, body := signed("GET", p, "", mem, memCSRF); st != 200 {
			t.Errorf("member GET %s: %d %s", p, st, body)
		}
	}
}
