package sources

import (
	"context"
	"net/http"
	"testing"
)

// idIgnoringSite lists tvdbid, answers every id-only search with its newest
// releases and every title search with the given names.
func idIgnoringSite(t *testing.T, byTitle ...string) *Client {
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch {
		case q.Get("t") == "caps":
			_, _ = w.Write([]byte(`<caps><searching><search available="yes" supportedParams="q"/><tv-search available="yes" supportedParams="q,season,ep,tvdbid"/></searching></caps>`))
		case q.Get("q") != "":
			_, _ = w.Write([]byte(tvRSS(byTitle...)))
		default:
			_, _ = w.Write([]byte(tvRSS(notitleLatest...)))
		}
	})
	return New(Config{URL: srv.URL + "/api", Protocol: "torrent", AllowLocal: true})
}

// TestSearch_IgnoredIDNoTitleHits: a site that ignores the id and finds
// nothing by title gives no results, not its newest releases.
func TestSearch_IgnoredIDNoTitleHits(t *testing.T) {
	q := Query{Title: "Cabin in the Woods", TV: true, Season: 1, Words: "S01", TVDB: 4242}
	items, err := idIgnoringSite(t).Search(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("kept the site's newest releases: %v", titles(items))
	}
}

// TestSearch_IDHitsUnderAlias keeps id results named by another name of the
// show and does not search again.
func TestSearch_IDHitsUnderAlias(t *testing.T) {
	q := Query{Title: "Money Heist", Aliases: []string{"La Casa de Papel"}, TV: true, Season: 1, TVDB: 4242}
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		v := r.URL.Query()
		switch {
		case v.Get("t") == "caps":
			_, _ = w.Write([]byte(`<caps><searching><search available="yes" supportedParams="q"/><tv-search available="yes" supportedParams="q,season,ep,tvdbid"/></searching></caps>`))
		case v.Get("q") != "":
			t.Errorf("searched again by title: %v", v)
		default:
			_, _ = w.Write([]byte(tvRSS("La.Casa.de.Papel.S01.1080p.WEB-GRP")))
		}
	})
	items, err := New(Config{URL: srv.URL + "/api", Protocol: "torrent", AllowLocal: true}).Search(context.Background(), q)
	if err != nil || len(items) != 1 {
		t.Fatalf("items %v err %v", titles(items), err)
	}
}

// TestSearch_IgnoredIDTitleSearchFails: when the site ignored the id and the
// follow-up title search fails, its newest releases are not kept.
func TestSearch_IgnoredIDTitleSearchFails(t *testing.T) {
	q := Query{Title: "Cabin in the Woods", TV: true, Season: 1, Words: "S01", TVDB: 4242}
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		v := r.URL.Query()
		switch {
		case v.Get("t") == "caps":
			_, _ = w.Write([]byte(`<caps><searching><search available="yes" supportedParams="q"/><tv-search available="yes" supportedParams="q,season,ep,tvdbid"/></searching></caps>`))
		case v.Get("q") != "":
			http.Error(w, "slow down", http.StatusTooManyRequests)
		default:
			_, _ = w.Write([]byte(tvRSS(notitleLatest...)))
		}
	})
	items, _ := New(Config{URL: srv.URL + "/api", Protocol: "torrent", AllowLocal: true}).Search(context.Background(), q)
	if len(items) != 0 {
		t.Fatalf("kept the site's newest releases: %v", titles(items))
	}
}
