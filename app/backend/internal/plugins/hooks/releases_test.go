package hooks

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestReleaseTransportTable(t *testing.T) {
	b := newTestBroker(map[string][]string{"idx": {"sources.releases"}})
	stub(t, b, "idx", Releases, func(j Job) Answer {
		var in FeedCall
		_ = json.Unmarshal(j.Payload, &in)
		out := FeedAnswer{Status: 200, Type: "application/rss+xml", Body: []byte(`<rss><channel><item><title>Dune</title><link>plugin://idx/get?id=1</link></item></channel></rss>`)}
		if in.Path == "/get" {
			out = FeedAnswer{Status: 200, Type: "application/x-bittorrent", Body: []byte("d8:announce")}
		}
		raw, _ := json.Marshal(out)
		return Answer{Data: raw}
	})
	rt := &ReleaseTransport{Broker: b, Plugin: "idx", Wait: time.Second}
	for _, c := range []struct {
		name, url string
		ok        bool
		body      string
	}{
		{"search", "plugin://idx/api?t=search&q=dune", true, "<rss>"},
		{"grab through the plugin", "plugin://idx/get?id=1", true, "d8:announce"},
		{"internal address", "http://127.0.0.1:8080/x", false, ""},
		{"cloud metadata", "http://169.254.169.254/latest", false, ""},
		{"another plugin", "plugin://other/get?id=1", false, ""},
		{"https outside", "https://example.com/x.torrent", false, ""},
	} {
		req, _ := http.NewRequest(http.MethodGet, c.url, nil)
		resp, err := rt.RoundTrip(req)
		if (err == nil) != c.ok {
			t.Errorf("%s: err %v", c.name, err)
			continue
		}
		if !c.ok {
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != 200 || len(body) < len(c.body) || string(body[:len(c.body)]) != c.body {
			t.Errorf("%s: %d %s", c.name, resp.StatusCode, body)
		}
	}
}
