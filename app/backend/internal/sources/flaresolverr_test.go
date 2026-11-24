package sources

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// checkedSite answers with Cloudflare's check page unless the request
// carries the cookie and User-Agent FlareSolverr's browser got.
func checkedSite(t *testing.T, direct *atomic.Int32) string {
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		direct.Add(1)
		if strings.Contains(r.Header.Get("Cookie"), "cf_clearance=ok") && r.Header.Get("User-Agent") == "Browser/1" {
			_, _ = w.Write([]byte("<html><body>the page " + r.URL.Path + "</body></html>"))
			return
		}
		w.Header().Set("Server", "cloudflare")
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("<html><head><title>Just a moment...</title></head></html>"))
	})
	return srv.URL
}

func fakeFlare(t *testing.T, asked *atomic.Int32, answer func(flareAsk) any) *url.URL {
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/v1" {
			t.Errorf("FlareSolverr got %s %s", r.Method, r.URL.Path)
		}
		var a flareAsk
		if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
			t.Error(err)
		}
		_ = json.NewEncoder(w).Encode(answer(a))
	})
	u, _ := url.Parse(srv.URL)
	return u
}

func solved(a flareAsk) any {
	u, _ := url.Parse(a.URL)
	return map[string]any{"status": "ok", "solution": map[string]any{"url": a.URL, "status": 200,
		"response": "<html><body>solved " + u.Path + "</body></html>", "userAgent": "Browser/1",
		"cookies": []map[string]any{{"name": "cf_clearance", "value": "ok"}}}}
}

func TestFlareSolverr_PassesTheCheckThenReusesItsCookie(t *testing.T) {
	var direct, asked atomic.Int32
	site := checkedSite(t, &direct)
	c := New(Config{URL: site, AllowLocal: true, Format: "site", FlareSolverr: fakeFlare(t, &asked, func(a flareAsk) any {
		if a.Cmd != "request.get" || !strings.HasSuffix(a.URL, "/one") || a.MaxTimeout != 60000 {
			t.Errorf("asked %+v", a)
		}
		return solved(a)
	})})
	body, err := c.send(context.Background(), http.MethodGet, site+"/one", nil, nil)
	if err != nil || !strings.Contains(string(body), "solved /one") {
		t.Fatalf("first call: %q, %v", body, err)
	}
	// The cookie is sent straight to the site next time; FlareSolverr is not asked again.
	body, err = c.send(context.Background(), http.MethodGet, site+"/two", nil, nil)
	if err != nil || !strings.Contains(string(body), "the page /two") {
		t.Fatalf("second call: %q, %v", body, err)
	}
	if asked.Load() != 1 || direct.Load() != 2 {
		t.Fatalf("FlareSolverr asked %d times, site %d times", asked.Load(), direct.Load())
	}
}

func TestFlareSolverr_NotSetStaysBlocked(t *testing.T) {
	var direct atomic.Int32
	site := checkedSite(t, &direct)
	c := New(Config{URL: site, AllowLocal: true, Format: "site"})
	_, err := c.send(context.Background(), http.MethodGet, site+"/one", nil, nil)
	var se *Error
	if !errors.As(err, &se) || se.Kind != KindBlocked || se.Detail != BlockBotCheck || !strings.Contains(se.Msg, "FlareSolverr") {
		t.Fatalf("got %v", err)
	}
}

func TestFlareSolverr_FailureIsABotCheckBlock(t *testing.T) {
	var direct, asked atomic.Int32
	site := checkedSite(t, &direct)
	c := New(Config{URL: site, AllowLocal: true, Format: "site", FlareSolverr: fakeFlare(t, &asked, func(flareAsk) any {
		return map[string]any{"status": "error", "message": "Error: Challenge not solved"}
	})})
	_, err := c.send(context.Background(), http.MethodGet, site+"/one", nil, nil)
	var se *Error
	if !errors.As(err, &se) || se.Kind != KindBlocked || !strings.Contains(se.Msg, "Challenge not solved") {
		t.Fatalf("got %v", err)
	}
}

func TestFlareSolverr_POSTIsNotHandedOver(t *testing.T) {
	var direct, asked atomic.Int32
	site := checkedSite(t, &direct)
	c := New(Config{URL: site, AllowLocal: true, Format: "site", FlareSolverr: fakeFlare(t, &asked, solved)})
	_, err := c.send(context.Background(), http.MethodPost, site+"/v1", []byte("{}"), http.Header{"Content-Type": {"application/json"}})
	var se *Error
	if !errors.As(err, &se) || se.Kind != KindBlocked || asked.Load() != 0 {
		t.Fatalf("got %v, asked %d", err, asked.Load())
	}
}

func TestFlareSolverr_KeepsTheSourceAddressRule(t *testing.T) {
	var direct, asked atomic.Int32
	site := checkedSite(t, &direct)
	c := New(Config{URL: site, AllowLocal: true, Format: "site", FlareSolverr: fakeFlare(t, &asked, solved)})
	// The rule changes after the check page came back: a Source that may not
	// reach the local network never has FlareSolverr open a local page.
	c.cfg.AllowLocal = false
	c.cfg.AllowAddr = func(net.IP) bool { return false }
	_, err := c.send(context.Background(), http.MethodGet, site+"/one", nil, nil)
	var se *Error
	if !errors.As(err, &se) || se.Kind != KindBlocked || asked.Load() != 0 {
		t.Fatalf("got %v, asked %d", err, asked.Load())
	}
}

func TestFlareSolverr_SendsTheSourceProxy(t *testing.T) {
	var direct, asked atomic.Int32
	site := checkedSite(t, &direct)
	var got *flareProxy
	c := New(Config{URL: site, AllowLocal: true, Format: "site", FlareSolverr: fakeFlare(t, &asked, func(a flareAsk) any {
		got = a.Proxy
		return solved(a)
	})})
	// Set after New, so the test's own calls still go straight to the fake site.
	c.cfg.Proxy, _ = url.Parse("socks5://me:secret@vpn:1080")
	if _, err := c.solve(context.Background(), site+"/one"); err != nil {
		t.Fatal(err)
	}
	if got == nil || got.URL != "socks5://vpn:1080" || got.Username != "me" || got.Password != "secret" {
		t.Fatalf("proxy sent: %+v", got)
	}
}

func TestParseFlareSolverr(t *testing.T) {
	for in, want := range map[string]string{"": "", " http://flaresolverr:8191 ": "http://flaresolverr:8191", "http://vpn:8191/v1": "http://vpn:8191",
		"https://fs.example/": "https://fs.example"} {
		if got, err := ParseFlareSolverr(in); err != nil || got != want {
			t.Errorf("%q: %q, %v", in, got, err)
		}
	}
	for _, in := range []string{"flaresolverr:8191", "ftp://x:1", "http://u:p@x:8191", "http://x:8191/other", "http://x:8191/?a=1"} {
		if _, err := ParseFlareSolverr(in); !errors.Is(err, ErrFlareSolverr) {
			t.Errorf("%q passed", in)
		}
	}
}

func TestUnwrapPre(t *testing.T) {
	page := `<html><head><meta name="color-scheme" content="light dark"></head><body><pre style="word-wrap: break-word;">{"a":"&lt;b&gt;"}</pre></body></html>`
	if got := string(unwrapPre([]byte(page))); got != `{"a":"<b>"}` {
		t.Fatalf("got %q", got)
	}
	if got := string(unwrapPre([]byte("<html><body>x</body></html>"))); got != "<html><body>x</body></html>" {
		t.Fatalf("plain page changed: %q", got)
	}
}

func TestFlareSolverr_SavedOnTheSourceAndUsed(t *testing.T) {
	addr := " http://flaresolverr:8191/ "
	var src domain.Source
	if _, _, err := fill(Input{Name: "1337x", Format: "site", Site: "knaben", FlareSolverr: &addr}, &src, true); err != nil {
		t.Fatal(err)
	}
	if src.FlareSolverrURL != "http://flaresolverr:8191" {
		t.Fatalf("stored %q", src.FlareSolverrURL)
	}
	cfg, err := ConfigFor(context.Background(), src, nil)
	if err != nil || cfg.FlareSolverr == nil || cfg.FlareSolverr.String() != "http://flaresolverr:8191" {
		t.Fatalf("config %+v, %v", cfg.FlareSolverr, err)
	}
	if v := SourceViewOf(src, Summary{}); v.FlareSolverr != "http://flaresolverr:8191" {
		t.Fatalf("view %q", v.FlareSolverr)
	}
	// Nil keeps it; empty clears it.
	if _, _, err := fill(Input{Name: "1337x"}, &src, false); err != nil || src.FlareSolverrURL == "" {
		t.Fatalf("kept %q, %v", src.FlareSolverrURL, err)
	}
	none := ""
	if _, _, err := fill(Input{Name: "1337x", FlareSolverr: &none}, &src, false); err != nil || src.FlareSolverrURL != "" {
		t.Fatalf("cleared %q, %v", src.FlareSolverrURL, err)
	}
	bad := "flaresolverr"
	if _, _, err := fill(Input{Name: "1337x", FlareSolverr: &bad}, &src, false); !errors.Is(err, ErrFlareSolverr) {
		t.Fatalf("bad address: %v", err)
	}
}

func TestFlareSolverr_RedirectToAnotherSiteIsRefused(t *testing.T) {
	var direct, asked atomic.Int32
	site := checkedSite(t, &direct)
	c := New(Config{URL: site, AllowLocal: true, Format: "site", FlareSolverr: fakeFlare(t, &asked, func(a flareAsk) any {
		ans := solved(a).(map[string]any)
		ans["solution"].(map[string]any)["url"] = "https://elsewhere.example/landing"
		return ans
	})})
	_, err := c.send(context.Background(), http.MethodGet, site+"/one", nil, nil)
	var se *Error
	if !errors.As(err, &se) || se.Kind != KindBlocked || !strings.Contains(se.Msg, "another site") {
		t.Fatalf("got %v", err)
	}
	if c.clearanceFor(strings.TrimPrefix(site, "http://")) != nil || c.pass != nil {
		t.Fatal("cookies from another site were kept")
	}
}

func TestFlareSolverr_RedirectToAPrivateAddressIsRefused(t *testing.T) {
	var direct, asked atomic.Int32
	site := checkedSite(t, &direct)
	u, _ := url.Parse(site)
	c := New(Config{URL: site, AllowLocal: true, Format: "site", FlareSolverr: fakeFlare(t, &asked, func(a flareAsk) any {
		ans := solved(a).(map[string]any)
		// The same name, which now leads home.
		ans["solution"].(map[string]any)["url"] = "http://www.site.example:" + u.Port() + "/one"
		return ans
	})})
	// The site's own name looks up as public; its www. name as a home address.
	c.cfg.AllowLocal = false
	c.cfg.AllowAddr = func(ip net.IP) bool { return !ip.IsPrivate() && !ip.IsLoopback() }
	c.cfg.resolve = func(_ context.Context, host string) ([]net.IP, error) {
		if strings.HasPrefix(host, "www.") {
			return []net.IP{net.ParseIP("192.168.1.10")}, nil
		}
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	}
	_, err := c.solve(context.Background(), "http://site.example:"+u.Port()+"/one")
	var se *Error
	if !errors.As(err, &se) || se.Kind != KindBlocked || asked.Load() != 1 || c.pass != nil {
		t.Fatalf("got %v, asked %d, kept %+v", err, asked.Load(), c.pass)
	}
}

func TestSameSite(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{{"1337x.to", "www.1337x.to", true}, {"WWW.Site.example.", "site.example", true}, {"x1337x.ws", "1337x.to", false}, {"", "a", false}} {
		if got := sameSite(c.a, c.b); got != c.want {
			t.Errorf("%q %q: %v", c.a, c.b, got)
		}
	}
}

// FlareSolverr may take longer over a bot check than the Source's own
// timeout, which is for the calls to the site itself.
func TestFlareSolverr_CallWaitsForTheCheck(t *testing.T) {
	var direct, asked atomic.Int32
	site := checkedSite(t, &direct)
	c := New(Config{URL: site, AllowLocal: true, Format: "site", Timeout: 300 * time.Millisecond, FlareSolverr: fakeFlare(t, &asked, func(a flareAsk) any {
		time.Sleep(900 * time.Millisecond)
		return solved(a)
	})})
	body, err := c.send(context.Background(), http.MethodGet, site+"/one", nil, nil)
	if err != nil || !strings.Contains(string(body), "solved /one") {
		t.Fatalf("got %q, %v", body, err)
	}
}

// Two searches of one Source share a Client, so the cookie FlareSolverr got
// and the Source's limits hold, whatever the Source's own timeout is.
func TestFlareSolverr_SearcherKeepsTheClient(t *testing.T) {
	for _, ms := range []int{0, 20000, 120000} {
		var direct, asked atomic.Int32
		site := serve(t, func(w http.ResponseWriter, r *http.Request) {
			direct.Add(1)
			if strings.Contains(r.Header.Get("Cookie"), "cf_clearance=ok") && r.Header.Get("User-Agent") == "Browser/1" {
				_, _ = w.Write([]byte(`{"torrents":[]}`))
				return
			}
			w.Header().Set("Server", "cloudflare")
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("<html><head><title>Just a moment...</title></head></html>"))
		})
		flare := fakeFlare(t, &asked, func(a flareAsk) any {
			out := solved(a).(map[string]any)
			out["solution"].(map[string]any)["response"] = `<html><head></head><body><pre>{"torrents":[]}</pre></body></html>`
			return out
		})
		st, _ := openStore(t)
		sr := &Searcher{Store: st}
		src := domain.Source{Meta: domain.Meta{ID: 1}, Name: "Lab", Protocol: "torrent", Format: "site", URL: site.URL, AllowLocal: true,
			Extra: map[string]string{extraSiteKind: "torrentscsv"}, Limits: domain.SourceLimits{TimeoutMS: ms}, FlareSolverrURL: flare.String()}
		var used []*Client
		for i := range 2 {
			c, err := sr.client(context.Background(), src)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := c.Search(context.Background(), Query{Title: "Dune"}); err != nil {
				t.Fatalf("timeout %d, search %d: %v", ms, i+1, err)
			}
			used = append(used, c)
		}
		if used[0] != used[1] || asked.Load() != 1 || direct.Load() != 2 {
			t.Errorf("timeout %d: same Client %v, FlareSolverr asked %d, site asked %d", ms, used[0] == used[1], asked.Load(), direct.Load())
		}
	}
}

// A plugin Source's address is no page on the web, so its answer is never
// taken to FlareSolverr, however much it looks like a bot check.
func TestFlareSolverr_PluginSourceIsNotHandedOver(t *testing.T) {
	old := PluginTransport
	t.Cleanup(func() { PluginTransport = old })
	PluginTransport = func(string) http.RoundTripper {
		return rtFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusForbidden, Status: "403 Forbidden", Header: http.Header{"Server": {"cloudflare"}, "Content-Type": {"text/html"}},
				Body: io.NopCloser(strings.NewReader("<html><head><title>Just a moment...</title></head></html>")), Request: r}, nil
		})
	}
	var asked atomic.Int32
	// The plugin's id may well be a name that resolves, so the address rule alone does not hold FlareSolverr back.
	c := New(Config{URL: "plugin://lab", Format: "torznab", AllowLocal: true, FlareSolverr: fakeFlare(t, &asked, solved),
		resolve: func(context.Context, string) ([]net.IP, error) { return []net.IP{net.IPv4(203, 0, 113, 7)}, nil }})
	_, err := c.Caps(context.Background())
	var se *Error
	if !errors.As(err, &se) || se.Detail != BlockBotCheck || asked.Load() != 0 {
		t.Fatalf("got %v, asked %d", err, asked.Load())
	}
}
