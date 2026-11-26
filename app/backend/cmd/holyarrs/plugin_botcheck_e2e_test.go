package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/plugins/hooks"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

// TestPluginBotcheckEndToEnd: a fake solver plugin, polling over HTTP with
// its token, opens a Source's bot check; the feed it hands back is read and
// the next call goes straight to the site with its cookie. A plugin without
// sources.botcheck gets 403. Only local fake servers answer.
func TestPluginBotcheckEndToEnd(t *testing.T) {
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
	const feed = `<rss version="2.0"><channel><item><title>Dune 2021 1080p</title><guid>g1</guid><link>https://x.example/dl/1.torrent</link><size>100</size></item></channel></rss>`
	var direct atomic.Int32
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		direct.Add(1)
		if strings.Contains(r.Header.Get("Cookie"), "cf_clearance=ok") && r.Header.Get("User-Agent") == "Browser/1" {
			_, _ = w.Write([]byte(feed))
			return
		}
		w.Header().Set("Server", "cloudflare")
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("<html><head><title>Just a moment...</title></head></html>"))
	}))
	t.Cleanup(site.Close)

	reg := newPluginHooks()
	oldBC := sources.BotCheck
	t.Cleanup(func() { sources.PluginTransport = nil; sources.BotCheck = oldBC })
	wireBotCheck(reg.Broker, func(string) []string { return []string{"fs.home.arpa:8191"} })
	au := accounts.New(st, accounts.Options{Vault: v})
	a := buildHandlers(services{Store: st, Queue: jobs.New(st, jobs.Options{Workers: 1}), Metadata: metadata.Lab(), Version: "test", Auth: au, Secrets: v,
		Live: webserver.NewLiveHub(webserver.LiveOptions{}), PluginHooks: reg})
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	t.Cleanup(cancel)

	full := accounts.IssuePluginToken("solver", []string{"sources.botcheck"})
	none := accounts.IssuePluginToken("reader", []string{"sources.releases"})
	t.Cleanup(func() { accounts.RevokePluginToken("solver"); accounts.RevokePluginToken("reader"); reg.Drop("solver") })
	p := stubPlugin{t: t, base: srv.URL, tok: full}
	q := stubPlugin{t: t, base: srv.URL, tok: none}
	if code, _ := q.do("GET", "/api/v1/plugin-hooks/botcheck", nil); code != http.StatusForbidden {
		t.Fatalf("botcheck without its scope: %d", code)
	}
	if sources.BotCheck() != nil {
		t.Fatal("a solver serves before any plugin polled")
	}
	var asked atomic.Int32
	p.serve(ctx, "botcheck", func(j hooks.Job) any {
		asked.Add(1)
		var in hooks.BotcheckAsk
		_ = json.Unmarshal(j.Payload, &in)
		return hooks.BotcheckAnswer{URL: in.URL, Status: 200, UserAgent: "Browser/1", Cookies: []hooks.BotcheckCookie{{Name: "cf_clearance", Value: "ok"}},
			Body: `<html><head></head><body><div id="webkit-xml-viewer-source-xml">` + feed + `</div></body></html>`}
	})
	waitProvider(t, reg, hooks.Botcheck, "solver")
	s := sources.BotCheck()
	if s == nil || s.Pin != sources.BotCheckPin("solver", []string{"fs.home.arpa:8191"}) {
		t.Fatalf("solver %+v", s)
	}
	c := sources.New(sources.Config{URL: site.URL + "/api", AllowLocal: true, Protocol: "torrent"})
	for i := range 2 {
		items, err := c.Feed(ctx, nil)
		if err != nil || len(items) != 1 || items[0].Title != "Dune 2021 1080p" {
			t.Fatalf("feed %d: %+v, %v", i+1, items, err)
		}
	}
	if asked.Load() != 1 || direct.Load() != 2 {
		t.Fatalf("solver asked %d times, site %d times", asked.Load(), direct.Load())
	}
	// A Source with a key saved before this solver is not handed over.
	keyed := sources.New(sources.Config{URL: site.URL + "/api", AllowLocal: true, Protocol: "torrent", APIKey: "KEY123", BotCheckPin: "other:1"})
	if _, err := keyed.Feed(ctx, nil); err == nil || !strings.Contains(err.Error(), "type the key") || asked.Load() != 1 {
		t.Fatalf("keyed: %v, asked %d", err, asked.Load())
	}
}
