package sources

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
)

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestPluginSourceAddress(t *testing.T) {
	for _, c := range []struct {
		raw string
		ok  bool
	}{
		{"plugin://my-indexer", true},
		{"plugin://my-indexer/api", false},
		{"plugin://my-indexer?apikey=x", false},
		{"plugin://", false},
		{"plugin://a_b", false},
		{"https://indexer.example", true},
	} {
		if _, err := checkURL(c.raw); (err == nil) != c.ok {
			t.Errorf("%q: %v", c.raw, err)
		}
	}
}

func TestPluginSourceSearchGoesThroughPlugin(t *testing.T) {
	old := PluginTransport
	t.Cleanup(func() { PluginTransport = old })
	var asked []string
	PluginTransport = func(id string) http.RoundTripper {
		return rtFunc(func(r *http.Request) (*http.Response, error) {
			asked = append(asked, id+" "+r.URL.Path)
			body := `<?xml version="1.0"?><rss xmlns:torznab="http://torznab.com/schemas/2015/feed"><channel><item><title>Dune 2021 1080p</title><guid>1</guid><link>plugin://idx/get?id=1</link><size>100</size><torznab:attr name="seeders" value="5"/></item></channel></rss>`
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/rss+xml"}}, Body: io.NopCloser(bytes.NewBufferString(body)), Request: r}, nil
		})
	}
	c := New(Config{URL: "plugin://idx", Protocol: "torrent", Format: "torznab"})
	items, err := c.searchWith(context.Background(), Query{}, Caps{}, map[string][]string{"t": {"search"}, "q": {"Dune"}})
	if err != nil || len(items) != 1 || items[0].Title != "Dune 2021 1080p" {
		t.Fatalf("items %+v err %v", items, err)
	}
	if len(asked) != 1 || asked[0] != "idx /api" {
		t.Errorf("asked %v", asked)
	}
	PluginTransport = nil
	if _, err := New(Config{URL: "plugin://idx", Format: "torznab"}).searchWith(context.Background(), Query{}, Caps{}, map[string][]string{"t": {"search"}}); err == nil {
		t.Error("a plugin source worked with plugins off")
	}
}
