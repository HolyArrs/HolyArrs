package sources

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// siteServer serves one saved fixture and records each request.
func siteServer(t *testing.T, fixture string, check func(r *http.Request, body string)) *httptest.Server {
	t.Helper()
	data, err := os.ReadFile("testdata/sites/" + fixture)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		check(r, string(b))
		_, _ = w.Write(data)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func siteClient(url, site, key string) *Client {
	return New(Config{URL: url, Format: "site", Site: site, Protocol: "torrent", AllowLocal: true, APIKey: key})
}

func TestSiteText(t *testing.T) {
	t.Parallel()
	cases := []struct {
		q    Query
		want string
	}{
		{Query{Title: "Big Buck Bunny", Year: 2008}, "Big Buck Bunny 2008"},
		{Query{Title: "Show", TV: true, Season: 1, Episode: "2"}, "Show S01E02"},
		{Query{Title: "Show", TV: true, Season: 3}, "Show S03"},
		{Query{Title: "Daily", TV: true, Season: 2026, Episode: "11/12"}, "Daily 2026 11 12"},
		{Query{Music: true, Artist: "Artist", Album: "Album"}, "Artist Album"},
		{Query{feed: true, Title: "x"}, ""},
	}
	for _, c := range cases {
		if got := siteText(c.q); got != c.want {
			t.Errorf("siteText(%+v) = %q, want %q", c.q, got, c.want)
		}
	}
}

func TestKnabenSearch(t *testing.T) {
	t.Parallel()
	srv := siteServer(t, "knaben.json", func(r *http.Request, body string) {
		var b map[string]any
		_ = json.Unmarshal([]byte(body), &b)
		if r.Method != http.MethodPost || r.URL.Path != "/v1" || b["query"] != "Big Buck Bunny 2008" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("request %s %s %s", r.Method, r.URL.Path, body)
		}
		if cats, _ := b["categories"].([]any); len(cats) != 1 || cats[0] != float64(3000000) {
			t.Errorf("categories %v", b["categories"])
		}
	})
	items, err := siteClient(srv.URL, "knaben", "").Search(context.Background(), Query{Title: "Big Buck Bunny", Year: 2008})
	if err != nil || len(items) != 3 {
		t.Fatalf("items %d err %v", len(items), err)
	}
	it := items[0]
	if it.Title != "Big Buck Bunny (2008) 720p Bluray nHD x264-NhaNc3" || it.Size != 219538056 || it.Seeders == nil ||
		!strings.HasPrefix(it.Link, "magnet:") || it.InfoHash != "a9df29de659e590a8ad3cb084407be38ec2ce4f2" || it.Published.IsZero() {
		t.Fatalf("item %+v", it)
	}
}

func TestTorrentsCSVSearch(t *testing.T) {
	t.Parallel()
	var calls int
	srv := siteServer(t, "torrentscsv.json", func(r *http.Request, _ string) {
		calls++
		if q := r.URL.Query().Get("q"); r.URL.Path != "/service/search" || q != "Show S01E02" && q != "linux" {
			t.Errorf("request %s", r.URL)
		}
	})
	c := siteClient(srv.URL, "torrentscsv", "")
	items, err := c.Search(context.Background(), Query{Title: "Show", TV: true, Season: 1, Episode: "2"})
	if err != nil || len(items) != 1 {
		t.Fatalf("items %v err %v", items, err)
	}
	it := items[0]
	if it.InfoHash != "dd8255ecdc7ca55fb0bbf81323d87062db1f6d1c" || *it.Seeders != 348 || *it.Peers != 403 || it.Size != 276445467 ||
		!strings.HasPrefix(it.Link, "magnet:?xt=urn:btih:dd8255") {
		t.Fatalf("item %+v", it)
	}
	// The site has no newest list, so the feed is empty and makes no call.
	if feed, err := c.Feed(context.Background(), nil); err != nil || len(feed) != 0 {
		t.Fatalf("feed %v %v", feed, err)
	}
	if n, err := c.Test(context.Background()); err != nil || n != 1 || calls != 2 {
		t.Fatalf("test %d %v calls %d", n, err, calls)
	}
}

func TestSubsPleaseSearch(t *testing.T) {
	t.Parallel()
	var seen []string
	srv := siteServer(t, "subsplease.json", func(r *http.Request, _ string) { seen = append(seen, r.URL.RawQuery) })
	c := siteClient(srv.URL, "subsplease", "")
	items, err := c.Search(context.Background(), Query{Title: "Sousou no Frieren", TV: true, Season: 1, Episode: "11"})
	if err != nil || len(items) == 0 {
		t.Fatalf("items %v err %v", items, err)
	}
	for _, it := range items {
		if !strings.HasPrefix(it.Title, "[SubsPlease]") || len(it.InfoHash) != 40 || it.Size <= 0 || it.Published.IsZero() {
			t.Fatalf("item %+v", it)
		}
	}
	if _, err := c.Feed(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 2 || !strings.Contains(seen[0], "f=search") || !strings.Contains(seen[0], "s=Sousou+no+Frieren") || !strings.Contains(seen[1], "f=latest") {
		t.Fatalf("calls %v", seen)
	}
	if got, _ := parseSubsPlease([]byte("[]")); len(got) != 0 {
		t.Fatal("empty answer")
	}
}

func TestIPTorrentsCookieSearch(t *testing.T) {
	t.Parallel()
	srv := siteServer(t, "iptorrents.html", func(r *http.Request, _ string) {
		if r.URL.Path != "/t" || r.URL.Query().Get("q") != "Sample Show S01E02" || r.Header.Get("Cookie") != "uid=1234; pass=cookiepass" {
			t.Errorf("request %s cookie %q", r.URL, r.Header.Get("Cookie"))
		}
	})
	c := siteClient(srv.URL, "iptorrents", "uid=1234; pass=cookiepass; tp=passkey99")
	items, err := c.Search(context.Background(), Query{Title: "Sample Show", TV: true, Season: 1, Episode: "2"})
	if err != nil || len(items) != 2 {
		t.Fatalf("items %v err %v", items, err)
	}
	a, b := items[0], items[1]
	if a.Title != "Sample Show S01E02 1080p WEB h264-GROUP" || *a.Seeders != 1204 || *a.Peers != 1241 || a.Size < 1500000000 || a.Size > 1600000000 ||
		a.Link != srv.URL+"/download.php/1001/Sample.Show.S01E02.1080p.WEB.h264-GROUP.torrent?torrent_pass=passkey99" {
		t.Fatalf("first %+v", a)
	}
	if b.Title != "Sample Film 2008 720p BluRay x264-OTHER & Co" || *b.Seeders != 9 || b.Size != 812<<20 {
		t.Fatalf("second %+v", b)
	}
	// Stored links never keep the passkey.
	if l := hideKeys(a.Link, secretValues(c.cfg)); strings.Contains(l, "passkey99") || !strings.Contains(l, hidden) {
		t.Fatalf("hidden link %s", l)
	}
}

func TestIPTorrentsSignInPage(t *testing.T) {
	t.Parallel()
	srv := siteServer(t, "iptorrents_login.html", func(*http.Request, string) {})
	_, err := siteClient(srv.URL, "iptorrents", "uid=1; pass=old").Search(context.Background(), Query{Title: "x"})
	var se *Error
	if !errors.As(err, &se) || se.Kind != KindLoginRefused {
		t.Fatalf("err %v", err)
	}
	// No key at all fails before any call.
	_, err = siteClient(srv.URL, "iptorrents", "").Search(context.Background(), Query{Title: "x"})
	if !errors.As(err, &se) || se.Kind != KindLoginRefused {
		t.Fatalf("err %v", err)
	}
}

func TestIPTorrentsRSSKey(t *testing.T) {
	t.Parallel()
	rss := `<rss><channel><item><title>Sample Show S01E02 720p</title><link>https://example.invalid/download.php/5/x.torrent</link>` +
		`<guid>https://example.invalid/details.php?id=5</guid><description>1.2 GB; TV/x264</description></item></channel></rss>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/t.rss" || !strings.Contains(r.URL.RawQuery, "u=77;tp=rsskey1;download;q=Sample") {
			t.Errorf("request %s", r.URL)
		}
		_, _ = w.Write([]byte(rss))
	}))
	t.Cleanup(srv.Close)
	items, err := siteClient(srv.URL, "iptorrents", "u=77; tp=rsskey1").Search(context.Background(), Query{Title: "Sample"})
	if err != nil || len(items) != 1 || items[0].Protocol != "torrent" {
		t.Fatalf("items %v err %v", items, err)
	}
}

func TestMagnetHashBase32(t *testing.T) {
	t.Parallel()
	if got := magnetHash("urn:btih:VLJ3XGIWPFSZPFC3WRVIL2UCEQFHAW2B"); len(got) != 40 {
		t.Fatalf("hash %q", got)
	}
	if got := magnetHash("urn:btih:DD8255ECDC7CA55FB0BBF81323D87062DB1F6D1C"); got != "dd8255ecdc7ca55fb0bbf81323d87062db1f6d1c" {
		t.Fatalf("hash %q", got)
	}
}

// route: GET /api/v1/sources/sites
func TestSiteSourceAdd(t *testing.T) {
	t.Parallel()
	srv, _ := sourceServer(t)
	var sites struct {
		Sites []Site `json:"sites"`
	}
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/sources/sites", "", 200), &sites)
	if len(sites.Sites) != 4 {
		t.Fatalf("sites %+v", sites)
	}
	var x SourceView
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/sources", `{"name":"Knaben","format":"site","site":"knaben"}`, 201), &x)
	if x.Format != "site" || x.Site != "knaben" || x.Protocol != "torrent" || x.URL != "https://api.knaben.org" ||
		x.Limits.GapMS != 2000 || x.Limits.SearchesPerWindow != 100 || !x.UseFor.AutoSearch {
		t.Fatalf("added %+v", x)
	}
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/sources", `{"name":"SP","format":"site","site":"subsplease"}`, 201), &x)
	if len(x.Kinds) != 1 || x.Kinds[0] != "tv" {
		t.Fatalf("subsplease kinds %v", x.Kinds)
	}
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/sources", `{"name":"TCSV","format":"site","site":"torrentscsv"}`, 201), &x)
	if x.UseFor.Feed || !x.UseFor.AutoSearch || !x.UseFor.HandSearch {
		t.Fatalf("torrentscsv use_for %+v", x.UseFor)
	}
	do(t, "POST", srv.URL+"/api/v1/sources", `{"name":"Nope","format":"site","site":"other"}`, 400)
	do(t, "POST", srv.URL+"/api/v1/sources", `{"name":"Nope","format":"site","site":"knaben","protocol":"usenet"}`, 400)
	raw := string(do(t, "POST", srv.URL+"/api/v1/sources", `{"name":"IPT","format":"site","site":"iptorrents","api_key":"uid=1; pass=secretcookie"}`, 201))
	if strings.Contains(raw, "secretcookie") {
		t.Fatalf("key shown %s", raw)
	}
}

type oneSecret string

func (s oneSecret) Get(context.Context, int64) (string, error) { return string(s), nil }

// FileHeader gives the IPTorrents cookie (without the RSS key) for fetching
// a Release file, and nothing for other Sources or a key-only IPTorrents.
func TestFileHeaderIPTorrentsCookie(t *testing.T) {
	ref := int64(1)
	ipt := domain.Source{URL: "https://iptorrents.com", Format: "site", Extra: map[string]string{"site": "iptorrents"}, SecretRef: &ref}
	h, err := FileHeader(context.Background(), ipt, oneSecret("uid=1234; pass=cookiepass; tp=passkey99"))
	if err != nil || h.Get("Cookie") != "uid=1234; pass=cookiepass" {
		t.Fatalf("cookie %q, %v", h.Get("Cookie"), err)
	}
	if h, _ := FileHeader(context.Background(), ipt, oneSecret("u=1234; tp=passkey99")); h != nil {
		t.Fatalf("key only: %v", h)
	}
	knaben := domain.Source{URL: "https://knaben.org", Format: "site", Extra: map[string]string{"site": "knaben"}, SecretRef: &ref}
	if h, _ := FileHeader(context.Background(), knaben, oneSecret("uid=1; pass=x")); h != nil {
		t.Fatalf("knaben: %v", h)
	}
}
