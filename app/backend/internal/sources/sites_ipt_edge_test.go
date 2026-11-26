package sources

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

// Edge cases for the IPTorrents key, categories and RSS reader.
func TestIPTorrentsSizeWhitespace(t *testing.T) {
	t.Parallel()
	for _, size := range []string{"1.5 GB", "1.5&nbsp;GB", "1.5\u00a0GB", "1.5\u202fGB", "1.5<br>GB"} {
		t.Run(size, func(t *testing.T) {
			page := `<table><tr><td><a class="hv" href="/t/9">Sample</a></td><td><a href="/download.php/9/sample.torrent"></a></td><td>` + size + `</td></tr></table>`
			items, err := parseIPTorrents([]byte(page), "https://example.com", "")
			if err != nil || len(items) != 1 || items[0].Size != 3<<29 {
				t.Fatalf("HTML size: %+v, %v", items, err)
			}
			feed := `<rss version="2.0"><channel><item><title>Sample</title><link>https://example.com/download.php/9/sample.torrent</link><description><![CDATA[` + size + `; TV (S:1 L:0)]]></description></item></channel></rss>`
			items, err = parseIPTorrentsFeed([]byte(feed))
			if err != nil || len(items) != 1 || items[0].Size != 3<<29 {
				t.Fatalf("RSS size: %+v, %v", items, err)
			}
		})
	}
}

func TestIPTorrentsKeyEdges(t *testing.T) {
	t.Parallel()
	cases := []struct {
		raw, user, uid, pass string
		cookie               bool
	}{
		{"  rsskey1\n", " 77 ", "77", "rsskey1", false},
		{"https://iptorrents.com/t.rss?u=77;tp=rsskey1;72;download;q=x", "", "77", "rsskey1", false},
		{"https://iptorrents.com/t.rss?u=77;tp=rsskey1;72;download", "88", "88", "rsskey1", false},
		{"https://iptorrents.com/download.php/1/a.torrent?torrent_pass=rsskey1", "77", "77", "rsskey1", false},
		{"uid=77; pass=cookiepass", "", "77", "", true},
		{"uid=77; pass=cookiepass", "77", "77", "", true},
		{"", "77", "77", "", false},
	}
	for _, c := range cases {
		k := iptKeys(c.raw, c.user)
		if k.uid != c.uid || k.passkey != c.pass || (k.cookie() != "") != c.cookie {
			t.Errorf("%q/%q: got uid %q pass %q cookie %q", c.raw, c.user, k.uid, k.passkey, k.cookie())
		}
	}
}

func TestIPTorrentsCategoryEdges(t *testing.T) {
	t.Parallel()
	cases := []struct {
		q    Query
		want []int
	}{
		{Query{TV: true}, []int{73, 60}},
		{Query{Music: true}, []int{75}},
		{Query{TV: true, Music: true}, []int{75}},
		{Query{}, []int{72, 60}},
		{Query{feed: true, Categories: []int{5070}}, []int{73, 60}},
		{Query{feed: true, Categories: []int{3000, 2040}}, []int{72, 75, 60}},
		{Query{feed: true, Categories: []int{7000}}, []int{72, 73, 75, 60}},
	}
	for _, c := range cases {
		if got := iptCategories(c.q); !slices.Equal(got, c.want) {
			t.Errorf("%+v: got %v want %v", c.q, got, c.want)
		}
	}
}

func TestIPTorrentsFeedEdges(t *testing.T) {
	t.Parallel()
	// Empty feed: an empty answer, not an error.
	items, err := parseIPTorrentsFeed([]byte(`<?xml version="1.0"?><rss version="2.0"><channel><title>IPT</title></channel></rss>`))
	if err != nil || len(items) != 0 {
		t.Fatalf("empty: %v %v", items, err)
	}
	// An item with no description keeps unknown seeders and no size.
	items, err = parseIPTorrentsFeed([]byte(`<?xml version="1.0"?><rss version="2.0"><channel><item><title>A 2020 1080p</title><link>https://iptorrents.com/download.php/9/a.torrent?torrent_pass=k</link><guid>https://iptorrents.com/t/9</guid></item></channel></rss>`))
	if err != nil || len(items) != 1 || items[0].Seeders != nil {
		t.Fatalf("no description: %+v %v", items, err)
	}
	// The sign-in page in place of the feed is a refused sign-in.
	_, err = parseIPTorrentsFeed([]byte(`<html><body><form action="/login.php"><input name="password"></form></body></html>`))
	var se *Error
	if !errors.As(err, &se) || se.Kind != KindLoginRefused {
		t.Fatalf("sign-in: %v", err)
	}
}

// A pasted RSS link from the site works as the passkey field.
func TestIPTorrentsPastedLink(t *testing.T) {
	t.Parallel()
	var seen string
	pasted := "https://iptorrents.com/t.rss?u=77;" + "tp=rsskey1;72;download"
	srv := siteServer(t, "iptorrents.rss", func(r *http.Request, _ string) { seen = r.URL.RawQuery })
	c := New(Config{URL: srv.URL, Format: "site", Site: "iptorrents", Protocol: "torrent", AllowLocal: true,
		APIKey: pasted})
	if items, err := c.Search(context.Background(), Query{Title: "Sample", TV: true}); err != nil || len(items) != 2 {
		t.Fatalf("items %v err %v", items, err)
	}
	if seen != "u=77;tp=rsskey1;73;60;download;q=Sample" {
		t.Fatalf("query %q", seen)
	}
	// A 500 from the site stays "down", not a refused key.
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(502) }))
	t.Cleanup(down.Close)
	c = New(Config{URL: down.URL, Format: "site", Site: "iptorrents", Protocol: "torrent", AllowLocal: true, APIKey: "rsskey1", SiteUser: "77"})
	var se *Error
	if _, err := c.Search(context.Background(), Query{Title: "x"}); !errors.As(err, &se) || se.Kind == KindLoginRefused {
		t.Fatalf("502: %v", err)
	}
}

// A pasted link's path words and search text are not keys: only the passkey
// leaves stored links, and the link comes back whole.
func TestIPTorrentsPastedLinkHidesOnlyPasskey(t *testing.T) {
	t.Parallel()
	pasted := "https://iptorrents.com/t.rss?u=1234567;" + "tp=rsskey1;72;download;q=matrix"
	cfg := Config{Format: "site", Site: "iptorrents", SiteUser: "1234567",
		APIKey: pasted}
	keys := secretValues(cfg)
	if !slices.Equal(keys, []string{"rsskey1"}) {
		t.Fatalf("keys %q", keys)
	}
	link := "https://iptorrents.com/download.php/1234567/The.Matrix.matrix.torrent?torrent_pass=rsskey1"
	stored := cleanLink(link, keys)
	if stored != "https://iptorrents.com/download.php/1234567/The.Matrix.matrix.torrent?torrent_pass="+hidden+"0r" {
		t.Fatalf("stored %s", stored)
	}
	if back, err := revealLink(stored, keys); err != nil || back != link {
		t.Fatalf("back %s %v", back, err)
	}
}
