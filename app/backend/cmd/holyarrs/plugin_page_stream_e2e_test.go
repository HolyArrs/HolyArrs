package main

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/plugins/hooks"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

// playerPage is the plugin page the tests open: it asks the app's player to play
// file 7 and writes every message it gets back, so the test can read what the
// page was told.
const playerPage = `<!doctype html><meta charset="utf-8"><title>Player</title>
<pre id="out">[]</pre>
<script>
var log = [];
addEventListener('message', function (e) {
  log.push(e.data);
  document.getElementById('out').textContent = JSON.stringify(log);
});
parent.postMessage({ id: 1, player: 'play', file_id: 7 }, '*');
</script>`

// pageApp is a real HolyArrs router with a stub "player" plugin that has one
// page and may stream file 7, a 4 KB file.
type pageApp struct {
	srv  *httptest.Server
	reg  *hooks.Registry
	file string
}

func startPageApp(t *testing.T, ln net.Listener) *pageApp {
	t.Helper()
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	v, err := secrets.Open(ctx, st, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	movie := filepath.Join(t.TempDir(), "dune.mkv")
	body := make([]byte, 4096)
	for i := range body {
		body[i] = byte(i)
	}
	if err := os.WriteFile(movie, body, 0o600); err != nil {
		t.Fatal(err)
	}
	reg := newPluginHooks()
	t.Cleanup(func() { sources.PluginTransport = nil })
	hub := webserver.NewLiveHub(webserver.LiveOptions{})
	hub.Tap(reg.Publish)
	au := accounts.New(st, accounts.Options{Vault: v})
	a := buildHandlers(services{Store: st, Queue: jobs.New(st, jobs.Options{Workers: 1}), Metadata: metadata.Lab(), Version: "test", Auth: au, Secrets: v, Live: hub, PluginHooks: reg,
		PluginStreamFile: func(_ context.Context, pl string, id int64) (string, error) {
			if pl == "player" && id == 7 {
				return movie, nil
			}
			return "", errors.New("this file is not in a folder the plugin was given")
		}})
	srv := httptest.NewUnstartedServer(a.Handler())
	if ln != nil {
		_ = srv.Listener.Close()
		srv.Listener = ln
	}
	srv.Start()
	t.Cleanup(srv.Close)
	t.Cleanup(cancel) // the stub plugin stops before the server closes
	tok := accounts.IssuePluginToken("player", []string{"pages.add", "media.stream"})
	t.Cleanup(func() { accounts.RevokePluginToken("player"); reg.Drop("player") })
	p := stubPlugin{t: t, base: srv.URL, tok: tok}
	if code, b := p.do("PUT", "/api/v1/plugin-hooks/pages", []hooks.Page{{ID: "player", Title: "Player", File: "index.html"}}); code != http.StatusNoContent {
		t.Fatalf("pages: %d %s", code, b)
	}
	p.serve(ctx, "pages", func(hooks.Job) any { return hooks.FeedAnswer{Status: 200, Body: []byte(playerPage)} })
	waitProvider(t, reg, hooks.Pages, "player")
	return &pageApp{srv: srv, reg: reg, file: movie}
}

// TestPluginPagePlayerStreamsForTheSignedInPerson walks the player's route: the
// signed-in app asks for the link for a page's file and its own video fetches
// ranges with the person's session. A page on its own (no cookie, opaque origin)
// never plays it, and the page's bridge never hands out a link.
func TestPluginPagePlayerStreamsForTheSignedInPerson(t *testing.T) {
	app := startPageApp(t, nil)
	jar, _ := cookiejar.New(nil)
	owner := &signedIn{t: t, base: app.srv.URL, c: &http.Client{Jar: jar}}
	var setup struct {
		CSRF string `json:"csrf"`
	}
	decodeJSON(t, owner.call("POST", "/api/v1/auth/setup", `{"name":"luis","password":"long enough pass 123"}`, 201), &setup)
	owner.csrf = setup.CSRF
	owner.call("POST", "/api/v1/plugin-pages/player/player/open", "", 200)

	var play struct{ URL string }
	decodeJSON(t, owner.call("POST", "/api/v1/plugin-pages/player/play", `{"file_id":7}`, 200), &play)
	owner.call("POST", "/api/v1/plugin-pages/player/play", `{"file_id":8}`, http.StatusForbidden)
	for _, path := range []string{"/api/v1/plugin-hooks/stream", "/api/v1/plugin-pages/player/play"} {
		b := owner.call("POST", "/api/v1/plugin-pages/player/call", `{"method":"POST","path":"`+path+`","body":{"file_id":7}}`, http.StatusNotFound)
		if strings.Contains(string(b), "plugin-stream") {
			t.Errorf("the bridge handed out %s", b)
		}
	}

	get := func(c *http.Client, h map[string]string) (*http.Response, []byte) {
		req, _ := http.NewRequest("GET", app.srv.URL+play.URL, nil)
		for k, v := range h {
			req.Header.Set(k, v)
		}
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp, b
	}
	resp, b := get(owner.c, map[string]string{"Sec-Fetch-Dest": "video", "Sec-Fetch-Site": "same-origin", "Range": "bytes=100-199"})
	if resp.StatusCode != http.StatusPartialContent || len(b) != 100 || b[0] != 100 || resp.Header.Get("Content-Range") != "bytes 100-199/4096" {
		t.Fatalf("range for the signed-in person: %d %s %d bytes", resp.StatusCode, resp.Header.Get("Content-Range"), len(b))
	}
	page := map[string]string{"Sec-Fetch-Dest": "video", "Sec-Fetch-Site": "cross-site", "Origin": "null", "Range": "bytes=0-9"}
	for name, h := range map[string]map[string]string{"the sandboxed page": page, "a plain client": nil, "a page with a made-up binding": page} {
		c := &http.Client{}
		if resp, _ := get(c, h); resp.StatusCode < 400 {
			t.Errorf("%s plays the link: %d", name, resp.StatusCode)
		}
	}
	if resp, _ := get(owner.c, map[string]string{"Sec-Fetch-Dest": "video", "X-Forwarded-For": "203.0.113.7"}); resp.StatusCode != http.StatusNotFound {
		t.Errorf("behind a proxy: %d", resp.StatusCode)
	}
	accounts.RevokePluginToken("player")
	app.reg.Drop("player")
	if resp, _ := get(owner.c, map[string]string{"Sec-Fetch-Dest": "video"}); resp.StatusCode != http.StatusNotFound {
		t.Errorf("link plays after the revoke: %d", resp.StatusCode)
	}
}

// TestServePluginPageForBrowser starts the same app on HOLYARRS_PLUGIN_PAGE_ADDR
// and keeps it up for the browser spec (tests/plugins/plugin-page-stream.spec.ts);
// skipped in an ordinary go test run.
func TestServePluginPageForBrowser(t *testing.T) {
	addr := os.Getenv("HOLYARRS_PLUGIN_PAGE_ADDR")
	if addr == "" {
		t.Skip("only for the browser spec")
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	startPageApp(t, ln)
	time.Sleep(30 * time.Minute)
}
