package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/plugins/hooks"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

// stubPlugin is a plugin program in miniature: it talks to HolyArrs only
// over HTTP with its Bearer token, like a real one.
type stubPlugin struct {
	t    *testing.T
	base string
	tok  string
}

func (p stubPlugin) do(method, path string, body any) (int, []byte) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, p.base+path, rd)
	req.Header.Set("Authorization", "Bearer "+p.tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil // the server is gone (test over); the poll loop stops
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

// serve answers jobs on one hook until ctx ends.
func (p stubPlugin) serve(ctx context.Context, hook string, answer func(hooks.Job) any) {
	go func() {
		for ctx.Err() == nil {
			code, b := p.do("GET", "/api/v1/plugin-hooks/"+hook, nil)
			if code != http.StatusOK {
				if code != http.StatusNoContent {
					return
				}
				continue
			}
			var j hooks.Job
			_ = json.Unmarshal(b, &j)
			data, _ := json.Marshal(answer(j))
			p.do("POST", "/api/v1/plugin-hooks/"+hook, map[string]any{"job": j.ID, "data": json.RawMessage(data)})
		}
	}()
}

// waitProvider waits until the plugin polls hook.
func waitProvider(t *testing.T, reg *hooks.Registry, hook, id string) {
	t.Helper()
	for range 200 {
		for _, p := range reg.Broker.Providers(hook) {
			if p == id {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s never polled %s", id, hook)
}

// TestPluginHooksEndToEnd runs each provider hook through the real router
// with a stub plugin speaking HTTP, then revokes it and checks every hook
// stops at once.
func TestPluginHooksEndToEnd(t *testing.T) {
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
	reg := newPluginHooks()
	t.Cleanup(func() { sources.PluginTransport = nil })
	hub := webserver.NewLiveHub(webserver.LiveOptions{})
	hub.Tap(reg.Publish)
	au := accounts.New(st, accounts.Options{Vault: v})
	a := buildHandlers(services{Store: st, Queue: jobs.New(st, jobs.Options{Workers: 1}), Metadata: metadata.Lab(), Version: "test", Auth: au, Secrets: v, Live: hub, PluginHooks: reg})
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	t.Cleanup(cancel) // stub plugins stop before the server closes
	jar, _ := cookiejar.New(nil)
	owner := &signedIn{t: t, base: srv.URL, c: &http.Client{Jar: jar}}
	var setup struct {
		CSRF string `json:"csrf"`
	}
	decodeJSON(t, owner.call("POST", "/api/v1/auth/setup", `{"name":"luis","password":"long enough pass 123"}`, 201), &setup)
	owner.csrf = setup.CSRF

	all := []string{"sources.downloader", "sources.releases", "assistant.tools", "alerts.events.subscribe", "library.read",
		"pages.title-section", "pages.add", "pages.home-widget", "requests.users.read", "media.stream"}
	tok := accounts.IssuePluginToken("stub", all)
	t.Cleanup(func() { accounts.RevokePluginToken("stub"); reg.Drop("stub") })
	p := stubPlugin{t: t, base: srv.URL, tok: tok}

	// 1. Download client.
	var mu sync.Mutex
	var added []hooks.AddJob
	p.serve(ctx, "downloader", func(j hooks.Job) any {
		switch j.Op {
		case "add":
			var a hooks.AddJob
			_ = json.Unmarshal(j.Payload, &a)
			mu.Lock()
			added = append(added, a)
			mu.Unlock()
			return map[string]string{"id": "j1"}
		case "health":
			return map[string]string{"version": "stub 1"}
		}
		return []downloads.Job{{ID: "j1", Name: "Dune", State: downloads.StateDownloading}}
	})
	waitProvider(t, reg, hooks.Downloader, "stub")
	eng := pluginDownloader(reg)("stub")
	if h, err := eng.Health(ctx); err != nil || h.Version != "stub 1" {
		t.Fatalf("download client health: %v %v", h, err)
	}
	if id, err := eng.Add(ctx, downloads.AddRequest{Name: "Dune", File: []byte("d8:announce"), URL: "https://idx.example/get?apikey=SECRET"}); err != nil || id != "j1" {
		t.Fatalf("add: %q %v", id, err)
	}
	if jobs, err := eng.List(ctx, ""); err != nil || len(jobs) != 1 {
		t.Fatalf("queue: %v %v", jobs, err)
	}
	mu.Lock()
	if len(added) != 1 || strings.Contains(string(added[0].File)+added[0].Magnet, "SECRET") {
		t.Errorf("plugin got %+v", added)
	}
	mu.Unlock()

	// 2. Search source.
	p.serve(ctx, "releases", func(j hooks.Job) any {
		return hooks.FeedAnswer{Status: 200, Type: "application/rss+xml", Body: []byte(`<?xml version="1.0"?><rss xmlns:torznab="http://torznab.com/schemas/2015/feed"><channel><item><title>Dune 2021 1080p</title><guid>1</guid><link>plugin://stub/get?id=1</link><size>100</size></item></channel></rss>`)}
	})
	waitProvider(t, reg, hooks.Releases, "stub")
	items, err := sources.New(sources.Config{URL: "plugin://stub", Protocol: "torrent", Format: "torznab"}).Feed(ctx, nil)
	if err != nil || len(items) != 1 || items[0].Title != "Dune 2021 1080p" {
		t.Fatalf("search source: %+v %v", items, err)
	}
	if _, err := sources.Transport(sources.Config{URL: "plugin://stub"}).RoundTrip(httptest.NewRequest("GET", "http://127.0.0.1:1/x", nil)); err == nil {
		t.Error("a plugin source reached an inside address")
	}

	// 3. Assistant tools.
	if code, b := p.do("PUT", "/api/v1/plugin-hooks/tools", []hooks.Tool{{Name: "lyrics", About: "Find lyrics", Scopes: []string{"library.read"}}}); code != 204 {
		t.Fatalf("tools: %d %s", code, b)
	}
	p.serve(ctx, "tools", func(j hooks.Job) any { return map[string]string{"text": "la la"} })
	waitProvider(t, reg, hooks.Tools, "stub")
	pt := pluginTools{reg}
	ownerWho := webserver.Caller{Role: webserver.RoleOwner}
	if l := pt.List(ownerWho); len(l) != 1 || l[0]["name"] != "plugin.stub.lyrics" {
		t.Fatalf("tool list %v", l)
	}
	if l := pt.List(webserver.Caller{Role: "member"}); len(l) != 0 {
		t.Errorf("a member without every library.read right saw the tool: %v", l)
	}
	res, err := pt.Call(httptest.NewRequest("POST", "/mcp", nil), ownerWho, "plugin.stub.lyrics", map[string]any{})
	if r, ok := res.(hooks.ToolResult); err != nil || !ok || !r.Untrusted {
		t.Fatalf("tool call: %+v %v", res, err)
	}

	// 4. Events.
	evs := make(chan []byte, 4)
	go func() {
		for ctx.Err() == nil {
			code, b := p.do("GET", "/api/v1/plugin-hooks/events", nil)
			if code == 200 {
				evs <- b
				return
			}
			if code != 204 {
				return
			}
		}
	}()
	waitProvider(t, reg, hooks.Events, "stub")
	hub.Publish("event", map[string]any{"kind": "sign_in_failed", "summary": "from 10.0.0.9"})
	hub.Publish("event", map[string]any{"kind": "title_added", "title_id": 4, "summary": "Dune"})
	select {
	case b := <-evs:
		if !strings.Contains(string(b), "title_added") || strings.Contains(string(b), "10.0.0.9") {
			t.Errorf("event %s", b)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no event")
	}

	// 5. Title section, 6. Home widget.
	p.serve(ctx, "sections", func(j hooks.Job) any {
		return hooks.Card{Title: "On Trakt", Rows: []hooks.CardRow{{Text: "Watched twice"}}}
	})
	waitProvider(t, reg, hooks.Sections, "stub")
	if b := owner.call("GET", "/api/v1/titles/4/plugin-sections", "", 200); !strings.Contains(string(b), "Watched twice") {
		t.Errorf("sections %s", b)
	}
	if code, _ := p.do("PUT", "/api/v1/plugin-hooks/home-widget", hooks.Card{Title: "Now watching", Rows: []hooks.CardRow{}}); code != 204 {
		t.Fatal("widget refused")
	}
	if b := owner.call("GET", "/api/v1/plugin-widgets", "", 200); !strings.Contains(string(b), "Now watching") {
		t.Errorf("widgets %s", b)
	}

	// 7. Pages.
	if code, _ := p.do("PUT", "/api/v1/plugin-hooks/pages", []hooks.Page{{ID: "player", Title: "Player", File: "index.html"}}); code != 204 {
		t.Fatal("pages refused")
	}
	p.serve(ctx, "pages", func(j hooks.Job) any { return hooks.FeedAnswer{Status: 200, Body: []byte("<p>hi</p>")} })
	waitProvider(t, reg, hooks.Pages, "stub")
	var open struct{ URL string }
	_ = json.Unmarshal(owner.call("POST", "/api/v1/plugin-pages/stub/player/open", "", 200), &open)
	resp, err := http.Get(srv.URL + open.URL) // no cookie: the token in the path is the key
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 || (!strings.HasPrefix(resp.Header.Get("Content-Security-Policy"), "sandbox allow-scripts") || strings.Contains(resp.Header.Get("Content-Security-Policy"), "allow-same-origin") || len(resp.Header.Values("Content-Security-Policy")) != 1) {
		t.Fatalf("page file %d %q", resp.StatusCode, resp.Header.Get("Content-Security-Policy"))
	}
	owner.call("POST", "/api/v1/plugin-pages/stub/call", `{"method":"GET","path":"/api/v1/settings/ui"}`, http.StatusForbidden)
	owner.call("POST", "/api/v1/plugin-pages/stub/call", `{"method":"GET","path":"/api/v1/asks"}`, http.StatusForbidden) // plugin lacks requests.read
	var bridged struct{ Status int }
	_ = json.Unmarshal(owner.call("POST", "/api/v1/plugin-pages/stub/call", `{"method":"GET","path":"/api/v1/titles"}`, 200), &bridged)
	if bridged.Status != http.StatusOK {
		t.Errorf("bridge call both may make: %d", bridged.Status)
	}

	// 8. People's names.
	if code, b := p.do("GET", "/api/v1/plugin-hooks/users", nil); code != 200 || !strings.Contains(string(b), `"name":"luis"`) || strings.Contains(string(b), "role") {
		t.Errorf("users %d %s", code, b)
	}

	// 9. Media stream: no folder granted (no engine), so refused.
	if code, _ := p.do("POST", "/api/v1/plugin-hooks/stream", map[string]int{"file_id": 1}); code != http.StatusForbidden {
		t.Errorf("stream without a granted folder: %d", code)
	}

	// Revoke: everything stops at once.
	accounts.RevokePluginToken("stub")
	reg.Drop("stub")
	if code, _ := p.do("GET", "/api/v1/plugin-hooks/users", nil); code != http.StatusUnauthorized {
		t.Errorf("revoked token: %d", code)
	}
	if _, err := eng.Health(ctx); err == nil {
		t.Error("download client still answers after revoke")
	}
	if b := owner.call("GET", "/api/v1/plugin-widgets", "", 200); strings.Contains(string(b), "Now watching") {
		t.Error("widget still shows after revoke")
	}
	if len(pt.List(ownerWho)) != 0 {
		t.Error("tools still listed after revoke")
	}
	if resp, err := http.Get(srv.URL + open.URL); err == nil {
		resp.Body.Close()
		if resp.StatusCode == 200 {
			t.Error("page still served after revoke")
		}
	}
}
