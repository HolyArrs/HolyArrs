package api

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// fakeYouTube plays YouTube's video servers for the relay: every
// *.googlevideo.com address is sent to it. release holds the second half
// of the segment until the test has read the first half.
func fakeYouTube(t *testing.T) (release chan struct{}) {
	t.Helper()
	release = make(chan struct{})
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/hls_variant/"):
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			_, _ = w.Write([]byte("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1000,CODECS=\"avc1.4d401f,mp4a.40.2\"\nhttps://manifest.googlevideo.com/api/manifest/hls_playlist/id/x/itag/95/index.m3u8\n"))
		case strings.Contains(r.URL.Path, "/hls_playlist/"):
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			_, _ = w.Write([]byte("#EXTM3U\n#EXT-X-MAP:URI=\"https://rr1---sn-abc.googlevideo.com/videoplayback/init\"\n#EXTINF:5,\nhttps://rr1---sn-abc.googlevideo.com/videoplayback/sq/0\n#EXT-X-ENDLIST\n"))
		case r.URL.Path == "/videoplayback/sq/0":
			w.Header().Set("Content-Type", "video/mp2t")
			if r.Header.Get("Range") == "bytes=0-3" {
				w.Header().Set("Content-Range", "bytes 0-3/8")
				w.WriteHeader(http.StatusPartialContent)
				_, _ = w.Write([]byte("AAAA"))
				return
			}
			_, _ = w.Write([]byte("AAAA\n"))
			w.(http.Flusher).Flush()
			select {
			case <-release:
			case <-time.After(5 * time.Second):
			}
			_, _ = w.Write([]byte("BBBB\n"))
		case r.URL.Path == "/evil/index.m3u8":
			_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:5,\nhttps://evil.example.com/seg.ts\n"))
		default:
			w.WriteHeader(http.StatusForbidden)
		}
	})
	yt := httptest.NewTLSServer(h)
	t.Cleanup(yt.Close)
	tr := yt.Client().Transport.(*http.Transport).Clone()
	tr.TLSClientConfig.InsecureSkipVerify = true // the fake's certificate names 127.0.0.1
	tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, _, _ := net.SplitHostPort(addr)
		if !strings.HasSuffix(host, ".googlevideo.com") {
			t.Errorf("relay dialled %s", addr)
		}
		return (&net.Dialer{}).DialContext(ctx, network, yt.Listener.Addr().String())
	}
	old := trailerProxyClient
	trailerProxyClient = &http.Client{Transport: tr, CheckRedirect: old.CheckRedirect}
	t.Cleanup(func() { trailerProxyClient = old })
	return release
}

func proxyRig(t *testing.T) (*httptest.Server, *API) {
	t.Helper()
	fakeYTDLP(t, "ok")
	_, st := discoverRig(t, trailerTMDB(t))
	a := New(Server{Store: st, Metadata: trailerTMDB(t), Version: "test"})
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	return srv, a
}

// route: GET /api/v1/trailer/stream/{id}/manifest.m3u8
// route: GET /api/v1/trailer/stream/{id}/seg/{n}
func TestTrailerPassThrough(t *testing.T) {
	release := fakeYouTube(t)
	srv, _ := proxyRig(t)
	var s TrailerStream
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/trailer/movie/11", "", 200), &s)
	if !strings.HasPrefix(s.ProxyURL, "/api/v1/trailer/stream/") || !strings.HasSuffix(s.ProxyURL, "/manifest.m3u8") {
		t.Fatalf("proxy_url %q", s.ProxyURL)
	}
	base := srv.URL + strings.TrimSuffix(s.ProxyURL, "manifest.m3u8")

	// The manifest's addresses point at the relay, never at YouTube.
	res, err := http.Get(srv.URL + s.ProxyURL)
	if err != nil {
		t.Fatal(err)
	}
	body := readAll(t, res)
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != m3u8Type || res.Header.Get("Cache-Control") != "private, no-store" ||
		strings.Contains(body, "googlevideo") || !strings.Contains(body, "\nseg/0\n") {
		t.Fatalf("%d %v\n%s", res.StatusCode, res.Header, body)
	}
	// The variant playlist: its segment and EXT-X-MAP become numbers beside it.
	list := string(do(t, "GET", base+"seg/0", "", 200))
	if strings.Contains(list, "googlevideo") || !strings.Contains(list, `URI="1"`) || !strings.Contains(list, "\n2\n") {
		t.Fatalf("playlist\n%s", list)
	}
	// The segment streams: the first half arrives before YouTube sends the rest.
	res, err = http.Get(base + "seg/2")
	if err != nil {
		t.Fatal(err)
	}
	if res.Header.Get("Content-Type") != "video/mp2t" || res.Header.Get("Cache-Control") != "private, no-store" {
		t.Fatalf("headers %v", res.Header)
	}
	rd := bufio.NewReader(res.Body)
	if first, _ := rd.ReadString('\n'); first != "AAAA\n" {
		t.Fatalf("first %q", first)
	}
	close(release)
	if second, _ := rd.ReadString('\n'); second != "BBBB\n" {
		t.Fatalf("second %q", second)
	}
	res.Body.Close()
	// Range is passed on.
	req, _ := http.NewRequest("GET", base+"seg/2", nil)
	req.Header.Set("Range", "bytes=0-3")
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if b := readAll(t, res); res.StatusCode != 206 || b != "AAAA" || res.Header.Get("Content-Range") != "bytes 0-3/8" {
		t.Fatalf("range %d %q %v", res.StatusCode, b, res.Header)
	}
	// YouTube refusing (the init address answers 403 here) is a 502.
	do(t, "GET", base+"seg/1", "", 502)
}

func TestTrailerPassThroughRefusals(t *testing.T) {
	fakeYouTube(t)
	srv, a := proxyRig(t)
	var s TrailerStream
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/trailer/movie/11", "", 200), &s)
	base := srv.URL + strings.TrimSuffix(s.ProxyURL, "manifest.m3u8")

	// An id HolyArrs never handed out, and a number never seen.
	do(t, "GET", srv.URL+"/api/v1/trailer/stream/0123456789abcdef0123456789abcdef/manifest.m3u8", "", 404)
	do(t, "GET", base+"seg/0", "", 404)
	do(t, "GET", base+"seg/x", "", 404)
	// Another person's stream.
	other := a.trailerRelay.register(4242, "https://manifest.googlevideo.com/api/manifest/hls_variant/other/index.m3u8", time.Now().Add(time.Hour))
	do(t, "GET", srv.URL+"/api/v1/trailer/stream/"+other+"/manifest.m3u8", "", 404)
	// A foreign host is never taken in, and a playlist naming one is refused.
	if id := a.trailerRelay.register(0, "https://evil.example.com/hls_variant/index.m3u8", time.Now().Add(time.Hour)); id != "" {
		t.Fatalf("foreign manifest registered as %s", id)
	}
	for _, raw := range []string{"http://manifest.googlevideo.com/x", "https://googlevideo.com.evil.example/x", "https://127.0.0.1/x"} {
		if googlevideo(raw) {
			t.Fatalf("%s allowed", raw)
		}
	}
	evil := a.trailerRelay.register(a.person(httptest.NewRequest("GET", "/", nil)).UserID, "https://manifest.googlevideo.com/evil/index.m3u8", time.Now().Add(time.Hour))
	do(t, "GET", srv.URL+"/api/v1/trailer/stream/"+evil+"/manifest.m3u8", "", 502)
	base2, _ := url.Parse("https://manifest.googlevideo.com/a/b.m3u8")
	if _, err := rewriteM3U8([]byte("#EXTM3U\n#EXT-X-MEDIA:TYPE=AUDIO,URI=\"https://evil.example.com/a.m3u8\"\n"), base2, "seg/", func(string) (int, bool) { return 0, true }); err == nil {
		t.Fatal("foreign URI attribute kept")
	}
	if _, err := rewriteM3U8([]byte("<html>"), base2, "seg/", func(string) (int, bool) { return 0, true }); err == nil {
		t.Fatal("not a playlist kept")
	}
	// Expired streams are gone.
	old := a.trailerRelay.register(0, "https://manifest.googlevideo.com/api/manifest/hls_variant/old/index.m3u8", time.Now().Add(-time.Minute))
	do(t, "GET", srv.URL+"/api/v1/trailer/stream/"+old+"/manifest.m3u8", "", 404)
}

// A person may have proxyStreams streams going at once; an idle one frees
// its place.
func TestTrailerPassThroughCap(t *testing.T) {
	var p trailerProxy
	exp := time.Now().Add(time.Hour)
	ids := []string{}
	for _, m := range []string{"a", "b", "c", "d", "e"} {
		ids = append(ids, p.register(7, "https://manifest.googlevideo.com/api/manifest/hls_variant/"+m+"/index.m3u8", exp))
	}
	now := time.Now()
	for _, id := range ids[:proxyStreams] {
		s, err := p.begin(7, id, now)
		if err != nil {
			t.Fatal(err)
		}
		p.done(7, s, 10)
	}
	// The stream past proxyStreams waits.
	if _, err := p.begin(7, ids[len(ids)-1], now); err != errProxyBusy {
		t.Fatalf("stream past the cap: %v", err)
	}
	if s, err := p.begin(8, p.register(8, "https://manifest.googlevideo.com/api/manifest/hls_variant/a/index.m3u8", exp), now); err != nil {
		t.Fatalf("another person: %v", err)
	} else {
		p.done(8, s, 0)
	}
	// The others go quiet: the last may start.
	if s, err := p.begin(7, ids[len(ids)-1], now.Add(proxyIdle+time.Second)); err != nil {
		t.Fatalf("after idle: %v", err)
	} else {
		p.done(7, s, 0)
	}
	// In-flight requests on one stream are capped too.
	var held []*proxyStream
	for range proxyRequests {
		s, err := p.begin(9, p.register(9, "https://manifest.googlevideo.com/api/manifest/hls_variant/z/index.m3u8", exp), now)
		if err != nil {
			t.Fatal(err)
		}
		held = append(held, s)
	}
	if _, err := p.begin(9, p.register(9, "https://manifest.googlevideo.com/api/manifest/hls_variant/z/index.m3u8", exp), now); err != errProxyBusy {
		t.Fatalf("in flight: %v", err)
	}
	for _, s := range held {
		p.done(9, s, 0)
	}
}

// The trailer key is kept 30 days (memory and SQLite) and Explore cards
// carry it once known.
func TestTrailerKeyKept(t *testing.T) {
	fakeYTDLP(t, "ok")
	_, st := discoverRig(t, trailerTMDB(t))
	a := New(Server{Store: st, Metadata: trailerTMDB(t), Version: "test"})
	ctx := context.Background()
	if k := a.knownTrailerKey(ctx, "movie", "11", "en"); k != "" {
		t.Fatalf("known before lookup: %q", k)
	}
	if _, err := a.resolveTitle(ctx, "movie", "11", "en"); err != nil {
		t.Fatal(err)
	}
	b := New(Server{Store: st, Version: "test"}) // a restart, and no metadata source at all
	if k := b.knownTrailerKey(ctx, "movie", "11", "en"); k != "trailerKey1" {
		t.Fatalf("after restart %q", k)
	}
	items := []discoverItemView{{}, {}}
	items[0].Kind, items[0].TMDBID = "movie", "11"
	items[1].Kind, items[1].TMDBID = "movie", "12"
	b.fillTrailerKeys(httptest.NewRequest("GET", "/?language=en", nil), items)
	if items[0].TrailerKey != "trailerKey1" || items[1].TrailerKey != "" {
		t.Fatalf("%+v", items)
	}
}

func readAll(t *testing.T, res *http.Response) string {
	t.Helper()
	defer res.Body.Close()
	var sb strings.Builder
	_, _ = bufio.NewReader(res.Body).WriteTo(&sb)
	return sb.String()
}

// The whole install has at most proxyMaxActive streams going, and one
// stream relays for at most proxyLife.
func TestTrailerPassThroughInstallCap(t *testing.T) {
	var p trailerProxy
	exp := time.Now().Add(time.Hour)
	now := time.Now()
	manifest := func(m string) string {
		return "https://manifest.googlevideo.com/api/manifest/hls_variant/" + m + "/index.m3u8"
	}
	for u := int64(1); u <= proxyMaxActive; u++ {
		s, err := p.begin(u, p.register(u, manifest("a"), exp), now)
		if err != nil {
			t.Fatalf("person %d: %v", u, err)
		}
		p.done(u, s, 0)
	}
	if _, err := p.begin(99, p.register(99, manifest("a"), exp), now); err != errProxyBusy {
		t.Fatalf("over the install cap: %v", err)
	}
	// Relaying without a break past proxyLife is refused...
	var q trailerProxy
	id := q.register(1, manifest("a"), exp)
	at := now
	for ; at.Sub(now) <= proxyLife; at = at.Add(proxyIdle / 2) {
		s, err := q.begin(1, id, at)
		if err != nil {
			t.Fatalf("at %v: %v", at.Sub(now), err)
		}
		q.done(1, s, 0)
		s.last = at // done stamps the wall clock
	}
	if _, err := q.begin(1, id, at); err != errProxyBusy {
		t.Fatalf("past the stream life: %v", err)
	}
	// ...but the same trailer played again later (same id) starts afresh.
	if s, err := q.begin(1, id, at.Add(proxyIdle)); err != nil {
		t.Fatalf("a new play of an old stream: %v", err)
	} else {
		q.done(1, s, 0)
	}
}

// An idle restart clears a stream's own count, never the person's hourly bytes.
func TestTrailerProxyPersonCapSurvivesIdle(t *testing.T) {
	var p trailerProxy
	now := time.Now()
	id := p.register(1, "https://rr1---sn.googlevideo.com/m.m3u8", now.Add(time.Hour))
	for i := 0; i < 5; i++ {
		s, err := p.begin(1, id, now)
		if err != nil {
			if i >= 4 {
				return
			}
			t.Fatalf("play %d refused early: %v", i, err)
		}
		p.done(1, s, proxyMaxStream)
		now = now.Add(proxyIdle + time.Second)
	}
	t.Fatal("hourly cap never stopped the replays")
}

// A stream with a request still running is not idle, however long it has been quiet.
func TestTrailerProxyBusyStreamNotFresh(t *testing.T) {
	var p trailerProxy
	now := time.Now()
	id := p.register(1, "https://rr1---sn.googlevideo.com/m.m3u8", now.Add(time.Hour))
	s, err := p.begin(1, id, now)
	if err != nil {
		t.Fatal(err)
	}
	started := s.started
	if _, err := p.begin(1, id, now.Add(proxyIdle+time.Second)); err != nil {
		t.Fatal(err)
	}
	if !s.started.Equal(started) {
		t.Fatal("a busy stream restarted its life")
	}
}
