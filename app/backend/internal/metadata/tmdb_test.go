package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const fakeCred = "test-read-value"

func fakeTMDB(t *testing.T, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Header.Get("Authorization") != "Bearer "+fakeCred {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"status_message":"Invalid API key"}`))
			return
		}
		if r.URL.Query().Has("api_key") {
			t.Error("token must go in the header, never the URL")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/3/search/movie":
			if q := r.URL.Query(); q.Get("query") == "dune" && q.Get("year") != "2024" && q.Get("year") != "" {
				t.Errorf("query %v", r.URL.Query())
			}
			_, _ = w.Write([]byte(`{"results":[{"id":693134,"title":"Dune: Part Two","release_date":"2024-02-27","overview":"Paul...",
				"original_language":"en","poster_path":"/p.jpg"},{"id":5,"title":"No date","release_date":""}]}`))
		case "/3/movie/693134":
			_, _ = w.Write([]byte(`{"id":693134,"title":"Dune: Part Two","release_date":"2024-02-27","overview":"Paul...","runtime":167,
				"original_language":"en","poster_path":"/p.jpg","imdb_id":"tt15239678",
				"belongs_to_collection":{"id":726871,"name":"Dune Collection"}}`))
		case "/3/movie/5":
			_, _ = w.Write([]byte(`{"id":5,"title":"Standalone","belongs_to_collection":null}`))
		case "/3/find/tt15239678":
			_, _ = w.Write([]byte(`{"movie_results":[{"id":693134}]}`))
		case "/3/movie/429":
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(429)
		default:
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"status_message":"not found"}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newTMDB(srv *httptest.Server, tok string) *TMDB {
	return &TMDB{BaseURL: srv.URL + "/3", Access: func(context.Context) (string, error) { return tok, nil }, Gap: time.Millisecond}
}

func TestTMDBSearchAndDetails(t *testing.T) {
	var hits atomic.Int32
	p := newTMDB(fakeTMDB(t, &hits), fakeCred)
	var _ Provider = p
	ctx := context.Background()
	got, err := p.Search(ctx, "dune", 2024)
	if err != nil || len(got) != 2 {
		t.Fatalf("search %+v %v", got, err)
	}
	if got[0].Name != "Dune: Part Two" || got[0].Year != 2024 || got[0].IDs["tmdb"] != "693134" || got[0].Poster != ImageBase+"/w500/p.jpg" {
		t.Fatalf("result %+v", got[0])
	}
	if got[0].CollectionKnown || got[0].Collection != nil {
		t.Fatalf("search invented collection certainty: %+v", got[0])
	}
	if got[1].Poster != "" || got[1].Year != 0 {
		t.Fatalf("no poster or date: %+v", got[1])
	}
	m, err := p.Movie(ctx, "tmdb:693134")
	if err != nil || m.Runtime != 167 || m.IDs["imdb"] != "tt15239678" || m.OriginalLanguage != "en" {
		t.Fatalf("movie %+v %v", m, err)
	}
	if !m.CollectionKnown || m.Collection == nil || m.Collection.ID != "726871" || m.Collection.Name != "Dune Collection" {
		t.Fatalf("collection %+v", m)
	}
	standalone, err := p.Movie(ctx, "tmdb:5")
	if err != nil || !standalone.CollectionKnown || standalone.Collection != nil {
		t.Fatalf("standalone %+v %v", standalone, err)
	}
	m2, err := p.Movie(ctx, "imdb:tt15239678")
	if err != nil || m2.IDs["tmdb"] != "693134" {
		t.Fatalf("by imdb %+v %v", m2, err)
	}
	if _, err := p.Movie(ctx, "tmdb:1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
	if _, err := p.Movie(ctx, "nonsense"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bad id: %v", err)
	}
}

func TestTMDBCaches(t *testing.T) {
	var hits atomic.Int32
	p := newTMDB(fakeTMDB(t, &hits), fakeCred)
	ctx := context.Background()
	for range 3 {
		if _, err := p.Movie(ctx, "tmdb:693134"); err != nil {
			t.Fatal(err)
		}
	}
	if hits.Load() != 1 {
		t.Fatalf("%d requests, want 1", hits.Load())
	}
}

func TestTMDBBadTokenNotLeaked(t *testing.T) {
	var hits atomic.Int32
	p := newTMDB(fakeTMDB(t, &hits), "wrong-secret-token")
	_, err := p.Movie(context.Background(), "tmdb:693134")
	if !errors.Is(err, ErrNoAccess) || strings.Contains(err.Error(), "wrong-secret-token") {
		t.Fatalf("err %v", err)
	}
	p.Access = func(context.Context) (string, error) { return "", nil }
	if _, err := p.Search(context.Background(), "dune", 0); !errors.Is(err, ErrNoAccess) {
		t.Fatalf("no token: %v", err)
	}
}

func TestTMDBRateLimitAndTimeout(t *testing.T) {
	var hits atomic.Int32
	p := newTMDB(fakeTMDB(t, &hits), fakeCred)
	if _, err := p.Movie(context.Background(), "tmdb:429"); err == nil || !strings.Contains(err.Error(), "busy") {
		t.Fatalf("429: %v", err)
	}
	if hits.Load() != 2 {
		t.Fatalf("429 should be retried once: %d", hits.Load())
	}
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	defer slow.Close()
	q := &TMDB{BaseURL: slow.URL, Access: func(context.Context) (string, error) { return fakeCred, nil }, Timeout: 50 * time.Millisecond}
	start := time.Now()
	if _, err := q.Search(context.Background(), "x", 0); err == nil || time.Since(start) > time.Second {
		t.Fatalf("timeout not applied: %v", err)
	}
}

func TestTMDBGapBetweenRequests(t *testing.T) {
	var hits atomic.Int32
	p := newTMDB(fakeTMDB(t, &hits), fakeCred)
	p.Gap = 40 * time.Millisecond
	start := time.Now()
	for _, q := range []string{"a", "b", "c"} {
		_, _ = p.Search(context.Background(), q, 0)
	}
	if time.Since(start) < 80*time.Millisecond {
		t.Fatal("requests not spaced")
	}
}

func TestTokenFile(t *testing.T) {
	f := t.TempDir() + "/tok"
	if err := writeFile(f, " abc \n"); err != nil {
		t.Fatal(err)
	}
	got, err := TokenFile(f)(context.Background())
	if err != nil || got != "abc" {
		t.Fatalf("%q %v", got, err)
	}
}

func writeFile(p, s string) error { return os.WriteFile(p, []byte(s), 0o600) }

// The members' average and vote count come with every movie and show answer
// and are kept to one decimal; an answer without them gives 0 and 0.
func TestTMDBScoreAndVotes(t *testing.T) {
	var m tmdbMovie
	if err := json.Unmarshal([]byte(`{"id":1,"title":"A","vote_average":7.86,"vote_count":1200}`), &m); err != nil {
		t.Fatal(err)
	}
	if got := m.movie(""); got.Score != 7.9 || got.Votes != 1200 {
		t.Fatalf("movie score %v votes %d", got.Score, got.Votes)
	}
	var s tmdbShow
	if err := json.Unmarshal([]byte(`{"id":2,"name":"B","vote_average":6.04,"vote_count":80}`), &s); err != nil {
		t.Fatal(err)
	}
	if got := s.show(""); got.Score != 6.0 || got.Votes != 80 {
		t.Fatalf("show score %v votes %d", got.Score, got.Votes)
	}
	var bare tmdbMovie
	_ = json.Unmarshal([]byte(`{"id":3,"title":"C"}`), &bare)
	if got := bare.movie(""); got.Score != 0 || got.Votes != 0 {
		t.Fatalf("no score %v votes %d", got.Score, got.Votes)
	}
}
