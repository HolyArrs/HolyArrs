package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// signedVariants is how many variants the fake's freshly signed manifest lists.
const signedVariants = 400

// relayMaxURLs is how many addresses the relay keeps for one stream.
const relayMaxURLs = 5000

// fakeYouTube plays YouTube's video servers for the relay: every
// *.googlevideo.com address is sent to it. release holds the second half
// of the segment until the test has read the first half.
func fakeYouTube(t *testing.T) (release chan struct{}) {
	t.Helper()
	release = make(chan struct{})
	var signed atomic.Int32
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/hls_variant/signed/"):
			// Like YouTube: every fetch signs its addresses afresh.
			sig := signed.Add(1)
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			var sb strings.Builder
			sb.WriteString("#EXTM3U\n")
			for i := range signedVariants {
				fmt.Fprintf(&sb, "#EXT-X-STREAM-INF:BANDWIDTH=1000\nhttps://manifest.googlevideo.com/api/manifest/hls_playlist/id/x/itag/%d/sig/%d/index.m3u8\n", i, sig)
			}
			_, _ = w.Write([]byte(sb.String()))
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
	var old *http.Client
	old = metadata.UseRelayClient(nil)
	metadata.UseRelayClient(&http.Client{Transport: tr, CheckRedirect: old.CheckRedirect})
	t.Cleanup(func() { metadata.UseRelayClient(old) })
	return release
}

func proxyRig(t *testing.T) (*httptest.Server, *handlers) {
	t.Helper()
	fakeReader(t, "ok")
	_, st := trDiscoverRig(t, trailerTMDB(t))
	a := buildHandlers(services{Store: st, Metadata: trailerTMDB(t), Version: "test"})
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	if err := lookUp(t, a, "movie", "11"); err != nil {
		t.Fatal(err)
	}
	return srv, a
}

// route: GET /api/v1/trailer/stream/{id}/manifest.m3u8
// route: GET /api/v1/trailer/stream/{id}/seg/{n}
func TestTrailerPassThrough(t *testing.T) {
	release := fakeYouTube(t)
	srv, _ := proxyRig(t)
	var s metadata.TrailerStream
	_ = json.Unmarshal(trDo(t, "GET", srv.URL+"/api/v1/trailer/movie/11", "", 200), &s)
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
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != "application/vnd.apple.mpegurl" || res.Header.Get("Cache-Control") != "private, no-store" ||
		strings.Contains(body, "googlevideo") || !strings.Contains(body, "\nseg/0\n") {
		t.Fatalf("%d %v\n%s", res.StatusCode, res.Header, body)
	}
	// The variant playlist: its segment and EXT-X-MAP become numbers beside it.
	list := string(trDo(t, "GET", base+"seg/0", "", 200))
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
	trDo(t, "GET", base+"seg/1", "", 502)
}

func TestTrailerPassThroughRefusals(t *testing.T) {
	fakeYouTube(t)
	srv, a := proxyRig(t)
	var s metadata.TrailerStream
	_ = json.Unmarshal(trDo(t, "GET", srv.URL+"/api/v1/trailer/movie/11", "", 200), &s)
	base := srv.URL + strings.TrimSuffix(s.ProxyURL, "manifest.m3u8")

	// An id HolyArrs never handed out, and a number never seen.
	trDo(t, "GET", srv.URL+"/api/v1/trailer/stream/0123456789abcdef0123456789abcdef/manifest.m3u8", "", 404)
	trDo(t, "GET", base+"seg/0", "", 404)
	trDo(t, "GET", base+"seg/x", "", 404)
	// Another person's stream.
	other := a.Trailers().Relay(4242, "https://manifest.googlevideo.com/api/manifest/hls_variant/other/index.m3u8", time.Now().Add(time.Hour))
	trDo(t, "GET", srv.URL+"/api/v1/trailer/stream/"+other+"/manifest.m3u8", "", 404)
	// A foreign host is never taken in, and a playlist naming one is refused.
	if id := a.Trailers().Relay(0, "https://evil.example.com/hls_variant/index.m3u8", time.Now().Add(time.Hour)); id != "" {
		t.Fatalf("foreign manifest registered as %s", id)
	}
	for _, raw := range []string{"http://manifest.googlevideo.com/x", "https://googlevideo.com.evil.example/x", "https://127.0.0.1/x"} {
		if metadata.Googlevideo(raw) {
			t.Fatalf("%s allowed", raw)
		}
	}
	evil := a.Trailers().Relay(a.TrailerTest().Person(httptest.NewRequest("GET", "/", nil)).UserID, "https://manifest.googlevideo.com/evil/index.m3u8", time.Now().Add(time.Hour))
	trDo(t, "GET", srv.URL+"/api/v1/trailer/stream/"+evil+"/manifest.m3u8", "", 502)
	base2, _ := url.Parse("https://manifest.googlevideo.com/a/b.m3u8")
	if _, err := metadata.RewriteM3U8([]byte("#EXTM3U\n#EXT-X-MEDIA:TYPE=AUDIO,URI=\"https://evil.example.com/a.m3u8\"\n"), base2, "seg/", func(string) int { return 0 }); err == nil {
		t.Fatal("foreign URI attribute kept")
	}
	if _, err := metadata.RewriteM3U8([]byte("<html>"), base2, "seg/", func(string) int { return 0 }); err == nil {
		t.Fatal("not a playlist kept")
	}
	// Expired streams are gone.
	old := a.Trailers().Relay(0, "https://manifest.googlevideo.com/api/manifest/hls_variant/old/index.m3u8", time.Now().Add(-time.Minute))
	trDo(t, "GET", srv.URL+"/api/v1/trailer/stream/"+old+"/manifest.m3u8", "", 404)
}

// YouTube signs a manifest's addresses afresh on every fetch, so every play
// brings new ones. A stream keeps the ones used last and lets the oldest go:
// a replay is never refused, and a number let go answers 404, never another
// address.
func TestTrailerPassThroughReplays(t *testing.T) {
	fakeYouTube(t)
	srv, a := proxyRig(t)
	id := a.Trailers().Relay(a.TrailerTest().Person(httptest.NewRequest("GET", "/", nil)).UserID, "https://manifest.googlevideo.com/api/manifest/hls_variant/signed/index.m3u8", time.Now().Add(time.Hour))
	base := srv.URL + "/api/v1/trailer/stream/" + id + "/"
	trDo(t, "GET", base+"manifest.m3u8", "", 200)
	var last string
	for range 2 * relayMaxURLs / signedVariants {
		list := strings.Split(strings.TrimSpace(string(trDo(t, "GET", base+"manifest.m3u8", "", 200))), "\n")
		last = list[len(list)-1]
		// The first play's first variant stays in use.
		trDo(t, "GET", base+"seg/0", "", 200)
	}
	if n := a.Trailers().RelayAddresses(id); n > relayMaxURLs {
		t.Fatalf("stream holds %d addresses", n)
	}
	trDo(t, "GET", base+last, "", 200)
	trDo(t, "GET", base+"seg/1", "", 404)
}

// The trailer key is kept 30 days (memory and SQLite) and Explore cards
// carry it once known.
func TestTrailerKeyKept(t *testing.T) {
	fakeReader(t, "ok")
	_, st := trDiscoverRig(t, trailerTMDB(t))
	a := buildHandlers(services{Store: st, Metadata: trailerTMDB(t), Version: "test"})
	ctx := context.Background()
	if k := a.Trailers().KnownKey(ctx, "movie", "11", "en"); k != "" {
		t.Fatalf("known before lookup: %q", k)
	}
	if _, err := a.Trailers().ResolveTitle(ctx, "movie", "11", "en"); err != nil {
		t.Fatal(err)
	}
	b := buildHandlers(services{Store: st, Version: "test"}) // a restart, and no metadata source at all
	if k := b.Trailers().KnownKey(ctx, "movie", "11", "en"); k != "trailerKey1" {
		t.Fatalf("after restart %q", k)
	}
	items := []discover.ItemView{{}, {}}
	items[0].Kind, items[0].TMDBID = "movie", "11"
	items[1].Kind, items[1].TMDBID = "movie", "12"
	b.TrailerTest().FillTrailerKeys(httptest.NewRequest("GET", "/?language=en", nil), items)
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
