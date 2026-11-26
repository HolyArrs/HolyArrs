package sources

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

func TestYTSSearchAndFeed(t *testing.T) {
	t.Parallel()
	var requests []url.Values
	srv := siteServer(t, "yts.json", func(r *http.Request, _ string) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v2/list_movies.json" {
			t.Errorf("request %s %s", r.Method, r.URL)
		}
		requests = append(requests, r.URL.Query())
	})
	c := siteClient(srv.URL, "yts", "")
	items, err := c.Search(context.Background(), Query{Title: "Salt Choir", Year: 2020, IMDb: "12345"})
	if err != nil || len(items) != 2 {
		t.Fatalf("items %+v: %v", items, err)
	}
	it := items[0]
	if it.Title != "Salt Choir (2020) 1080p BluRay 5.1 x264 -YTS" || it.Size != 1500000000 || it.IMDb != "12345" ||
		it.InfoURL != "https://yts.pw/movies/salt-choir-2020" || *it.Seeders != 42 || *it.Peers != 50 || it.Published.Unix() != 1700000000 ||
		it.InfoHash != strings.Repeat("a", 40) || it.GUID != it.InfoHash || it.Protocol != "torrent" || len(it.Languages) != 1 || it.Languages[0] != "en" {
		t.Fatalf("first result %+v", it)
	}
	u, err := url.Parse(it.Link)
	if err != nil || u.Scheme != "magnet" || u.Query().Get("xt") != "urn:btih:"+it.InfoHash || u.Query().Get("dn") != it.Title {
		t.Fatalf("magnet %s: %v", it.Link, err)
	}
	parsed := releasename.Parse(items[1].Title, releasename.Hints{Size: items[1].Size})
	if parsed.Resolution.Value != 2160 || parsed.Source.Value != "web_rip" || parsed.Codec.Value != "h265" {
		t.Fatalf("release facts %+v", parsed)
	}
	if _, err := c.Search(context.Background(), Query{Title: "Salt Choir", Year: 2020}); err != nil {
		t.Fatal(err)
	}
	if feed, err := c.Feed(context.Background(), nil); err != nil || len(feed) != 2 {
		t.Fatalf("feed %+v: %v", feed, err)
	}
	if n, err := c.Test(context.Background()); err != nil || n != 2 {
		t.Fatalf("test %d: %v", n, err)
	}
	if len(requests) != 4 || requests[0].Get("query_term") != "tt0012345" || requests[1].Get("query_term") != "Salt Choir" || requests[2].Has("query_term") || requests[3].Has("query_term") {
		t.Fatalf("queries %+v", requests)
	}
	for _, q := range requests {
		if q.Get("limit") != "50" || q.Get("sort_by") != "date_added" || q.Get("order_by") != "desc" {
			t.Errorf("query %+v", q)
		}
	}
	// Movie-only sources must not return their newest list for an empty search,
	// a show, or a music query.
	if _, err := c.Search(context.Background(), Query{}); !errors.Is(err, ErrNothingToSearch) {
		t.Fatalf("empty search: %v", err)
	}
	for _, q := range []Query{{TV: true, Title: "A Show"}, {Music: true, Artist: "An Artist"}} {
		if got, err := c.Search(context.Background(), q); err != nil || len(got) != 0 {
			t.Fatalf("unsupported query: %+v %v", got, err)
		}
	}
	if len(requests) != 4 {
		t.Fatalf("unexpected request: %+v", requests)
	}
}

func TestYTSRefusesUnrelatedIDResults(t *testing.T) {
	t.Parallel()
	srv := siteServer(t, "yts.json", func(*http.Request, string) {})
	items, err := siteClient(srv.URL, "yts", "").Search(context.Background(), Query{IMDb: "tt9999999"})
	if err != nil || len(items) != 0 {
		t.Fatalf("unrelated ID results %+v: %v", items, err)
	}
}

func TestYTSResponseErrorsAndEmptyResults(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{`<html>bot check</html>`, `{}`, `null`, `{"status":"error","status_message":"bad request"}`, `{"status":"ok"}`, `{"status":"ok","data":[]}`} {
		if _, err := parseYTS([]byte(raw)); err == nil {
			t.Errorf("accepted invalid response %s", raw)
		}
	}
	// The API may report matches without listing movies or torrents.
	for _, raw := range []string{`{"status":"ok","data":{"movie_count":0}}`, `{"status":"ok","data":{"movie_count":1,"movies":[]}}`} {
		if items, err := parseYTS([]byte(raw)); err != nil || len(items) != 0 {
			t.Errorf("empty response: %+v %v", items, err)
		}
	}
}

func TestYTSIMDbAndCapabilities(t *testing.T) {
	t.Parallel()
	for input, want := range map[string]string{"12345": "tt0012345", "tt0012345": "tt0012345", "TT12345678": "tt12345678", "": "", "tt": "", "abc": "", "0": "", "-123": ""} {
		if got := ytsIMDb(input); got != want {
			t.Errorf("IMDb %q: %q, want %q", input, got, want)
		}
	}
	caps, err := siteClient("https://movies-api.accel.li", "yts", "").Caps(context.Background())
	if err != nil || !caps.Search || !caps.MovieSearch || !caps.Supports("movie", "imdbid") || caps.TVSearch || caps.MusicSearch {
		t.Fatalf("caps %+v: %v", caps, err)
	}
}
