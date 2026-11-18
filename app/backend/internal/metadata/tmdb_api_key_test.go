package metadata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// TMDB hands out two credentials: a v4 read access token (a JWT) and a v3
// API key (32 hex characters, the "API Key" on TMDB's own settings page,
// which is the one most people copy). The API key has to work, or a key
// pasted from TMDB's site fails as if the metadata server had refused the
// install. It goes in the query, never in an Authorization header.
func TestTMDBAcceptsV3APIKey(t *testing.T) {
	const apiKey = "e9a6f0b6d1c24a3f8b7d5e0c9a1f2b3d"
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h := r.Header.Get("Authorization"); h != "" {
			t.Errorf("a v3 API key must not go in a header, got %q", h)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if got := r.URL.Query().Get("api_key"); got != apiKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/3/search/movie" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if q := r.URL.Query(); q.Get("query") != "resident evil" || q.Get("year") != "2002" {
			t.Errorf("query %v", r.URL.Query())
		}
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"id":1576,"title":"Resident Evil","release_date":"2002-03-15","original_language":"en"}]}`))
	}))
	defer srv.Close()

	tm := &TMDB{
		BaseURL: srv.URL + "/3",
		Access:  func(context.Context) (string, error) { return apiKey, nil },
		HTTP:    srv.Client(),
	}
	got, err := tm.Search(context.Background(), "resident evil", 2002)
	if err != nil {
		t.Fatalf("search with a v3 API key: %v", err)
	}
	if len(got) != 1 || got[0].Name != "Resident Evil" || got[0].IDs["tmdb"] != "1576" {
		t.Fatalf("got %+v", got)
	}
	if hits.Load() != 1 {
		t.Fatalf("the server answered %d times, want 1", hits.Load())
	}
}

// A v4 read access token still rides the header, and never the query.
func TestTMDBKeepsV4TokenInTheHeader(t *testing.T) {
	const token = "eyJhbGciOiJIUzI1NiJ9.fake-read-token.signature"
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("api_key") {
			t.Errorf("a v4 token must not go in the URL: %v", r.URL.Query())
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+token {
			t.Errorf("Authorization = %q", got)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"id":1576,"title":"Resident Evil","release_date":"2002-03-15"}]}`))
	}))
	defer srv.Close()

	tm := &TMDB{
		BaseURL: srv.URL + "/3",
		Access:  func(context.Context) (string, error) { return token, nil },
		HTTP:    srv.Client(),
	}
	if _, err := tm.Search(context.Background(), "resident evil", 0); err != nil {
		t.Fatalf("search with a v4 token: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("the server answered %d times, want 1", hits.Load())
	}
}

// The shape alone decides: a 32-character credential that is not hex is a
// token, not an API key, so it rides the header.
func TestTMDBCredentialShapes(t *testing.T) {
	cases := []struct {
		name   string
		tok    string
		bearer string
		apiKey string
	}{
		{"v3 api key", "0123456789abcdef0123456789abcdef", "", "0123456789abcdef0123456789abcdef"},
		{"v3 api key with spaces", "  0123456789abcdef0123456789abcdef\n", "", "0123456789abcdef0123456789abcdef"},
		{"v4 token", "eyJhbGciOiJIUzI1NiJ9.payload.sig", "eyJhbGciOiJIUzI1NiJ9.payload.sig", ""},
		{"32 chars that are not hex", "nothexnothexnothexnothexnothex12", "nothexnothexnothexnothexnothex12", ""},
		{"old short token", "test-read-value", "test-read-value", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bearer, apiKey := tmdbCredential(c.tok)
			if bearer != c.bearer || apiKey != c.apiKey {
				t.Fatalf("tmdbCredential(%q) = (%q, %q), want (%q, %q)", c.tok, bearer, apiKey, c.bearer, c.apiKey)
			}
		})
	}
}

// withQuery keeps a query already on the URL instead of adding a second "?".
func TestWithQueryKeepsTheExistingQuery(t *testing.T) {
	if got := withQuery("https://api.themoviedb.org/3/search/movie", "api_key", "abc"); got != "https://api.themoviedb.org/3/search/movie?api_key=abc" {
		t.Fatalf("no query: %q", got)
	}
	got := withQuery("https://api.themoviedb.org/3/search/movie?query=dune&year=2024", "api_key", "abc")
	if got != "https://api.themoviedb.org/3/search/movie?query=dune&year=2024&api_key=abc" {
		t.Fatalf("existing query: %q", got)
	}
}
