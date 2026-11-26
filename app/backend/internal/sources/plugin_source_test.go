package sources

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
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

// A plugin Source may carry a download type its own plugin declares, and
// nothing else may.
func TestPluginSourceDownloadType(t *testing.T) {
	PluginDownloadTypes = func(id string) []string {
		if id == "slskd" {
			return []string{"soulseek"}
		}
		return nil
	}
	t.Cleanup(func() { PluginDownloadTypes = nil })
	for _, c := range []struct {
		url, protocol string
		ok            bool
	}{
		{"plugin://slskd", "soulseek", true},
		{"plugin://slskd", "torrent", true},
		{"plugin://slskd", "ed2k", false},
		{"plugin://other", "soulseek", false},
		{"https://indexer.example", "soulseek", false},
	} {
		var src domain.Source
		_, _, err := fill(Input{Name: "Soulseek", Format: "torznab", Protocol: c.protocol, URL: c.url}, &src, true)
		if (err == nil) != c.ok || c.ok && src.Protocol != c.protocol {
			t.Errorf("%s %s: %v %q", c.url, c.protocol, err, src.Protocol)
		}
	}
	src := domain.Source{Name: "Soulseek", Format: "torznab", Protocol: "soulseek", URL: "plugin://slskd", Extra: map[string]string{}}
	if _, _, err := fill(Input{Name: "Soulseek", Protocol: "soulseek"}, &src, false); err != nil {
		t.Errorf("edit kept on its stored address: %v", err)
	}
	for _, raw := range []string{"https://example.org/rss", "plugin://other"} {
		moved := src
		if _, _, err := fill(Input{Name: "Soulseek", URL: raw}, &moved, false); err == nil {
			t.Errorf("address moved to %s kept protocol soulseek", raw)
		}
	}
	if got := NoDownloader("soulseek"); got != "no soulseek Downloader is set up" {
		t.Errorf("no Downloader line %q", got)
	}
}
