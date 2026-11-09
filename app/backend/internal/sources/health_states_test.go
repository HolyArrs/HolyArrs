package sources

import (
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

func TestHealth_EachState(t *testing.T) {
	until := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	reason := func(s string) *string { return &s }
	cases := []struct {
		src  domain.Source
		want string
		says string
	}{
		{domain.Source{State: "enabled"}, StateWorking, "Working"},
		{domain.Source{State: "disabled"}, StateDisabled, "Paused by you"},
		{domain.Source{State: "paused", PauseReason: reason(KindLoginRefused), PausedUntil: &until}, StateAuthFailed, "turned down the key"},
		{domain.Source{State: "paused", PauseReason: reason(KindDown), PausedUntil: &until}, StateUnreachable, "could not reach"},
		{domain.Source{State: "paused", PauseReason: reason(KindRateLimited), PausedUntil: &until}, StateUnreachable, "slow down"},
		{domain.Source{State: "paused", PauseReason: reason(KindBlocked), PausedUntil: &until}, StateUnreachable, "local network"},
	}
	for _, c := range cases {
		h := HealthOf(c.src)
		if h.State != c.want || !strings.Contains(h.Text, c.says) {
			t.Errorf("%+v: got %+v", c.src.State, h)
		}
		if c.src.State == "paused" && (h.Until == nil || !h.Until.Equal(until)) {
			t.Errorf("%s: until %v", c.src.State, h.Until)
		}
	}
}

// addSource adds one more Source to a seeded store.
func addSource(t *testing.T, s *database.Store, name, u string, prio int, state string) int64 {
	t.Helper()
	src := &domain.Source{Name: name, Protocol: "torrent", Format: "torznab", URL: u, AllowLocal: true,
		UseFor: domain.SourceUse{AutoSearch: true, HandSearch: true}, Kinds: []string{"movie"},
		Categories: map[string][]int{"movie": {2000}}, Priority: prio, State: state, Extra: map[string]string{}}
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := database.Insert(tx, "sources", src, domain.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return src.ID
}

func torznab(t *testing.T, feed string, hits *atomic.Int32) *httptest.Server {
	return serve(t, func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Query().Get("t") == "caps" {
			_, _ = w.Write(file(t, "caps.xml"))
			return
		}
		_, _ = w.Write([]byte(feed))
	})
}

// Two working Sources are searched side by side in priority order. A Source
// that refuses the key shows auth_failed, one that does not answer shows
// unreachable, a paused one is left alone, and every call is recorded.
func TestHandle_SeveralSourcesStatesAndStats(t *testing.T) {
	var a, b, refused, off atomic.Int32
	srvA := torznab(t, duneFeed, &a)
	srvB := torznab(t, strings.Replace(duneFeed, "HOLYARRS", "SECOND", -1), &b)
	srvKey := serve(t, func(w http.ResponseWriter, r *http.Request) { refused.Add(1); w.WriteHeader(http.StatusUnauthorized) })
	srvOff := torznab(t, duneFeed, &off)
	dead := httptest.NewServer(http.NotFoundHandler())
	deadURL := dead.URL
	dead.Close()

	f := seed(t, srvA.URL+"/api")
	ctx := context.Background()
	idB := addSource(t, f.s, "Second", srvB.URL+"/api", 10, "enabled")
	idKey := addSource(t, f.s, "Wrong key", srvKey.URL+"/api", 30, "enabled")
	idDead := addSource(t, f.s, "Gone", deadURL+"/api", 40, "enabled")
	addSource(t, f.s, "Paused", srvOff.URL+"/api", 5, "disabled")

	sr := &Searcher{Store: f.s}
	var order []string
	sr.onAsk = func(name string) { order = append(order, name) }
	runSearch(t, f, sr)
	if strings.Join(order, ",") != "Second,Test,Wrong key,Gone" {
		t.Fatalf("asked in order %v", order)
	}
	if a.Load() == 0 || b.Load() == 0 || refused.Load() == 0 || off.Load() != 0 {
		t.Fatalf("hits a=%d b=%d refused=%d paused=%d", a.Load(), b.Load(), refused.Load(), off.Load())
	}
	var rels int
	_ = f.s.Read().QueryRow("SELECT count(DISTINCT source_id) FROM releases").Scan(&rels)
	if rels != 2 {
		t.Fatalf("releases from %d Sources, want 2", rels)
	}
	state := func(id int64) Health {
		var src domain.Source
		if err := database.Get(ctx, f.s.Read(), "sources", id, &src); err != nil {
			t.Fatal(err)
		}
		return HealthOf(src)
	}
	if h := state(idKey); h.State != StateAuthFailed {
		t.Fatalf("wrong key: %+v", h)
	}
	if h := state(idDead); h.State != StateUnreachable || h.Until == nil {
		t.Fatalf("gone: %+v", h)
	}
	if h := state(idB); h.State != StateWorking {
		t.Fatalf("second: %+v", h)
	}

	sum, err := Summarise(ctx, f.s.Read(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if s := sum[idB]; s.Calls != 2 || s.Failures != 0 || s.LastSuccess == nil {
		t.Fatalf("second summary %+v", s)
	}
	if s := sum[idKey]; s.Failures != 1 || s.LastFailure == nil || s.LastSuccess != nil {
		t.Fatalf("wrong key summary %+v", s)
	}
	st, err := StatsFor(ctx, f.s.Read(), f.srcID, 7, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Days) != 7 || st.Days[6].Calls != 2 || len(st.Recent) != 2 || st.Recent[0].Op != "search" {
		t.Fatalf("stats %+v", st)
	}
	stB, _ := StatsFor(ctx, f.s.Read(), idB, 7, time.Now())
	grabs := 0
	for i := range st.Days {
		grabs += st.Days[i].Grabs + stB.Days[i].Grabs
	}
	if grabs != 1 {
		t.Fatalf("grabs %d, want 1 (the chosen Release)", grabs)
	}
}

const plainRSS = `<?xml version="1.0"?><rss version="2.0"><channel><title>lab</title>
<item><title>Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS</title><link>http://feed/dl/1.torrent</link>
<enclosure url="http://feed/dl/1.torrent" length="1000" type="application/x-bittorrent"/></item>
<item><title>No link</title></item>
</channel></rss>`

func TestFeed_RSSReadWithSecretQuery(t *testing.T) {
	var got string
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.RawQuery
		_, _ = w.Write([]byte(plainRSS))
	})
	query := "passkey=" + "abc123"
	c := New(Config{URL: srv.URL + "/rss", APIKey: query, Format: "rss", Protocol: "torrent", AllowLocal: true})
	items, err := c.Feed(context.Background(), []int{2000})
	if err != nil {
		t.Fatal(err)
	}
	if got != "passkey=abc123" {
		t.Fatalf("query %q: an rss feed is read as it is", got)
	}
	if len(items) != 1 || items[0].Protocol != "torrent" || items[0].Size != 1000 {
		t.Fatalf("items %+v", items)
	}
	if n, err := c.Test(context.Background()); err != nil || n != 1 {
		t.Fatalf("test %d %v", n, err)
	}
}

func TestFeed_ParserRefusesJunk(t *testing.T) {
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("<html>not a feed")) })
	_, err := New(Config{URL: srv.URL, Format: "rss", AllowLocal: true}).Feed(context.Background(), nil)
	var se *Error
	if !errors.As(err, &se) || se.Kind != KindBroken {
		t.Fatalf("got %v", err)
	}
}

func TestProxy_HTTPCarriesSource(t *testing.T) {
	var srcHits atomic.Int32
	src := torznab(t, duneFeed, &srcHits)
	addr, seen := connectProxy(t, strings.TrimPrefix(src.URL, "http://"))
	pu, _ := url.Parse("http://" + addr)
	_, err := New(Config{URL: src.URL + "/api", AllowLocal: true, Proxy: pu}).Caps(context.Background())
	if err == nil || !strings.Contains(err.Error(), "network policy") {
		t.Fatalf("local proxy without leave: %v", err)
	}
	c := New(Config{URL: src.URL + "/api", AllowLocal: true, Proxy: pu, ProxyAllowLocal: true})
	if _, err := c.Caps(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := seen.Load("connect"); !ok || srcHits.Load() != 1 {
		t.Fatalf("proxy connected %v source %d", ok, srcHits.Load())
	}
	seen.Delete("connect")
	_, err = New(Config{URL: src.URL + "/api", Proxy: pu, ProxyAllowLocal: true}).Caps(context.Background())
	var se *Error
	_, contacted := seen.Load("connect")
	if !errors.As(err, &se) || se.Kind != KindBlocked || contacted {
		t.Fatalf("local Source through proxy: %v (proxy contacted %v)", err, contacted)
	}
}

func TestProxy_PasswordAndKeyNeverInErrors(t *testing.T) {
	dead := httptest.NewServer(http.NotFoundHandler())
	addr := dead.URL
	dead.Close()
	pu, _ := url.Parse(strings.Replace(addr, "http://", "http://me:pr0xy-secret@", 1))
	stand := "k3y-" + "secret"
	c := New(Config{URL: "http://example.invalid/api", APIKey: stand, Proxy: pu, ProxyAllowLocal: true})
	_, err := c.Caps(context.Background())
	if err == nil || strings.Contains(err.Error(), "pr0xy-secret") || strings.Contains(err.Error(), "k3y-secret") {
		t.Fatalf("error %v", err)
	}
	if msg := c.scrub("a pr0xy-secret and k3y-secret"); strings.Contains(msg, "secret") {
		t.Fatalf("scrub %q", msg)
	}
}

// socksServer is a tiny SOCKS5 proxy, with a name and password or none.
func socksServer(t *testing.T, user, pass string, hits *atomic.Int32) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				head := make([]byte, 2)
				if _, err := io.ReadFull(c, head); err != nil {
					return
				}
				methods := make([]byte, head[1])
				_, _ = io.ReadFull(c, methods)
				if user != "" {
					_, _ = c.Write([]byte{5, 2})
					b := make([]byte, 2)
					_, _ = io.ReadFull(c, b)
					u := make([]byte, b[1])
					_, _ = io.ReadFull(c, u)
					_, _ = io.ReadFull(c, b[:1])
					p := make([]byte, b[0])
					_, _ = io.ReadFull(c, p)
					if string(u) != user || string(p) != pass {
						_, _ = c.Write([]byte{1, 1})
						return
					}
					_, _ = c.Write([]byte{1, 0})
				} else {
					_, _ = c.Write([]byte{5, 0})
				}
				req := make([]byte, 4)
				if _, err := io.ReadFull(c, req); err != nil {
					return
				}
				var host string
				switch req[3] {
				case 1:
					ip := make([]byte, 4)
					_, _ = io.ReadFull(c, ip)
					host = net.IP(ip).String()
				case 3:
					n := make([]byte, 1)
					_, _ = io.ReadFull(c, n)
					h := make([]byte, n[0])
					_, _ = io.ReadFull(c, h)
					host = string(h)
				}
				pb := make([]byte, 2)
				_, _ = io.ReadFull(c, pb)
				up, err := net.Dial("tcp", net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(pb)))))
				if err != nil {
					_, _ = c.Write([]byte{5, 5, 0, 1, 0, 0, 0, 0, 0, 0})
					return
				}
				defer up.Close()
				hits.Add(1)
				_, _ = c.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0})
				go func() { _, _ = io.Copy(up, c) }()
				_, _ = io.Copy(c, up)
			}(c)
		}
	}()
	return ln.Addr().String()
}

func TestProxy_SOCKS5CarriesSource(t *testing.T) {
	var hits, srcHits atomic.Int32
	src := torznab(t, duneFeed, &srcHits)
	addr := socksServer(t, "lab", "lab-pass", &hits)
	pu, _ := url.Parse("socks5://lab:lab-pass@" + addr)
	c := New(Config{URL: src.URL + "/api", AllowLocal: true, Proxy: pu, ProxyAllowLocal: true})
	if _, err := c.Caps(context.Background()); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 1 || srcHits.Load() != 1 {
		t.Fatalf("socks %d source %d", hits.Load(), srcHits.Load())
	}
	bad, _ := url.Parse("socks5://lab:wrong@" + addr)
	if _, err := New(Config{URL: src.URL + "/api", AllowLocal: true, Proxy: bad, ProxyAllowLocal: true}).Caps(context.Background()); err == nil {
		t.Fatal("a wrong proxy password should fail")
	}
}

func openVault(t *testing.T) (*database.Store, *secrets.Vault) {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	v, err := secrets.Open(context.Background(), s, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	return s, v
}

func TestSave_KeepsKeysInVault(t *testing.T) {
	labKey := "lab-" + "key-1"
	ctx := context.Background()
	s, v := openVault(t)
	by := domain.System
	src, err := Add(ctx, s, v, Input{Name: "Lab", Format: "torznab", URL: "http://lab.example/api", APIKey: labKey,
		Proxy: ptr("socks5://me:proxy-pass@10.0.0.9:1080")}, by)
	if err != nil {
		t.Fatal(err)
	}
	if src.SecretRef == nil || src.ProxySecretRef == nil || strings.Contains(src.ProxyURL, "proxy-pass") || src.ProxyURL != "socks5://me@10.0.0.9:1080" {
		t.Fatalf("saved %+v", src)
	}
	if !src.UseFor.Feed || !src.UseFor.AutoSearch || src.Priority != 25 || src.State != "enabled" || src.MinSeeders == nil || len(src.Kinds) != 3 {
		t.Fatalf("defaults %+v", src)
	}
	cfg, err := ConfigFor(ctx, src, v)
	if err != nil || cfg.APIKey != labKey || cfg.Proxy.String() != "socks5://me:proxy-pass@10.0.0.9:1080" {
		t.Fatalf("config %+v %v", cfg, err)
	}
	if _, err := Add(ctx, s, v, Input{Name: "lab", Format: "torznab", URL: "http://x.example/api"}, by); !errors.Is(err, ErrNameUsed) {
		t.Fatalf("same name: %v", err)
	}
	if _, err := Change(ctx, s, v, src.ID, Input{Name: "Lab", URL: "http://other.example/api"}, by); !errors.Is(err, ErrKeyAgain) {
		t.Fatalf("new host: %v", err)
	}
	if src, err = Change(ctx, s, v, src.ID, Input{Name: "Lab", URL: "http://lab.example/torznab/api", Priority: ptr(3)}, by); err != nil || src.Priority != 3 {
		t.Fatalf("same host: %v %+v", err, src)
	}
	if _, err := Change(ctx, s, v, src.ID, Input{Name: "Lab", Proxy: ptr("http://elsewhere.example:3128")}, by); !errors.Is(err, ErrProxyAgain) {
		t.Fatalf("new proxy host: %v", err)
	}
	if src, err = Change(ctx, s, v, src.ID, Input{Name: "Lab", Proxy: ptr("")}, by); err != nil || src.ProxyURL != "" || src.ProxySecretRef != nil {
		t.Fatalf("drop proxy: %v %+v", err, src)
	}
	for _, bad := range []Input{
		{Name: "", Format: "torznab", URL: "http://x/api"},
		{Name: "a", Format: "html", URL: "http://x/api"},
		{Name: "a", Format: "torznab", URL: "ftp://x/api"},
		{Name: "a", Format: "torznab", URL: "http://x/api", Priority: ptr(0)},
		{Name: "a", Format: "torznab", URL: "http://x/api", Proxy: ptr("socks4://x:1")},
		{Name: "a", Format: "torznab", URL: "http://x/api", Kinds: []string{"books"}},
		{Name: "a", Format: "rss", URL: "http://x/rss", UseFor: &domain.SourceUse{AutoSearch: true}},
	} {
		if _, err := Add(ctx, s, v, bad, by); err == nil {
			t.Errorf("%+v was saved", bad)
		}
	}

	r, err := Add(ctx, s, v, Input{Name: "Feed", Format: "rss", URL: "http://feed.example/rss?passkey=zz9"}, by)
	if err != nil || r.URL != "http://feed.example/rss" || r.SecretRef == nil || r.UseFor.AutoSearch || !r.UseFor.Feed {
		t.Fatalf("rss %+v %v", r, err)
	}
	if k, _ := v.Get(ctx, *r.SecretRef); k != "passkey=zz9" {
		t.Fatalf("rss key %q", k)
	}

	p, err := SetPaused(ctx, s, src.ID, true, by)
	if err != nil || HealthOf(p).State != StateDisabled {
		t.Fatalf("pause %+v %v", p, err)
	}
	p, err = SetPaused(ctx, s, src.ID, false, by)
	if err != nil || HealthOf(p).State != StateWorking {
		t.Fatalf("resume %+v %v", p, err)
	}

	ref := *src.SecretRef
	if err := Remove(ctx, s, src.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Get(ctx, ref); err == nil {
		t.Fatal("key kept after remove")
	}
}

func TestExplain_PlainWords(t *testing.T) {
	if got := Explain(&Error{Kind: KindLoginRefused, Msg: "401 Unauthorized"}); !strings.Contains(got, "turned down the key") {
		t.Fatal(got)
	}
	if got := Explain(&Error{Kind: KindBlocked}); !strings.Contains(got, "local network") {
		t.Fatal(got)
	}
	if got := Explain(errors.New("x")); !strings.Contains(got, "could not reach") {
		t.Fatal(got)
	}
}

func TestHealth_UpdatesKeepChangedSource(t *testing.T) {
	ctx := context.Background()
	s, v := openVault(t)
	by := domain.System
	stale, err := Add(ctx, s, v, Input{Name: "Lab", Format: "torznab", URL: "http://lab.example/api", APIKey: "first-" + "key",
		Proxy: ptr("socks5://me:proxy-pass@10.0.0.9:1080")}, by)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Change(ctx, s, v, stale.ID, Input{Name: "Lab", APIKey: "second-" + "key", Proxy: ptr("socks5://me:new-pass@10.0.0.9:1080")}, by); err != nil {
		t.Fatal(err)
	}
	sr := &Searcher{Store: s}
	sr.pause(ctx, stale, &Error{Kind: KindDown, Msg: "x"})
	var got domain.Source
	_ = database.Get(ctx, s.Read(), "sources", stale.ID, &got)
	if got.State != "paused" || got.Extra["failures"] != "1" {
		t.Fatalf("not paused: %+v", got)
	}
	cfg, err := ConfigFor(ctx, got, v)
	if err != nil || cfg.APIKey != "second-key" || cfg.Proxy.String() != "socks5://me:new-pass@10.0.0.9:1080" {
		t.Fatalf("after pause: %+v %v", cfg, err)
	}
	sr.pause(ctx, stale, &Error{Kind: KindDown, Msg: "x"})
	_ = database.Get(ctx, s.Read(), "sources", stale.ID, &got)
	if got.Extra["failures"] != "2" {
		t.Fatalf("failures counted from the stale copy: %q", got.Extra["failures"])
	}
	sr.recovered(ctx, stale)
	_ = database.Get(ctx, s.Read(), "sources", stale.ID, &got)
	if got.State != "enabled" || got.Extra["failures"] != "" || got.PauseReason != nil {
		t.Fatalf("not recovered: %+v", got)
	}
	if cfg, err = ConfigFor(ctx, got, v); err != nil || cfg.APIKey != "second-key" {
		t.Fatalf("after recovered: %+v %v", cfg, err)
	}
	if _, err := SetPaused(ctx, s, stale.ID, true, by); err != nil {
		t.Fatal(err)
	}
	stale.State = "paused"
	sr.recovered(ctx, stale)
	sr.pause(ctx, stale, &Error{Kind: KindDown, Msg: "x"})
	_ = database.Get(ctx, s.Read(), "sources", stale.ID, &got)
	if got.State != "disabled" || got.PauseReason != nil {
		t.Fatalf("disabled Source changed: %+v", got)
	}
}
