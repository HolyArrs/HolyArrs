package wikidata

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const artistID = "a74b1b7f-71a5-4011-9441-d0b5e4122711"

func claimQ(p, q string) string {
	return `"` + p + `":[{"rank":"normal","mainsnak":{"snaktype":"value","datavalue":{"value":{"id":"` + q + `"}}}}]`
}

func claimS(p, v string) string {
	return `"` + p + `":[{"rank":"normal","mainsnak":{"snaktype":"value","datavalue":{"value":"` + v + `"}}}]`
}

func claimT(p, t string) string {
	return `"` + p + `":[{"rank":"normal","mainsnak":{"snaktype":"value","datavalue":{"value":{"time":"+` + t + `T00:00:00Z","precision":11}}}}]`
}

// artistWiki is a fake Wikidata and Wikipedia for the artist lookups.
type artistWiki struct {
	statement string // haswbstatement hit ("" none)
	hits      string // wbsearchentities "search" array
	items     map[string]string
	fail      atomic.Bool
	calls     atomic.Int64
	searches  atomic.Int64
}

func (a *artistWiki) serve(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.calls.Add(1)
		if a.fail.Load() {
			w.WriteHeader(502)
			return
		}
		q := r.URL.Query()
		switch {
		case strings.HasPrefix(r.URL.Path, "/rest/page/summary/"):
			w.Write([]byte(`{"title":"Band","extract":"A band.","content_urls":{"desktop":{"page":"https://en.wikipedia.org/wiki/Band"}}}`))
		case q.Get("list") == "search":
			if a.statement == "" {
				w.Write([]byte(`{"query":{"search":[]}}`))
				return
			}
			w.Write([]byte(`{"query":{"search":[{"title":"` + a.statement + `"}]}}`))
		case q.Get("action") == "wbsearchentities":
			a.searches.Add(1)
			w.Write([]byte(`{"search":[` + a.hits + `]}`))
		case q.Get("props") == "labels":
			w.Write([]byte(`{"entities":{"Q900":{"labels":{"en":{"value":"art pop"}}}}}`))
		default:
			var parts []string
			for _, id := range strings.Split(q.Get("ids"), "|") {
				if body, ok := a.items[id]; ok {
					parts = append(parts, `"`+id+`":{"id":"`+id+`","claims":{`+body+`},"sitelinks":{"enwiki":{"title":"Band"}}}`)
				}
			}
			w.Write([]byte(`{"entities":{` + strings.Join(parts, ",") + `}}`))
		}
	}))
}

func countries() map[string]string {
	return map[string]string{"Q30": claimS("P297", "US"), "Q145": claimS("P297", "GB")}
}

func openArtist(t *testing.T, base string, mb *MBArtist) *Service {
	now := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	s, err := Open(filepath.Join(t.TempDir(), "w.sqlite"), Options{APIURL: base + "/api", SPARQLURL: base + "/sparql", WikipediaURL: base + "/rest", Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	s.SetArtistMB(func(context.Context, string) (MBArtist, bool) {
		if mb == nil {
			return MBArtist{}, false
		}
		return *mb, true
	})
	return s
}

func TestArtistByMusicBrainzLinkWithFacts(t *testing.T) {
	w := &artistWiki{statement: "Q999", items: countries()}
	w.items["Q5"] = claimQ("P495", "Q145") + "," + claimT("P571", "1985-01-01") + "," + claimQ("P136", "Q900")
	srv := w.serve(t)
	defer srv.Close()
	mb := &MBArtist{Name: "Band", Country: "US", Wikidata: "Q5"}
	s := openArtist(t, srv.URL, mb)
	e, err := s.Artist(context.Background(), artistID)
	if err != nil {
		t.Fatal(err)
	}
	if e.QID != "Q5" || e.Match != "musicbrainz" || e.Summary == nil || e.Summary.Extract != "A band." {
		t.Fatalf("entity %+v", e)
	}
	if e.Country != "GB" || e.Begin != "1985-01-01" || len(e.Genres) != 1 || e.Genres[0] != "art pop" {
		t.Fatalf("wikidata facts %+v", e)
	}
	// MusicBrainz first: its country stays; begin and genres come from Wikidata.
	f := e.Facts
	if f.Country != "US" || f.Sources["country"] != "musicbrainz" || f.Begin != "1985-01-01" || f.Sources["begin"] != "wikidata" || f.Genres[0] != "art pop" || f.Sources["genres"] != "wikidata" {
		t.Fatalf("facts %+v", f)
	}
}

func TestArtistNameSearchChecks(t *testing.T) {
	hit := func(id, label, desc string) string {
		return `{"id":"` + id + `","description":"` + desc + `","match":{"type":"label","text":"` + label + `"}}`
	}
	w := &artistWiki{items: countries()}
	w.hits = strings.Join([]string{
		hit("Q1", "Band", "American actor"),                  // not about music
		hit("Q2", "Band Two", "American rock band"),          // another name
		hit("Q3", "Band", "British rock band"),               // wrong country
		hit("Q4", "Band", "American rock band"),              // wrong year
		hit("Q6", "Band", "American indie band"),             // another MusicBrainz artist
		hit("Q7", "BAND", "American electronic music group"), // the one
	}, ",")
	w.items["Q3"] = claimQ("P495", "Q145") + "," + claimT("P571", "1990-01-01")
	w.items["Q4"] = claimQ("P495", "Q30") + "," + claimT("P571", "1970-01-01")
	w.items["Q6"] = claimQ("P495", "Q30") + "," + claimT("P571", "1990-01-01") + "," + claimS("P434", "00000000-0000-0000-0000-000000000000")
	w.items["Q7"] = claimQ("P495", "Q30") + "," + claimT("P571", "1990-06-01")
	srv := w.serve(t)
	defer srv.Close()
	s := openArtist(t, srv.URL, &MBArtist{Name: "Band", Country: "US", Begin: "1990"})
	e, err := s.Artist(context.Background(), artistID)
	if err != nil || e.QID != "Q7" || e.Match != "name" {
		t.Fatalf("%+v %v", e, err)
	}

	// Two hits that pass: no item (never guess).
	w2 := &artistWiki{items: countries()}
	w2.hits = hit("Q7", "Band", "American band") + "," + hit("Q8", "Band", "American DJ")
	w2.items["Q7"] = claimQ("P495", "Q30")
	w2.items["Q8"] = claimQ("P27", "Q30")
	srv2 := w2.serve(t)
	defer srv2.Close()
	s2 := openArtist(t, srv2.URL, &MBArtist{Name: "Band", Country: "US"})
	if _, err := s2.Artist(context.Background(), artistID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("two hits answered %v", err)
	}
}

func TestArtistWithoutMusicBrainzAnswerAsksAgain(t *testing.T) {
	w := &artistWiki{items: countries()}
	srv := w.serve(t)
	defer srv.Close()
	s := openArtist(t, srv.URL, nil)
	if _, err := s.Artist(context.Background(), artistID); !errors.Is(err, ErrLater) {
		t.Fatalf("got %v", err)
	}
	n := w.calls.Load()
	if _, err := s.Artist(context.Background(), artistID); !errors.Is(err, ErrLater) || w.calls.Load() == n {
		t.Fatal("a missing MusicBrainz answer was kept as not found")
	}
	if w.searches.Load() != 0 {
		t.Fatal("name search without MusicBrainz's checks")
	}
}

func TestArtistFailureIsNotKeptAsMissing(t *testing.T) {
	w := &artistWiki{items: countries(), statement: "Q7"}
	w.items["Q7"] = claimQ("P495", "Q30")
	srv := w.serve(t)
	defer srv.Close()
	s := openArtist(t, srv.URL, &MBArtist{Name: "Band"})
	w.fail.Store(true)
	if _, err := s.Artist(context.Background(), artistID); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("a 502 answered %v", err)
	}
	w.fail.Store(false)
	e, err := s.Artist(context.Background(), artistID)
	if err != nil || e.QID != "Q7" || e.Match != "statement" {
		t.Fatalf("after the failure: %+v %v", e, err)
	}
}

func TestParseMBArtist(t *testing.T) {
	b := []byte(`{"source_data":{"name":"LISA","country":"","area":{"iso-3166-1-codes":["KR"]},"life-span":{"begin":"1997-03-27"},"genres":[{"name":"k-pop"}],
	"relations":[{"type":"wikidata","url":{"resource":"https://www.wikidata.org/wiki/Q26707663"}},{"type":"streaming","url":{"resource":"https://open.spotify.com/artist/x"}}]}}`)
	a, ok := ParseMBArtist(b)
	if !ok || a.Name != "LISA" || a.Country != "KR" || a.Begin != "1997-03-27" || a.Wikidata != "Q26707663" || len(a.Genres) != 1 {
		t.Fatalf("%+v", a)
	}
	if a, _ := ParseMBArtist([]byte(`{"source_data":{"name":"X","country":"XW"}}`)); a.Country != "" {
		t.Fatal("worldwide kept as a country")
	}
}
