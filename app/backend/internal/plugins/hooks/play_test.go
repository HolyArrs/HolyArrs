package hooks

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// playRig is a plugin "player" with a page that may stream file 7.
func playRig(t *testing.T) (*rig, map[string][]string) {
	t.Helper()
	movie := filepath.Join(t.TempDir(), "dune.mkv")
	if err := os.WriteFile(movie, []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	grants := map[string][]string{"player": {"media.stream", "pages.add"}}
	g := newRig(t, grants)
	g.h.StreamFile = func(_ context.Context, pl string, id int64) (string, error) {
		if pl == "player" && id == 7 {
			return movie, nil
		}
		return "", errors.New("this file is not in a folder the plugin was given")
	}
	g.h.AsPlugin = func(string, *http.Request) (*http.Response, error) { return nil, errors.New("not used") }
	g.h.BridgeAllowed = func(pl string, who webserver.Caller, r *http.Request) error {
		if who.Role != webserver.RoleOwner {
			return errors.New("your account may not do this; ask the owner")
		}
		return nil
	}
	if w := g.do(plug("player"), "PUT", "/api/v1/plugin-hooks/pages", `[{"id":"p","title":"P","file":"index.html"}]`); w.Code != http.StatusNoContent {
		t.Fatal(w.Code, w.Body)
	}
	return g, grants
}

func TestPlayRefusals(t *testing.T) {
	g, grants := playRig(t)
	kid := &webserver.Caller{Via: "session", UserID: 5, Role: webserver.RoleOwner, MaxRating: "PG"}
	member := &webserver.Caller{Via: "session", UserID: 6, Role: webserver.RoleMember}
	for _, c := range []struct {
		name   string
		who    *webserver.Caller
		plugin string
		body   string
		code   int
	}{
		{"the owner, a granted file", owner, "player", `{"file_id":7}`, 200},
		{"a file outside the granted folders", owner, "player", `{"file_id":8}`, http.StatusForbidden},
		{"no file", owner, "player", `{}`, http.StatusBadRequest},
		{"another person without the right", member, "player", `{"file_id":7}`, http.StatusForbidden},
		{"a person with an age limit", kid, "player", `{"file_id":7}`, http.StatusForbidden},
		{"an API key", &webserver.Caller{Via: "api_key", KeyID: 2, UserID: 1, Role: "owner"}, "player", `{"file_id":7}`, http.StatusForbidden},
		{"a plugin itself", plug("player"), "player", `{"file_id":7}`, http.StatusForbidden},
		{"nobody signed in", nil, "player", `{"file_id":7}`, http.StatusForbidden},
		{"a plugin with no page", owner, "ghost", `{"file_id":7}`, http.StatusNotFound},
	} {
		if w := g.do(c.who, "POST", "/api/v1/plugin-pages/"+c.plugin+"/play", c.body); w.Code != c.code {
			t.Errorf("%s: %d, want %d: %s", c.name, w.Code, c.code, w.Body)
		}
	}
	grants["player"] = []string{"pages.add"}
	if w := g.do(owner, "POST", "/api/v1/plugin-pages/player/play", `{"file_id":7}`); w.Code != http.StatusForbidden {
		t.Errorf("a plugin without media.stream: %d", w.Code)
	}
}

// The link plays only for the person's own session, reached directly; a
// sandboxed page (opaque origin, no cookie) or a ?page= guess never plays it.
func TestPlayLinkIsNotAcceptedFromAPage(t *testing.T) {
	g, _ := playRig(t)
	w := g.do(owner, "POST", "/api/v1/plugin-pages/player/play", `{"file_id":7}`)
	var out struct{ URL string }
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if !strings.HasPrefix(out.URL, "/api/v1/plugin-stream/") {
		t.Fatalf("link %q", out.URL)
	}
	get := func(url string, who *webserver.Caller, h map[string]string) int {
		r := httptest.NewRequest("GET", url, nil)
		r.Header.Set("Range", "bytes=2-4")
		for k, v := range h {
			r.Header.Set(k, v)
		}
		if who != nil {
			r = r.WithContext(webserver.ContextWith(r.Context(), *who))
		}
		rec := httptest.NewRecorder()
		g.mux.ServeHTTP(rec, r)
		return rec.Code
	}
	page := map[string]string{"Sec-Fetch-Dest": "video", "Sec-Fetch-Site": "cross-site", "Origin": "null"}
	for name, c := range map[string]struct {
		url string
		who *webserver.Caller
		h   map[string]string
	}{
		"the page, no cookie":              {out.URL, nil, page},
		"the page with a page binding":     {out.URL + "?page=hms_guess", nil, page},
		"the page with a binding, headers": {out.URL + "?page=hms_guess", nil, nil},
		"the person behind a proxy":        {out.URL, owner, map[string]string{"X-Forwarded-For": "203.0.113.9"}},
		"the person's session in a frame":  {out.URL, owner, map[string]string{"Sec-Fetch-Dest": "iframe"}},
	} {
		if code := get(c.url, c.who, c.h); code != http.StatusNotFound {
			t.Errorf("%s: %d", name, code)
		}
	}
	if code := get(out.URL, owner, map[string]string{"Sec-Fetch-Dest": "video"}); code != http.StatusPartialContent {
		t.Errorf("the app's own player: %d", code)
	}
}

// The page can't get a link or ask for one through the bridge.
func TestBridgeNeverHandsAPageALink(t *testing.T) {
	g, _ := playRig(t)
	for _, path := range []string{"/api/v1/plugin-hooks/stream", "/api/v1/plugin-stream/hms_x", "/api/v1/plugin-pages/player/play"} {
		body := `{"method":"POST","path":"` + path + `","body":{"file_id":7}}`
		if w := g.do(owner, "POST", "/api/v1/plugin-pages/player/call", body); w.Code != http.StatusNotFound || strings.Contains(w.Body.String(), "plugin-stream/hms_") {
			t.Errorf("%s: %d %s", path, w.Code, w.Body)
		}
	}
}

// The page's own policy lets it reach nothing outside HolyArrs: no outside
// host in any directive, no connections, no forms.
func TestPageCSPReachesNothingOutside(t *testing.T) {
	for _, d := range strings.Split(PageCSP, ";") {
		d = strings.TrimSpace(d)
		if strings.Contains(d, "http") || strings.Contains(d, "*") || strings.Contains(d, "//") {
			t.Errorf("directive reaches outside: %q", d)
		}
	}
	for _, want := range []string{"sandbox allow-scripts allow-forms", "default-src 'none'", "connect-src 'none'", "form-action 'none'", "base-uri 'none'", "frame-ancestors 'self'"} {
		if !strings.Contains(PageCSP, want) {
			t.Errorf("policy lacks %s", want)
		}
	}
	if strings.Contains(PageCSP, "allow-same-origin") || strings.Contains(PageCSP, "allow-popups") || strings.Contains(PageCSP, "allow-top-navigation") {
		t.Error("the sandbox is too open")
	}
}
