package sources

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// Only local fake servers and a fake solver answer here, never a real site
// or a real FlareSolverr.

const checkPageHTML = "<html><head><title>Just a moment...</title></head></html>"

// checkedSite answers with Cloudflare's check page unless the request
// carries the cookie and User-Agent the solver's browser got.
func checkedSite(t *testing.T, direct *atomic.Int32, page func(path string) string) string {
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		direct.Add(1)
		if strings.Contains(r.Header.Get("Cookie"), "cf_clearance=ok") && r.Header.Get("User-Agent") == "Browser/1" {
			_, _ = w.Write([]byte(page(r.URL.Path)))
			return
		}
		w.Header().Set("Server", "cloudflare")
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(checkPageHTML))
	})
	return srv.URL
}

func plainPage(path string) string { return "<html><body>the page " + path + "</body></html>" }

func solved(a BotCheckAsk) BotCheckAnswer {
	u, _ := url.Parse(a.URL)
	return BotCheckAnswer{URL: a.URL, Status: 200, Body: "<html><body>solved " + u.Path + "</body></html>", UserAgent: "Browser/1",
		Cookies: []BotCheckCookie{{Name: "cf_clearance", Value: "ok"}}}
}

// fakeSolver serves as the bot-check plugin for one test.
func fakeSolver(t *testing.T, pin string, asked *atomic.Int32, answer func(BotCheckAsk) (BotCheckAnswer, error)) {
	t.Helper()
	old := BotCheck
	t.Cleanup(func() { BotCheck = old })
	BotCheck = func() *BotCheckSolver {
		return &BotCheckSolver{Pin: pin, Solve: func(_ context.Context, a BotCheckAsk) (BotCheckAnswer, error) {
			asked.Add(1)
			return answer(a)
		}}
	}
}

func noSolver(t *testing.T) {
	old := BotCheck
	t.Cleanup(func() { BotCheck = old })
	BotCheck = func() *BotCheckSolver { return nil }
}

func wantErr(t *testing.T, err error, kind, detail, msg string) {
	t.Helper()
	var se *Error
	if !errors.As(err, &se) || se.Kind != kind || se.Detail != detail || !strings.Contains(se.Msg, msg) {
		t.Fatalf("got %#v (%v), want %s %q %q", se, err, kind, detail, msg)
	}
}

// TestBotCheckSolver_PassesTheCheckThenReusesItsCookie: the solver opens the
// page once; its cookie and User-Agent then go straight to the site.
func TestBotCheckSolver_PassesTheCheckThenReusesItsCookie(t *testing.T) {
	var direct, asked atomic.Int32
	site := checkedSite(t, &direct, plainPage)
	fakeSolver(t, "fs:1", &asked, func(a BotCheckAsk) (BotCheckAnswer, error) {
		if !strings.HasSuffix(a.URL, "/one") || a.Proxy != nil {
			t.Errorf("asked %+v", a)
		}
		return solved(a), nil
	})
	c := New(Config{URL: site, AllowLocal: true, Format: "site"})
	body, err := c.send(context.Background(), http.MethodGet, site+"/one", nil, nil)
	if err != nil || !strings.Contains(string(body), "solved /one") {
		t.Fatalf("first call: %q, %v", body, err)
	}
	body, err = c.send(context.Background(), http.MethodGet, site+"/two", nil, nil)
	if err != nil || !strings.Contains(string(body), "the page /two") {
		t.Fatalf("second call: %q, %v", body, err)
	}
	if asked.Load() != 1 || direct.Load() != 2 {
		t.Fatalf("solver asked %d times, site %d times", asked.Load(), direct.Load())
	}
}

// TestBotCheckSolver_NoneStaysBlockedAsBefore: with no solver the answer is
// exactly today's Blocked one.
func TestBotCheckSolver_NoneStaysBlockedAsBefore(t *testing.T) {
	for name, set := range map[string]func(*testing.T){"unset": func(t *testing.T) {
		old := BotCheck
		t.Cleanup(func() { BotCheck = old })
		BotCheck = nil
	}, "none serving": noSolver} {
		t.Run(name, func(t *testing.T) {
			set(t)
			var direct atomic.Int32
			site := checkedSite(t, &direct, plainPage)
			_, err := New(Config{URL: site, AllowLocal: true, Format: "site"}).send(context.Background(), http.MethodGet, site+"/one", nil, nil)
			wantErr(t, err, KindBlocked, BlockBotCheck, blockText[BlockBotCheck])
			if Explain(err) != blockText[BlockBotCheck] {
				t.Fatalf("explain %q", Explain(err))
			}
		})
	}
}

// TestBotCheckSolver_NotHandedOver: a POST, a call with its own headers and a
// plugin Source never reach the solver.
func TestBotCheckSolver_NotHandedOver(t *testing.T) {
	var direct, asked atomic.Int32
	site := checkedSite(t, &direct, plainPage)
	fakeSolver(t, "fs:1", &asked, func(a BotCheckAsk) (BotCheckAnswer, error) { return solved(a), nil })
	for _, c := range []struct {
		name, method string
		body         []byte
		head         http.Header
	}{
		{"post", http.MethodPost, []byte("{}"), nil},
		{"own headers", http.MethodGet, nil, http.Header{"Cookie": {"session=1"}}},
		{"post with headers", http.MethodPost, []byte("{}"), http.Header{"Content-Type": {"application/json"}}},
	} {
		_, err := New(Config{URL: site, AllowLocal: true, Format: "site"}).send(context.Background(), c.method, site+"/one", c.body, c.head)
		wantErr(t, err, KindBlocked, BlockBotCheck, "")
	}
	old := PluginTransport
	t.Cleanup(func() { PluginTransport = old })
	PluginTransport = func(string) http.RoundTripper {
		return rtFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusForbidden, Status: "403 Forbidden", Header: http.Header{"Server": {"cloudflare"}, "Content-Type": {"text/html"}},
				Body: io.NopCloser(strings.NewReader(checkPageHTML)), Request: r}, nil
		})
	}
	// The plugin's id may well be a name that resolves, so the address rule alone does not hold it back.
	pc := New(Config{URL: "plugin://lab", Format: "torznab", AllowLocal: true,
		resolve: func(context.Context, string) ([]net.IP, error) { return []net.IP{net.IPv4(203, 0, 113, 7)}, nil }})
	_, err := pc.Caps(context.Background())
	wantErr(t, err, KindBlocked, BlockBotCheck, "")
	if asked.Load() != 0 {
		t.Fatalf("solver asked %d times", asked.Load())
	}
}

// TestBotCheckSolver_AddressRules: the Source's address rule is checked
// before the solver is asked and again where it ended.
func TestBotCheckSolver_AddressRules(t *testing.T) {
	var direct atomic.Int32
	site := checkedSite(t, &direct, plainPage)
	su, _ := url.Parse(site)
	public := func(context.Context, string) ([]net.IP, error) { return []net.IP{net.ParseIP("93.184.216.34")}, nil }
	for _, c := range []struct {
		name   string
		raw    string // the page asked for; "" is site+"/one"
		setup  func(*Client)
		endURL string // where the solver says it ended; "" is the page asked
		asks   int32
		msg    string
		ok     bool
	}{
		{name: "rule changed: local now refused", setup: func(c *Client) {
			c.cfg.AllowLocal, c.cfg.AllowAddr = false, func(net.IP) bool { return false }
		}, asks: 0, msg: "local network"},
		{name: "page on another site", raw: "http://elsewhere.example:" + su.Port() + "/one", asks: 0, msg: "another site"},
		{name: "ended on another site", endURL: "https://elsewhere.example/landing", asks: 1, msg: "another site"},
		{name: "ended on a file address", endURL: "file:///etc/passwd", asks: 1, msg: "another site"},
		{name: "ended on a name that leads home", setup: func(c *Client) {
			c.cfg.AllowAddr = func(ip net.IP) bool { return !ip.IsPrivate() && !ip.IsLoopback() }
			c.cfg.resolve = func(_ context.Context, host string) ([]net.IP, error) {
				if strings.HasPrefix(host, "www.") {
					return []net.IP{net.ParseIP("192.168.1.10")}, nil
				}
				return []net.IP{net.ParseIP("93.184.216.34")}, nil
			}
		}, raw: "http://site.example/one", endURL: "http://www.site.example/one", asks: 1, msg: "local network"},
		{name: "metadata address", setup: func(c *Client) {
			c.cfg.resolve = func(context.Context, string) ([]net.IP, error) { return []net.IP{net.ParseIP("169.254.169.254")}, nil }
		}, raw: "http://site.example/one", asks: 0, msg: ""},
		{name: "www. of the same site is fine", setup: func(c *Client) { c.cfg.resolve = public }, raw: "http://site.example/one",
			endURL: "https://www.site.example/one", asks: 1, ok: true},
	} {
		t.Run(c.name, func(t *testing.T) {
			var asked atomic.Int32
			fakeSolver(t, "fs:1", &asked, func(a BotCheckAsk) (BotCheckAnswer, error) {
				out := solved(a)
				if c.endURL != "" {
					out.URL = c.endURL
				}
				return out, nil
			})
			raw := c.raw
			if raw == "" {
				raw = site + "/one"
			}
			base := site
			if strings.Contains(raw, "site.example") {
				base = "http://site.example"
			}
			cl := New(Config{URL: base, AllowLocal: true, Format: "site"})
			if c.setup != nil {
				c.setup(cl)
			}
			u, _ := url.Parse(raw)
			_, err := cl.solve(context.Background(), BotCheck(), raw)
			if c.ok {
				if err != nil || cl.clearanceFor(u.Host) == nil {
					t.Fatalf("got %v, kept %v", err, cl.clearanceFor(u.Host))
				}
			} else {
				var se *Error
				if !errors.As(err, &se) || se.Kind != KindBlocked || !strings.Contains(se.Msg, c.msg) {
					t.Fatalf("got %v", err)
				}
				if cl.clearanceFor(u.Host) != nil {
					t.Fatal("cookies were kept")
				}
			}
			if asked.Load() != c.asks {
				t.Fatalf("solver asked %d times, want %d", asked.Load(), c.asks)
			}
		})
	}
}

// TestBotCheckSolver_SendsTheSourceProxy: the solver gets the Source's proxy
// with its password, so the page comes from the Source's address.
func TestBotCheckSolver_SendsTheSourceProxy(t *testing.T) {
	var direct, asked atomic.Int32
	site := checkedSite(t, &direct, plainPage)
	var got *url.URL
	fakeSolver(t, "fs:1", &asked, func(a BotCheckAsk) (BotCheckAnswer, error) { got = a.Proxy; return solved(a), nil })
	c := New(Config{URL: site, AllowLocal: true, Format: "site", BotCheckPin: "fs:1"})
	c.cfg.Proxy, _ = url.Parse("socks5://me:secret@vpn:1080") // after New, so the test's own calls go straight to the fake site
	if _, err := c.solve(context.Background(), BotCheck(), site+"/one"); err != nil {
		t.Fatal(err)
	}
	if pass, _ := got.User.Password(); got == nil || got.Host != "vpn:1080" || got.User.Username() != "me" || pass != "secret" {
		t.Fatalf("proxy sent: %v", got)
	}
}

// TestBotCheckSolver_KeyOnlyForItsSolver: a Source with a key, passkey or
// proxy password is handed only to the solver it was saved with.
func TestBotCheckSolver_KeyOnlyForItsSolver(t *testing.T) {
	var direct atomic.Int32
	site := checkedSite(t, &direct, plainPage)
	proxy, _ := url.Parse("http://me:pw@proxy.example:3128")
	for _, c := range []struct {
		name string
		cfg  Config
		ok   bool
	}{
		{"no secrets, no pin", Config{}, true},
		{"key, same solver", Config{APIKey: "KEY123", BotCheckPin: "fs:1"}, true},
		{"key, no pin", Config{APIKey: "KEY123"}, false},
		{"key, other solver", Config{APIKey: "KEY123", BotCheckPin: "other:1"}, false},
		{"passkey, other solver", Config{PathKeys: []string{"PK"}, BotCheckPin: "fs:0"}, false},
		{"proxy password, no pin", Config{Proxy: proxy}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			var asked atomic.Int32
			fakeSolver(t, "fs:1", &asked, func(a BotCheckAsk) (BotCheckAnswer, error) { return solved(a), nil })
			cfg := c.cfg
			cfg.URL, cfg.AllowLocal, cfg.Format = site, true, "site"
			cl := New(Config{URL: site, AllowLocal: true, Format: "site"})
			cfg.AllowAddr = cl.cfg.AllowAddr
			cl.cfg = cfg
			_, err := cl.solve(context.Background(), BotCheck(), site+"/one")
			if c.ok != (err == nil) || c.ok != (asked.Load() == 1) {
				t.Fatalf("err %v, asked %d", err, asked.Load())
			}
			if !c.ok && !strings.Contains(err.Error(), "type the key") {
				t.Fatalf("got %v", err)
			}
		})
	}
}

// TestBotCheckSolver_PinFollowsTheTypedKey: saving a Source with its key
// typed records the serving solver; a save without the key keeps the old one.
func TestBotCheckSolver_PinFollowsTheTypedKey(t *testing.T) {
	ctx := context.Background()
	s, v := openVault(t)
	by := domain.System
	var asked atomic.Int32
	fakeSolver(t, "fs:1", &asked, func(a BotCheckAsk) (BotCheckAnswer, error) { return solved(a), nil })
	src, err := Add(ctx, s, v, Input{Name: "Keyed", Format: "torznab", URL: "https://keyed.example/api", APIKey: "k-" + "keyed"}, by)
	if err != nil || src.Extra[extraBotCheckPin] != "fs:1" {
		t.Fatalf("add: pin %q, %v", src.Extra[extraBotCheckPin], err)
	}
	fakeSolver(t, "fs:2", &asked, func(a BotCheckAsk) (BotCheckAnswer, error) { return solved(a), nil })
	if src, err = Change(ctx, s, v, src.ID, Input{Name: "Keyed again"}, by); err != nil || src.Extra[extraBotCheckPin] != "fs:1" {
		t.Fatalf("change without key: pin %q, %v", src.Extra[extraBotCheckPin], err)
	}
	cfg, err := ConfigFor(ctx, src, v)
	if err != nil || cfg.BotCheckPin != "fs:1" {
		t.Fatalf("config pin %q, %v", cfg.BotCheckPin, err)
	}
	if src, err = Change(ctx, s, v, src.ID, Input{Name: "Keyed again", APIKey: "k-" + "keyed"}, by); err != nil || src.Extra[extraBotCheckPin] != "fs:2" {
		t.Fatalf("change with key: pin %q, %v", src.Extra[extraBotCheckPin], err)
	}
	noSolver(t)
	if src, err = Change(ctx, s, v, src.ID, Input{Name: "Keyed again", APIKey: "k-" + "keyed"}, by); err != nil || src.Extra[extraBotCheckPin] != "" {
		t.Fatalf("no solver: pin %q, %v", src.Extra[extraBotCheckPin], err)
	}
}

// TestBotCheckSolver_Answers: the solver's status, a feed error inside its
// page and a broken answer are read as the Source's own would be.
func TestBotCheckSolver_Answers(t *testing.T) {
	var direct atomic.Int32
	site := checkedSite(t, &direct, plainPage)
	for _, c := range []struct {
		name   string
		answer BotCheckAnswer
		err    error
		kind   string
		msg    string
	}{
		{"slow down", BotCheckAnswer{Status: 429}, nil, KindRateLimited, "slow down"},
		{"site down", BotCheckAnswer{Status: 502}, nil, KindDown, "502"},
		{"not found", BotCheckAnswer{Status: 404}, nil, KindBroken, "404"},
		{"feed error in the page", BotCheckAnswer{Status: 200, Body: `<html><head></head><body><pre>&lt;error code="100" description="bad key KEY123"/&gt;</pre></body></html>`}, nil, KindLoginRefused, "[hidden]"},
		{"solver failed, key scrubbed", BotCheckAnswer{}, errors.New("Challenge not solved for https://x/?apikey=KEY123"), KindBlocked, "Challenge not solved"},
	} {
		t.Run(c.name, func(t *testing.T) {
			var asked atomic.Int32
			fakeSolver(t, "fs:1", &asked, func(a BotCheckAsk) (BotCheckAnswer, error) { return c.answer, c.err })
			cl := New(Config{URL: site, AllowLocal: true, Format: "site", APIKey: "KEY123", BotCheckPin: "fs:1"})
			_, err := cl.send(context.Background(), http.MethodGet, site+"/one", nil, nil)
			var se *Error
			if !errors.As(err, &se) || se.Kind != c.kind || !strings.Contains(se.Msg, c.msg) || strings.Contains(se.Msg, "KEY123") {
				t.Fatalf("got %v", err)
			}
		})
	}
}

// TestBotCheckSolver_TorznabFeedEndToEnd: a Torznab feed comes back from the
// solver as Chrome's XML viewer page, is read as the feed, and the next
// call goes straight to the site with the solver's cookie.
func TestBotCheckSolver_TorznabFeedEndToEnd(t *testing.T) {
	const feed = `<rss version="2.0" xmlns:torznab="http://torznab.com/schemas/2015/feed"><channel><item><title>Dune 2021 1080p</title>` +
		`<guid>g1</guid><link>https://x.example/dl/1.torrent</link><size>100</size><torznab:attr name="seeders" value="5"></torznab:attr></item></channel></rss>`
	var direct, asked atomic.Int32
	site := checkedSite(t, &direct, func(string) string { return `<?xml version="1.0"?>` + feed })
	fakeSolver(t, "fs:1", &asked, func(a BotCheckAsk) (BotCheckAnswer, error) {
		out := solved(a)
		out.Body = `<html><head><style>x</style></head><body><div id="webkit-xml-viewer-source-xml">` + feed +
			`</div><div class="header"><span>This XML file does not appear to have any style information.</span></div></body></html>`
		return out, nil
	})
	c := New(Config{URL: site + "/api", AllowLocal: true, APIKey: "KEY123", BotCheckPin: "fs:1", Protocol: "torrent"})
	for i := range 2 {
		items, err := c.Feed(context.Background(), nil)
		if err != nil || len(items) != 1 || items[0].Title != "Dune 2021 1080p" {
			t.Fatalf("feed %d: %+v, %v", i+1, items, err)
		}
	}
	if asked.Load() != 1 || direct.Load() != 2 {
		t.Fatalf("solver asked %d times, site %d times", asked.Load(), direct.Load())
	}
}

func TestUnwrapPre(t *testing.T) {
	const xmlFeed = `<rss version="2.0"><channel><title>a &amp; b</title></channel></rss>`
	for _, c := range []struct{ name, in, want string }{
		{"json in pre", `<html><head><meta name="color-scheme" content="light dark"></head><body><pre style="word-wrap: break-word;">{"a":"&lt;b&gt;"}</pre></body></html>`, `{"a":"<b>"}`},
		{"json array", `<html><body><pre>[1,2]</pre></body></html>`, `[1,2]`},
		{"escaped xml in pre", `<html><head></head><body><pre>&lt;?xml version="1.0"?&gt;&lt;caps&gt;&lt;server/&gt;&lt;/caps&gt;</pre></body></html>`, `<?xml version="1.0"?><caps><server/></caps>`},
		{"chrome xml viewer", `<html><head></head><body><div id="webkit-xml-viewer-source-xml">` + xmlFeed + `</div><div class="pretty-print">...</div></body></html>`, `<?xml version="1.0" encoding="UTF-8"?>` + "\n" + xmlFeed},
		{"plain page", "<html><body>x</body></html>", "<html><body>x</body></html>"},
		{"broken xml in pre stays", `<html><body><pre>&lt;rss&gt;&lt;channel&gt;</pre></body></html>`, `<html><body><pre>&lt;rss&gt;&lt;channel&gt;</pre></body></html>`},
		{"text in pre stays", `<html><body><pre>hello</pre></body></html>`, `<html><body><pre>hello</pre></body></html>`},
	} {
		if got := string(unwrapPre([]byte(c.in))); got != c.want {
			t.Errorf("%s: got %q", c.name, got)
		}
	}
}

func TestCookieLine(t *testing.T) {
	for _, c := range []struct {
		name string
		in   []BotCheckCookie
		want string
	}{
		{"two", []BotCheckCookie{{"cf_clearance", "ok"}, {"__ddg1_", "x-y"}}, "cf_clearance=ok; __ddg1_=x-y"},
		{"header break left out", []BotCheckCookie{{"a", "1\r\nX-Evil: 1"}, {"b", "2"}}, "b=2"},
		{"semicolon left out", []BotCheckCookie{{"a", "1; c=3"}}, ""},
		{"bad name left out", []BotCheckCookie{{"a b", "1"}, {"", "2"}}, ""},
	} {
		if got := cookieLine(c.in); got != c.want {
			t.Errorf("%s: %q", c.name, got)
		}
	}
}

func TestSameSite(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{{"1337x.to", "www.1337x.to", true}, {"WWW.Site.example.", "site.example", true}, {"x1337x.ws", "1337x.to", false}, {"", "a", false}, {"evil.site.example", "site.example", false}} {
		if got := sameSite(c.a, c.b); got != c.want {
			t.Errorf("%q %q: %v", c.a, c.b, got)
		}
	}
}

func TestBotCheckPin(t *testing.T) {
	a := BotCheckPin("fs", []string{"flaresolverr:8191"})
	if a != BotCheckPin("fs", []string{"flaresolverr:8191"}) || a == BotCheckPin("fs", []string{"other:8191"}) || a == BotCheckPin("fs2", []string{"flaresolverr:8191"}) {
		t.Fatal("the pin does not follow the plugin and its hosts")
	}
	if !strings.HasPrefix(a, "fs:") {
		t.Fatalf("pin %q", a)
	}
}
