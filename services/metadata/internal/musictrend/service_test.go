package musictrend

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/model"
	_ "modernc.org/sqlite"
)

const (
	artistA = "11111111-1111-4111-8111-111111111111"
	artistB = "22222222-2222-4222-8222-222222222222"
	groupA  = "33333333-3333-4333-8333-333333333333"
	release = "44444444-4444-4444-8444-444444444444"
	singer  = "55555555-5555-4555-8555-555555555555"
	grand   = "66666666-6666-4666-8666-666666666666"
)

type fakeSearch struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (f *fakeSearch) Search(_ context.Context, kind, name, artist string) ([]Candidate, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	switch kind + "|" + name {
	case "artist|Chanteuse":
		return []Candidate{{MBID: "77777777-7777-4777-8777-777777777777", Name: "Chanteuse Tribute"}, {MBID: singer, Name: "CHANTEUSE"}}, nil
	case "artist|Beyoncé":
		return []Candidate{{MBID: artistB, Name: "Beyonce"}}, nil
	case "album|Grand Album (Deluxe)":
		return []Candidate{{MBID: grand, Name: "Grand Album", Artists: []string{"Chanteuse"}, ArtistMBID: singer}}, nil
	case "album|Big One - Single":
		return []Candidate{{MBID: groupA, Name: "Big One", Artists: []string{"Someone Else"}}}, nil
	}
	return nil, nil
}

type sources struct {
	fail  atomic.Bool
	hits  sync.Map
	agent atomic.Value
}

func (s *sources) count(path string) int64 {
	v, _ := s.hits.LoadOrStore(path, new(atomic.Int64))
	return v.(*atomic.Int64).Load()
}

const art = "https://is1-ssl.mzstatic.com/image/thumb/Music/v4/a.jpg/{w}x{h}bb.jpg"

func items(list ...[2]string) string {
	var b strings.Builder
	for i, it := range list {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`{"attributes":{"name":"` + it[0] + `","artistName":"` + it[1] + `","genreNames":["Pop","Music"],"artwork":{"url":"` + art + `"}}}`)
	}
	return b.String()
}

func charts(albums, songs string) string {
	return `{"results":{"albums":[{"chart":"most-played","data":[` + albums + `]}],"songs":[{"chart":"most-played","data":[` + songs + `]}]}}`
}

// serve fakes the Apple Music API charts for the US and France; other
// storefronts answer 404.
func serve(t *testing.T, s *sources) *httptest.Server {
	t.Helper()
	pages := map[string]string{
		"/v1/catalog/us/charts": charts(items([2]string{"Grand Album (Deluxe)", "Chanteuse"}), items([2]string{"One", "Beyoncé"}, [2]string{"Two", "Chanteuse"}, [2]string{"Three", "Beyoncé"})),
		"/v1/catalog/fr/charts": charts(items([2]string{"Big One - Single", "Chanteuse"}), items([2]string{"Un", "Chanteuse"})),
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v, _ := s.hits.LoadOrStore(r.URL.Path, new(atomic.Int64))
		v.(*atomic.Int64).Add(1)
		s.agent.Store(r.Header.Get("User-Agent"))
		if s.fail.Load() {
			w.WriteHeader(503)
			return
		}
		b, ok := pages[r.URL.Path]
		if !ok {
			w.WriteHeader(404)
			return
		}
		_, _ = w.Write([]byte(b))
	}))
}

type body struct {
	Kind    string   `json:"kind"`
	Region  string   `json:"region"`
	Source  string   `json:"source"`
	Regions []string `json:"regions"`
	Items   []struct {
		Rank       int    `json:"rank"`
		MBID       string `json:"mbid"`
		Name       string `json:"name"`
		Title      string `json:"title"`
		Artist     string `json:"artist_name"`
		ArtistMBID string `json:"artist_mbid"`
		CAARelease string `json:"caa_release_mbid"`
		Cover      string `json:"cover"`
	} `json:"items"`
}

func decode(t *testing.T, s *Service, kind, region string) body {
	t.Helper()
	b, etag, ok := s.List(kind, region)
	if !ok || etag == "" {
		t.Fatalf("%s/%s missing", kind, region)
	}
	var out body
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestServiceBuildsAppleChartsAndKeepsLastGood(t *testing.T) {
	src := &sources{}
	srv := serve(t, src)
	defer srv.Close()
	search := &fakeSearch{}
	path := filepath.Join(t.TempDir(), "music.sqlite")
	var reported []string
	opts := Options{ListenBrainzURL: srv.URL + "/lb", Apple: appleVia(srv), UserAgent: "test-agent", Search: search, Pause: -1,
		Report: func(source, code string, n int) { reported = append(reported, source+":"+code) }}
	s, err := Open(path, opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if src.agent.Load() != "test-agent" {
		t.Fatalf("user agent %v", src.agent.Load())
	}
	// Artists rank by appearances in songs and albums, ties by first place.
	g := decode(t, s, "artist", "global")
	if g.Source != SrcApple || len(g.Items) != 2 || g.Items[0].MBID != artistB || g.Items[0].Name != "Beyoncé" || g.Items[1].MBID != singer || strings.Join(g.Regions, ",") != "global,FR,US" {
		t.Fatalf("global artists %+v", g)
	}
	ga := decode(t, s, "album", "global")
	if len(ga.Items) != 1 || ga.Items[0].MBID != grand || ga.Items[0].ArtistMBID != singer || ga.Items[0].Cover != strings.Replace(art, "{w}x{h}bb", "500x500bb", 1) {
		t.Fatalf("global albums %+v", ga)
	}
	if fr := decode(t, s, "artist", "FR"); fr.Source != SrcApple || len(fr.Items) != 1 || fr.Items[0].MBID != singer {
		t.Fatalf("FR artists %+v", fr)
	}
	if _, _, ok := s.List("album", "FR"); ok {
		t.Fatal("FR album by the wrong artist matched")
	}
	if _, _, ok := s.List("artist", "DE"); ok {
		t.Fatal("storefront without a chart served")
	}
	// Read once a day: a second refresh reads nothing and searches nothing.
	calls, builds, reads := search.calls, s.Builds, src.count("/v1/catalog/us/charts")
	if err := s.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s.Builds != builds || src.count("/v1/catalog/us/charts") != reads || search.calls != calls {
		t.Fatal("charts read again within the day")
	}
	// Failures keep the last good lists and are reported.
	before, _, _ := s.List("artist", "global")
	src.fail.Store(true)
	s.appleAt = time.Time{}
	if err := s.Refresh(context.Background()); err == nil {
		t.Fatal("failure not returned")
	}
	if after, _, _ := s.List("artist", "global"); string(after) != string(before) {
		t.Fatal("last good list lost")
	}
	if !strings.Contains(strings.Join(reported, ","), "apple:fetch_failed") {
		t.Fatalf("reported %v", reported)
	}
	s.Close()
	// A restart serves the stored lists before any fetch.
	s2, err := Open(path, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if b, _, ok := s2.List("album", "global"); !ok || !strings.Contains(string(b), grand) {
		t.Fatal("stored lists not loaded")
	}
}

func TestRankArtists(t *testing.T) {
	got := RankArtists([]Entry{{Artist: "A"}, {Artist: "B"}, {Artist: "b"}}, []Entry{{Artist: "C"}, {Artist: "A"}})
	if len(got) != 3 || got[0].Name != "A" || got[1].Name != "B" || got[2].Name != "C" {
		t.Fatalf("%+v", got)
	}
}

func TestFailedSearchNotRemembered(t *testing.T) {
	db, err := openDB(filepath.Join(t.TempDir(), "m.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	f := &fakeSearch{err: errors.New("down")}
	m := &Matcher{DB: db, Search: f}
	if _, ok := m.Match(context.Background(), "artist", "Chanteuse", ""); ok {
		t.Fatal("matched while down")
	}
	f.err = nil
	if c, ok := m.Match(context.Background(), "artist", "Chanteuse", ""); !ok || c.MBID != singer {
		t.Fatal("failure was remembered")
	}
	m.Match(context.Background(), "artist", "Nobody", "")
	m.Match(context.Background(), "artist", "Nobody", "")
	if f.calls != 3 {
		t.Fatalf("no-match searched again within a week: %d", f.calls)
	}
}

func TestBaseTitle(t *testing.T) {
	for in, want := range map[string]string{"Grand Album (Deluxe)": "Grand Album", "X [Remastered] (Live)": "X", "Big One - Single": "Big One", "(What's the Story)": "(What's the Story)"} {
		if got := baseTitle(in); got != want {
			t.Errorf("baseTitle(%q) = %q", in, got)
		}
	}
}

type fetchFunc func(context.Context, model.Query) (model.Result, error)

func (f fetchFunc) Fetch(c context.Context, q model.Query) (model.Result, error) { return f(c, q) }

func TestMBSearchQueries(t *testing.T) {
	var got []model.Query
	m := MBSearch{Fetcher: fetchFunc(func(_ context.Context, q model.Query) (model.Result, error) {
		got = append(got, q)
		return model.Result{Data: json.RawMessage(`{"items":[{"source_id":"` + strings.ToUpper(grand) + `","title":"Grand Album","source_data":{"artist-credit":[{"name":"Chanteuse","artist":{"id":"` + singer + `","name":"Chanteuse"}}]}},{"source_id":"bad","title":"x"}]}`)}, nil
	})}
	cs, err := m.Search(context.Background(), "album", `Grand "Album" (Deluxe)`, "Chanteuse")
	if err != nil || len(cs) != 1 || cs[0].MBID != grand || cs[0].ArtistMBID != singer {
		t.Fatalf("%v %+v", err, cs)
	}
	if got[0].Provider != "musicbrainz" || got[0].Kind != "release-group" || got[0].Operation != "search" || got[0].Q != `releasegroup:"Grand \"Album\"" AND artist:"Chanteuse"` {
		t.Fatalf("query %+v", got[0])
	}
	if _, err := m.Search(context.Background(), "artist", "Chanteuse", ""); err != nil || got[1].Kind != "artist" || got[1].Q != `artist:"Chanteuse"` {
		t.Fatalf("artist query %+v", got[1])
	}
}

// appleVia reads the Apple Music API at a fake.
func appleVia(srv *httptest.Server) AppleAPI {
	return func(ctx context.Context, path string, q url.Values, out any) (bool, error) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+path+"?"+q.Encode(), nil)
		req.Header.Set("User-Agent", "test-agent")
		resp, err := srv.Client().Do(req)
		if err != nil {
			return false, err
		}
		defer resp.Body.Close()
		switch {
		case resp.StatusCode == 404:
			return false, nil
		case resp.StatusCode != 200:
			return false, errors.New("apple: answered " + resp.Status)
		}
		return true, json.NewDecoder(resp.Body).Decode(out)
	}
}

// A name held back (no match within a week) is not searched and not
// counted as unmatched again; only real searches count.
func TestUnmatchedCountsSearchesOnly(t *testing.T) {
	db, err := openDB(filepath.Join(t.TempDir(), "m.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	f := &fakeSearch{}
	m := &Matcher{DB: db, Search: f}
	for range 5 {
		m.Match(context.Background(), "artist", "Nobody", "")
	}
	if f.calls != 1 || m.Unmatched != 1 || m.Held != 4 {
		t.Fatalf("calls %d unmatched %d held %d", f.calls, m.Unmatched, m.Held)
	}
}
