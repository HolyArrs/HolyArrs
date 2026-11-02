package wikidata

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

const entityJSON = `{"entities":{"Q1":{"id":"Q1","claims":{
"P345":[{"rank":"normal","mainsnak":{"snaktype":"value","datavalue":{"value":"nm0000001"}}}],
"P4835":[{"rank":"normal","mainsnak":{"snaktype":"value","datavalue":{"value":"81189"}}}],
"P569":[{"rank":"normal","mainsnak":{"snaktype":"value","datavalue":{"value":{"time":"+1956-07-09T00:00:00Z","precision":11}}}}],
"P19":[{"rank":"normal","mainsnak":{"snaktype":"value","datavalue":{"value":{"id":"Q100"}}}}],
"P166":[
 {"rank":"normal","mainsnak":{"snaktype":"value","datavalue":{"value":{"id":"Q10"}}},"qualifiers":{"P585":[{"snaktype":"value","datavalue":{"value":{"time":"+1995-03-27T00:00:00Z","precision":11}}}],"P1686":[{"snaktype":"value","datavalue":{"value":{"id":"Q20"}}}]}},
 {"rank":"normal","mainsnak":{"snaktype":"value","datavalue":{"value":{"id":"Q11"}}},"qualifiers":{"P585":[{"snaktype":"value","datavalue":{"value":{"time":"+1994-00-00T00:00:00Z","precision":9}}}]}},
 {"rank":"deprecated","mainsnak":{"snaktype":"value","datavalue":{"value":{"id":"Q12"}}}}],
"P1411":[{"rank":"normal","mainsnak":{"snaktype":"value","datavalue":{"value":{"id":"Q13"}}}}]},
"sitelinks":{"enwiki":{"title":"Some Person"}}}}}`

const labelsJSON = `{"entities":{"Q10":{"labels":{"en":{"value":"Academy Award for Best Actor"}}},"Q11":{"labels":{"en":{"value":"Golden Globe Award for Best Actor"}}},"Q13":{"labels":{"en":{"value":"Primetime Emmy Award for Outstanding Lead Actor"}}},"Q20":{"labels":{"en":{"value":"Forrest Gump"}}},"Q100":{"labels":{"en":{"value":"Concord"}}}}}`

const sparqlMovies = `{"results":{"bindings":[{"tmdb":{"value":"603"},"wins":{"value":"4"}},{"tmdb":{"value":"13"},"wins":{"value":"6"}},{"tmdb":{"value":"x"},"wins":{"value":"1"}}]}}`
const sparqlShows = `{"results":{"bindings":[{"tmdb":{"value":"1396"},"wins":{"value":"2"}}]}}`

type upstream struct {
	calls atomic.Int64
	mu    sync.Mutex
	uas   []string
}

func fake(t *testing.T, u *upstream) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u.calls.Add(1)
		u.mu.Lock()
		u.uas = append(u.uas, r.UserAgent())
		u.mu.Unlock()
		q := r.URL.Query()
		switch {
		case r.URL.Path == "/sparql" && strings.Contains(q.Get("query"), "P4947"):
			w.Write([]byte(sparqlMovies))
		case r.URL.Path == "/sparql":
			w.Write([]byte(sparqlShows))
		case strings.HasPrefix(r.URL.Path, "/rest/page/summary/"):
			w.Write([]byte(`{"title":"Some Person","extract":"An actor.","content_urls":{"desktop":{"page":"https://en.wikipedia.org/wiki/Some_Person"}}}`))
		case q.Get("list") == "search":
			if q.Get("srsearch") == "haswbstatement:P4985=31" {
				w.Write([]byte(`{"query":{"search":[{"title":"Q1"}]}}`))
				return
			}
			w.Write([]byte(`{"query":{"search":[]}}`))
		case q.Get("props") == "labels":
			if strings.Count(q.Get("ids"), "|") != 4 {
				t.Errorf("labels not batched: %s", q.Get("ids"))
			}
			w.Write([]byte(labelsJSON))
		default:
			w.Write([]byte(entityJSON))
		}
	}))
}

func open(t *testing.T, base string, now *time.Time) *Service {
	s, err := Open(filepath.Join(t.TempDir(), "w.sqlite"), Options{APIURL: base + "/api", SPARQLURL: base + "/sparql", WikipediaURL: base + "/rest", Now: func() time.Time { return *now }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestPersonAwardsFactsAndSummary(t *testing.T) {
	u := &upstream{}
	srv := fake(t, u)
	defer srv.Close()
	now := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	s := open(t, srv.URL, &now)
	e, err := s.Person(context.Background(), 31)
	if err != nil {
		t.Fatal(err)
	}
	if e.QID != "Q1" || e.IDs.IMDb != "nm0000001" || e.IDs.TVDB != "81189" || e.Born != "1956-07-09" || e.BirthPlace != "Concord" || e.Wikipedia != "Some Person" {
		t.Fatalf("facts %+v", e)
	}
	if len(e.Awards) != 3 || !e.Awards[0].Won || e.Awards[0].Family != "oscar" || e.Awards[0].Year != 1995 || e.Awards[0].ForWork != "Forrest Gump" || e.Awards[2].Won || e.Awards[2].Family != "emmy" {
		t.Fatalf("awards %+v", e.Awards)
	}
	if len(e.Tallies) != 3 || e.Tallies[0] != (Tally{Family: "oscar", Wins: 1}) || e.Tallies[1].Family != "emmy" || e.Tallies[1].Nominations != 1 {
		t.Fatalf("tallies %+v", e.Tallies)
	}
	if e.Summary == nil || e.Summary.Extract != "An actor." || e.Summary.License != "CC BY-SA 4.0" || !strings.HasPrefix(e.Summary.URL, "https://en.wikipedia.org/") || e.Summary.Attribution == "" {
		t.Fatalf("summary %+v", e.Summary)
	}
	for _, ua := range u.uas {
		if !strings.HasPrefix(ua, "HolyArrsMetadata/") || !strings.Contains(ua, "github.com/CoderLuii/HolyArrs") {
			t.Fatalf("user agent %q", ua)
		}
	}
	// Kept 14 days: no new calls; after that, fetched again.
	n := u.calls.Load()
	now = now.Add(13 * 24 * time.Hour)
	if _, err := s.Person(context.Background(), 31); err != nil || u.calls.Load() != n {
		t.Fatalf("cache %v %d", err, u.calls.Load()-n)
	}
	now = now.Add(2 * 24 * time.Hour)
	if _, err := s.Person(context.Background(), 31); err != nil || u.calls.Load() == n {
		t.Fatal("not refreshed")
	}
}

func TestNotFoundIsCached(t *testing.T) {
	u := &upstream{}
	srv := fake(t, u)
	defer srv.Close()
	now := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	s := open(t, srv.URL, &now)
	if _, err := s.Title(context.Background(), "movie", 9); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := s.Title(context.Background(), "movie", 9); !errors.Is(err, ErrNotFound) || u.calls.Load() != 1 {
		t.Fatalf("miss not cached: %d", u.calls.Load())
	}
	if _, err := s.Artist(context.Background(), "not-an-mbid"); !errors.Is(err, ErrNotFound) {
		t.Fatal("bad mbid")
	}
}

func TestWinnersRefresh(t *testing.T) {
	u := &upstream{}
	srv := fake(t, u)
	defer srv.Close()
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "w.sqlite"), Options{SPARQLURL: srv.URL + "/sparql"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Winners("movie"); ok {
		t.Fatal("list before refresh")
	}
	if err := s.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	m, ok := s.Winners("movie")
	if !ok || len(m) != 2 || m[0].TMDB != 13 || m[0].Wins != 6 {
		t.Fatalf("movies %+v", m)
	}
	if tv, _ := s.Winners("tv"); len(tv) != 1 || tv[0].TMDB != 1396 {
		t.Fatalf("tv %+v", tv)
	}
	s.Close()
	// Kept across restarts.
	s2, err := Open(filepath.Join(dir, "w.sqlite"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if m, ok := s2.Winners("movie"); !ok || len(m) != 2 || s2.Stats().WinnersAt == "" {
		t.Fatal("not persisted")
	}
}

func TestFamily(t *testing.T) {
	for name, want := range map[string]string{"Academy Award for Best Picture": "oscar", "Primetime Emmy Award for Outstanding Drama Series": "emmy", "Golden Globe Award for Best Actor": "golden_globe", "BAFTA Award for Best Film": "bafta", "Grammy Award for Album of the Year": "grammy", "Saturn Award": ""} {
		if Family(name) != want {
			t.Errorf("%s -> %s", name, Family(name))
		}
	}
}

func TestWikipediaPage(t *testing.T) {
	for raw, want := range map[string]bool{"https://en.wikipedia.org/wiki/X": true, "https://wikipedia.org/": true, "javascript:alert(1)": false, "http://en.wikipedia.org/wiki/X": false, "https://evil.test/wikipedia.org": false, "https://en.wikipedia.org.evil.test/": false, "https://u@en.wikipedia.org/": false} {
		if WikipediaPage(raw) != want {
			t.Errorf("%s -> %v", raw, !want)
		}
	}
}

func TestSummaryReplacesUnsafePageLink(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"title":"X","extract":"x","content_urls":{"desktop":{"page":"javascript:alert(1)"}}}`))
	}))
	defer srv.Close()
	now := time.Now()
	s := open(t, srv.URL, &now)
	sum, err := s.summary(context.Background(), "Some Person")
	if err != nil || sum.URL != "https://en.wikipedia.org/wiki/Some_Person" || sum.LicenseURL != LicenseURL {
		t.Fatalf("%+v %v", sum, err)
	}
}

func TestCommonsThumb(t *testing.T) {
	// Path from Commons' own URL for this file (MD5 of "Albert_Einstein_Head.jpg" starts "d3").
	got := CommonsThumb("Albert Einstein Head.jpg", 330)
	want := "https://upload.wikimedia.org/wikipedia/commons/thumb/d/d3/Albert_Einstein_Head.jpg/330px-Albert_Einstein_Head.jpg"
	if got != want {
		t.Fatalf("CommonsThumb = %q, want %q", got, want)
	}
	if CommonsThumb("", 300) != "" {
		t.Fatal("no file should give no URL")
	}
}

func TestAPIErrorIsNotNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"error":{"code":"maxlag","info":"lagged"}}`))
	}))
	defer srv.Close()
	s, err := Open(filepath.Join(t.TempDir(), "w.sqlite"), Options{APIURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.Person(context.Background(), 1245); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("a busy Wikidata must be a failure, not not-found: %v", err)
	}
}

func TestTVmazePhoto(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch r.URL.Query().Get("q") {
		case "Joanna van Kampen":
			_, _ = w.Write([]byte(`[{"person":{"name":"Joanna van Kampen","image":{"medium":"https://static.tvmaze.com/uploads/images/medium_portrait/293/732996.jpg"}}},{"person":{"name":"Joanna van Kamp","image":null}}]`))
		case "John Smith":
			_, _ = w.Write([]byte(`[{"person":{"name":"John Smith","image":{"medium":"https://static.tvmaze.com/a.jpg"}}},{"person":{"name":"john smith","image":{"medium":"https://static.tvmaze.com/b.jpg"}}}]`))
		default:
			_, _ = w.Write([]byte(`[]`))
		}
	}))
	defer srv.Close()
	s, err := Open(filepath.Join(t.TempDir(), "w.sqlite"), Options{TVmazeURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if got := s.TVmazePhoto(ctx, 2908794, "Joanna van Kampen"); got != "https://static.tvmaze.com/uploads/images/medium_portrait/293/732996.jpg" {
		t.Fatalf("exact single match: %q", got)
	}
	if got := s.TVmazePhoto(ctx, 2908794, "Joanna van Kampen"); got == "" || calls != 1 {
		t.Fatalf("second ask should come from the cache (calls %d)", calls)
	}
	if got := s.TVmazePhoto(ctx, 7, "John Smith"); got != "" {
		t.Fatalf("two people with the name must not guess: %q", got)
	}
}

func TestArtistFreshAndStored(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	s, err := Open(filepath.Join(t.TempDir(), "w.sqlite"), Options{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.fresh("artist") != 30*24*time.Hour || s.fresh("movie") != 14*24*time.Hour {
		t.Fatal(s.fresh("artist"), s.fresh("movie"))
	}
	ctx := context.Background()
	if s.Stored(ctx, "movie", 1) {
		t.Fatal("nothing stored yet")
	}
	day := int64(24 * 3600)
	s.db.Exec(`INSERT INTO answers VALUES ('movie:1', '{}', ?), ('movie:2', '{}', ?), ('tv:3', NULL, ?), ('tv:4', NULL, ?)`,
		now.Unix()-13*day, now.Unix()-15*day, now.Unix()-3600, now.Unix()-2*day)
	for id, want := range map[int64]bool{1: true, 2: false} {
		if s.Stored(ctx, "movie", id) != want {
			t.Errorf("movie %d: want %v", id, want)
		}
	}
	if !s.Stored(ctx, "tv", 3) || s.Stored(ctx, "tv", 4) {
		t.Error("misses: kept a day")
	}
}
