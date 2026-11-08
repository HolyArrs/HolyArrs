package sources

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// TestBotCheck_ToldApartFromRefusedKey checks that a bot check or ban page is told apart from a refused key, while a plain 403 still means the key.
// Only a local fake server answers, never a real site.
func TestBotCheck_ToldApartFromRefusedKey(t *testing.T) {
	for _, c := range []struct {
		name   string
		status int
		header map[string]string
		body   string
		kind   string
		detail string
	}{
		{"cloudflare header", 403, map[string]string{"Cf-Mitigated": "challenge", "Server": "cloudflare"}, "<html></html>", KindBlocked, BlockBotCheck},
		{"cloudflare header on a 429", 429, map[string]string{"Cf-Mitigated": "challenge"}, "", KindBlocked, BlockBotCheck},
		{"cloudflare page", 403, html("cloudflare"), "<!DOCTYPE html><html><head><title>Just a moment...</title></head><body><script>window._cf_chl_opt={}</script></body></html>", KindBlocked, BlockBotCheck},
		{"cloudflare js check", 503, html("cloudflare"), "<html><head><title>Just a moment...</title></head><body><script src=\"/cdn-cgi/challenge-platform/h/b/orchestrate/x\"></script></body></html>", KindBlocked, BlockBotCheck},
		{"ddos-guard", 503, html("ddos-guard"), "<html><head><title>DDoS-Guard</title></head></html>", KindBlocked, BlockBotCheck},
		{"cloudflare ban", 403, html("cloudflare"), "<html><head><title>Access denied | x used Cloudflare to restrict access</title></head><body>Error code: 1020</body></html>", KindBlocked, BlockSiteBan},
		{"cloudflare firewall block", 403, html("cloudflare"), "<html><head><title>Attention Required! | Cloudflare</title></head><body><h1>Sorry, you have been blocked</h1></body></html>", KindBlocked, BlockSiteBan},
		{"plain 403", 403, nil, "Forbidden", KindLoginRefused, ""},
		{"cloudflare 403 without a check page", 403, map[string]string{"Server": "cloudflare"}, "invalid api key", KindLoginRefused, ""},
		{"site's own 403 page behind cloudflare", 403, html("cloudflare"), "<html><head><title>Invalid API key</title></head><body><script src=\"/cdn-cgi/challenge-platform/scripts/jsd/main.js\"></script><div id=\"cf-chl-widget-1\"></div></body></html>", KindLoginRefused, ""},
		{"feed error naming ddos-guard", 403, map[string]string{"Server": "ddos-guard", "Content-Type": "application/xml"}, "<error code=\"100\" description=\"key refused (ddos-guard)\"/>", KindLoginRefused, ""},
		{"cloudflare rate limit", 429, html("cloudflare"), "<html><head><title>Access denied</title></head><body>error code: 1015</body></html>", KindRateLimited, ""},
		{"cloudflare 500", 500, html("cloudflare"), "<title>Just a moment</title>", KindDown, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				for k, v := range c.header {
					w.Header().Set(k, v)
				}
				w.WriteHeader(c.status)
				_, _ = w.Write([]byte(c.body))
			}))
			defer srv.Close()
			_, err := New(Config{URL: srv.URL + "/api", Protocol: "torrent", AllowLocal: true}).Caps(context.Background())
			var se *Error
			if !errors.As(err, &se) || se.Kind != c.kind || se.Detail != c.detail {
				t.Fatalf("got %#v (%v), want kind %s detail %q", se, err, c.kind, c.detail)
			}
			if c.detail != "" && Explain(err) != blockText[c.detail] {
				t.Fatalf("explain: %q", Explain(err))
			}
		})
	}
}

// TestBotCheck_PauseHealthAndRecovery checks that the block kind is kept while paused, shown in health and the search outcome, and cleared when the Source answers again or fails another way.
func TestBotCheck_PauseHealthAndRecovery(t *testing.T) {
	ctx := context.Background()
	s, v := openVault(t)
	src, err := Add(ctx, s, v, Input{Name: "Guarded", Format: "torznab", URL: "http://guarded.example/api"}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	sr := &Searcher{Store: s}
	get := func() domain.Source {
		var got domain.Source
		if err := database.Get(ctx, s.Read(), "sources", src.ID, &got); err != nil {
			t.Fatal(err)
		}
		return got
	}
	sr.pause(ctx, src, &Error{Kind: KindBlocked, Detail: BlockBotCheck, Msg: "403"})
	got := get()
	if got.Extra["block"] != BlockBotCheck || got.Extra["failures"] != "1" {
		t.Fatalf("extra %+v", got.Extra)
	}
	if h := HealthOf(got); h.Text != "Blocked by the site's bot check. Reach this site through Jackett or Prowlarr. HolyArrs tries again on its own." || h.Reason != "blocked" {
		t.Fatalf("health %+v", h)
	}
	if HealthOf(got).Block != BlockBotCheck {
		t.Fatalf("health block %q", HealthOf(got).Block)
	}
	if BlockOf(got) != BlockBotCheck || pauseWords(BlockBotCheck) != "blocked by the site's bot check" {
		t.Fatalf("why %q", BlockOf(got))
	}
	sr.pause(ctx, got, &Error{Kind: KindDown, Msg: "x"})
	if got = get(); got.Extra["block"] != "" || got.Extra["failures"] != "2" {
		t.Fatalf("block kept after another failure: %+v", got.Extra)
	}
	sr.pause(ctx, got, &Error{Kind: KindBlocked, Detail: BlockSiteBan, Msg: "403"})
	if got = get(); !strings.HasPrefix(HealthOf(got).Text, "The site's protection refuses") {
		t.Fatalf("ban health %+v", HealthOf(got))
	}
	sr.recovered(ctx, got)
	if got = get(); got.State != "enabled" || got.Extra["block"] != "" || got.Extra["failures"] != "" {
		t.Fatalf("not recovered: %+v", got)
	}
	local := got
	local.State, local.PauseReason = "paused", ptr(KindBlocked)
	if h := HealthOf(local); !strings.Contains(h.Text, "local network") {
		t.Fatalf("local block %+v", h)
	}
}

// html returns response headers for an HTML page from the given server.
func html(server string) map[string]string {
	return map[string]string{"Server": server, "Content-Type": "text/html; charset=UTF-8"}
}
