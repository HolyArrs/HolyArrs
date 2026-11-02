package api

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/CoderLuii/holyarrs-metadata/internal/model"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type memoryStore struct {
	mu         sync.Mutex
	entries    map[string]model.Entry
	authorized bool
	putErr     error
}

func (m *memoryStore) Get(_ context.Context, k string) (model.Entry, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[k]
	return e, ok, nil
}
func (m *memoryStore) Put(_ context.Context, e model.Entry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.putErr != nil {
		return m.putErr
	}
	m.entries[e.Key] = e
	return nil
}
func (m *memoryStore) Authorized(_ context.Context, t string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.authorized && t == "synthetic-test-token", nil
}
func (m *memoryStore) Check(context.Context) error { return nil }
func (m *memoryStore) Ready(context.Context) error { return nil }

func TestIgnoredDetailVariantsAndInvalidIDsRejectedBeforeFetch(t *testing.T) {
	s, _, calls := setup(Options{Burst: 100})
	for _, path := range []string{lookup + "?page=2", lookup + "?limit=3", lookup + "?region=US", "/v1/shows/tvmaze/2?language=en", "/v1/movies/tmdb/not-an-id", "/v1/search?provider=tvmaze&kind=show&q=show&page=2", "/v1/search?provider=tvmaze&kind=show&q=show&limit=2"} {
		if w := req(s, path, "synthetic-test-token"); w.Code != 400 {
			t.Fatalf("%s returned %d", path, w.Code)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid request reached fetcher")
	}
}

func TestNonObjectAndProviderCredentialsNeverServeSuccessOrUnauthorized(t *testing.T) {
	for _, data := range []string{"null", "[]", "42", `"string"`} {
		s, _, _ := setup(Options{})
		s.fetcher = fetchFunc(func(context.Context, model.Query) (model.Result, error) {
			return model.Result{Data: json.RawMessage(data), Complete: true}, nil
		})
		if w := req(s, lookup, "synthetic-test-token"); w.Code != 503 {
			t.Fatalf("accepted %s status %d", data, w.Code)
		}
	}
	s, _, _ := setup(Options{})
	s.fetcher = fetchFunc(func(context.Context, model.Query) (model.Result, error) {
		return model.Result{}, &model.FetchError{Code: "provider_credentials", Status: 401}
	})
	if w := req(s, lookup, "synthetic-test-token"); w.Code != 503 || w.Header().Get("Retry-After") == "" {
		t.Fatalf("credential failure status %d", w.Code)
	}
}

func TestEmptySearchHasShortExpiryAndProviderPolicy(t *testing.T) {
	s, _, _ := setup(Options{TTL: 24 * time.Hour, ProviderTTLs: map[string]time.Duration{"tmdb": time.Hour}})
	s.fetcher = fetchFunc(func(context.Context, model.Query) (model.Result, error) {
		return model.Result{Data: json.RawMessage(`{"items":[]}`), Complete: true}, nil
	})
	w := req(s, "/v1/search?provider=tmdb&kind=movie&q=empty", "synthetic-test-token")
	var body struct {
		FetchedAt time.Time `json:"fetched_at"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	json.Unmarshal(w.Body.Bytes(), &body)
	if body.ExpiresAt.Sub(body.FetchedAt) != 5*time.Minute {
		t.Fatalf("negative TTL %v", body.ExpiresAt.Sub(body.FetchedAt))
	}
}

type fetchFunc func(context.Context, model.Query) (model.Result, error)

func (f fetchFunc) Fetch(c context.Context, q model.Query) (model.Result, error) { return f(c, q) }
func setup(o Options) (*Server, *memoryStore, *atomic.Int64) {
	m := &memoryStore{entries: map[string]model.Entry{}, authorized: true}
	calls := &atomic.Int64{}
	f := fetchFunc(func(c context.Context, q model.Query) (model.Result, error) {
		calls.Add(1)
		return model.Result{Data: json.RawMessage(`{"title":"Synthetic"}`), Complete: true}, nil
	})
	return New(m, f, o), m, calls
}
func req(s *Server, path, token string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", path, nil)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

const lookup = "/v1/movies/tmdb/42"

func TestAuthenticationAndRevocationOnWarmHit(t *testing.T) {
	s, m, calls := setup(Options{})
	if w := req(s, lookup, ""); w.Code != 401 {
		t.Fatalf("anonymous status %d", w.Code)
	}
	for range 2 {
		w := req(s, lookup, "synthetic-test-token")
		if w.Code != 200 {
			t.Fatalf("lookup %d: %s", w.Code, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("missing no-store")
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("calls %d", calls.Load())
	}
	m.mu.Lock()
	m.authorized = false
	m.mu.Unlock()
	if w := req(s, lookup, "synthetic-test-token"); w.Code != 401 {
		t.Fatalf("revoked warm status %d", w.Code)
	}
}
func TestRoutesAndInputBounds(t *testing.T) {
	s, _, _ := setup(Options{Burst: 100})
	for _, path := range []string{"/v1/search?provider=tmdb&kind=movie&q=Synthetic", "/v1/shows/tmdb/42/seasons/0", "/v1/music/musicbrainz/artist/32bc886e-42e9-4a55-8d7c-d8f48f940248"} {
		if w := req(s, path, "synthetic-test-token"); w.Code != 200 {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
	}
	for _, path := range []string{"/v1/search?provider=tmdb&kind=movie&q=a&url=http://private", "/v1/search?provider=arbitrary&kind=movie&q=a", "/v1/search?provider=tmdb&kind=movie&q=a&page=9999", "/v1/movies/tmdb/42?refresh=1", "/v1/shows/tvmaze/1/seasons/0"} {
		if w := req(s, path, "synthetic-test-token"); w.Code != 400 {
			t.Fatalf("accepted %s: %d", path, w.Code)
		}
	}
}
func TestQuotaAndMetricsProtection(t *testing.T) {
	s, _, _ := setup(Options{Burst: 2, PerMinute: 1})
	for range 2 {
		if w := req(s, lookup, "synthetic-test-token"); w.Code != 200 {
			t.Fatal(w.Code)
		}
	}
	w := req(s, lookup, "synthetic-test-token")
	if w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatalf("quota %d", w.Code)
	}
	if req(s, "/metrics", "").Code != 401 {
		t.Fatal("metrics public")
	}
}
func TestStampedeOneFetchAndCacheWrite(t *testing.T) {
	m := &memoryStore{entries: map[string]model.Entry{}, authorized: true}
	var calls atomic.Int64
	f := fetchFunc(func(c context.Context, q model.Query) (model.Result, error) {
		calls.Add(1)
		time.Sleep(30 * time.Millisecond)
		return model.Result{Data: json.RawMessage(`{"title":"Synthetic"}`), Complete: true}, nil
	})
	s := New(m, f, Options{Burst: 200, PerMinute: 100000, PerTokenConcurrent: 100, MaxConcurrent: 100})
	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			if w := req(s, lookup, "synthetic-test-token"); w.Code != 200 {
				t.Errorf("status %d", w.Code)
			}
		})
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("provider calls %d", calls.Load())
	}
}
func TestUpstreamFailureStaleAndPartialPreservation(t *testing.T) {
	q := model.Query{Provider: "tmdb", Kind: "movie", Operation: "detail", ID: "42", Page: 1, Limit: 20}
	m := &memoryStore{entries: map[string]model.Entry{q.Key(): {Key: q.Key(), Result: model.Result{Data: json.RawMessage(`{"title":"Old"}`), Complete: true}, FetchedAt: time.Now().Add(-time.Hour), ExpiresAt: time.Now().Add(-time.Minute)}}, authorized: true}
	f := fetchFunc(func(context.Context, model.Query) (model.Result, error) {
		return model.Result{}, &model.FetchError{Code: "upstream_status", Status: 503}
	})
	s := New(m, f, Options{StaleTTL: time.Hour})
	w := req(s, lookup, "synthetic-test-token")
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	var body map[string]any
	if json.Unmarshal(w.Body.Bytes(), &body) != nil || body["stale"] != true {
		t.Fatal("stale not identified")
	}
	s = New(m, fetchFunc(func(context.Context, model.Query) (model.Result, error) {
		return model.Result{Data: json.RawMessage(`{}`), Complete: false}, nil
	}), Options{StaleTTL: time.Hour})
	w = req(s, lookup, "synthetic-test-token")
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	e, _, _ := m.Get(context.Background(), q.Key())
	if string(e.Result.Data) != `{"title":"Old"}` {
		t.Fatal("partial replaced valid entry")
	}
}
func TestFailedCacheWriteStillReturnsValidatedFreshData(t *testing.T) {
	s, m, _ := setup(Options{})
	m.putErr = errors.New("disk full contains private information")
	w := req(s, lookup, "synthetic-test-token")
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w.Header().Get("X-Metadata-Cache") != "bypass" {
		t.Fatal("cache failure not identified")
	}
}
func TestHealthAndMethod(t *testing.T) {
	s, _, _ := setup(Options{})
	if req(s, "/healthz", "").Code != 200 || req(s, "/readyz", "").Code != 200 {
		t.Fatal("health failed")
	}
	r := httptest.NewRequest("POST", lookup, nil)
	r.Header.Set("Authorization", "Bearer synthetic-test-token")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 405 {
		t.Fatal(w.Code)
	}
}

// A spent upstream budget serves any cached answer, even past StaleTTL.
func TestSpentUpstreamBudgetServesCache(t *testing.T) {
	q := model.Query{Provider: "tmdb", Kind: "movie", Operation: "detail", ID: "42", Page: 1, Limit: 20}
	m := &memoryStore{entries: map[string]model.Entry{q.Key(): {Key: q.Key(), Result: model.Result{Data: json.RawMessage(`{"title":"Old"}`), Complete: true}, FetchedAt: time.Now().Add(-30 * 24 * time.Hour), ExpiresAt: time.Now().Add(-29 * 24 * time.Hour)}}, authorized: true}
	s := New(m, fetchFunc(func(context.Context, model.Query) (model.Result, error) {
		return model.Result{}, &model.FetchError{Code: "upstream_budget", Status: 503, RetryAfter: time.Minute}
	}), Options{StaleTTL: time.Hour})
	if w := req(s, lookup, "synthetic-test-token"); w.Code != 200 || w.Header().Get("X-Metadata-Cache") != "stale" {
		t.Fatalf("spent budget: %d %s", w.Code, w.Header().Get("X-Metadata-Cache"))
	}
}
