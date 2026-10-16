package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func serviceReply(w http.ResponseWriter, source, id string, data any) {
	_ = json.NewEncoder(w).Encode(map[string]any{"schema_version": 1, "source": source, "source_id": id,
		"fetched_at": time.Now().UTC().Add(-time.Second), "expires_at": time.Now().UTC().Add(time.Hour),
		"stale": false, "completeness": "complete", "data": data})
}

func testService(t *testing.T, handler http.HandlerFunc) *Service {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	s, err := NewService(ServiceOptions{URL: srv.URL, HTTP: srv.Client(), AllowLocal: true,
		Access: func(context.Context) (string, error) { return "fixture-installation", nil }})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestServiceURLAndCredentialBoundaries(t *testing.T) {
	for _, u := range []string{"", "http://example.com", "http://example.com?x=1", "https://user:pass@example.com", "https://example.com/path", "https://example.com#x", "http://169.254.169.254", "http://100.100.100.200", "https://metadata.google.internal", "http://0.0.0.0", "http://[::]", "http://224.0.0.1"} {
		if _, err := NewService(ServiceOptions{URL: u, HTTP: &http.Client{}, AllowLocal: true, Access: func(context.Context) (string, error) { return "fixture-installation", nil }}); err == nil {
			t.Fatalf("accepted unsafe URL %q", u)
		}
	}
	if _, err := NewService(ServiceOptions{URL: "https://example.com", Access: func(context.Context) (string, error) { return "fixture-installation", nil }}); err == nil {
		t.Fatal("nil guarded client accepted")
	}
	var calls atomic.Int32
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer fixture-installation" || r.Header.Get("Cookie") != "" || r.URL.Query().Has("token") {
			t.Error("credential boundary")
		}
		w.WriteHeader(http.StatusUnauthorized)
	})
	_, err := s.detail(context.Background(), "tmdb", "movie", "1")
	if !errors.Is(err, ErrServiceAccess) || calls.Load() != 1 {
		t.Fatalf("401: %v, %d calls", err, calls.Load())
	}
}

func TestServiceNoRedirectOrCachedRevocation(t *testing.T) {
	var leaked atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked.Add(1) }))
	defer other.Close()
	s := testService(t, func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, other.URL, http.StatusFound) })
	_, err := s.detail(context.Background(), "tmdb", "movie", "1")
	if err == nil || leaked.Load() != 0 {
		t.Fatal("redirect followed")
	}
	var calls atomic.Int32
	s = testService(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) > 1 {
			w.WriteHeader(401)
			return
		}
		serviceReply(w, "tmdb", "1", map[string]any{"source": "tmdb", "source_id": "1", "kind": "movie", "source_data": map[string]any{"id": 1, "title": "Fixture"}})
	})
	if _, err = s.detail(context.Background(), "tmdb", "movie", "1"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.detail(context.Background(), "tmdb", "movie", "1"); !errors.Is(err, ErrServiceAccess) {
		t.Fatalf("revoked: %v", err)
	}
}

func TestServiceFreshnessSchemaBoundsAndCancellation(t *testing.T) {
	for _, body := range []string{`{}`, `{"schema_version":2}`, `{"schema_version":1} trailing`, strings.Repeat(" ", (8<<20)+1)} {
		s := testService(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) })
		if _, err := s.detail(context.Background(), "tmdb", "movie", "1"); !errors.Is(err, ErrServiceInvalid) {
			t.Fatalf("invalid response: %v", err)
		}
	}
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"schema_version": 1, "source": "tmdb", "source_id": "1", "fetched_at": time.Now().Add(-2 * time.Hour), "expires_at": time.Now().Add(-time.Hour), "stale": false, "completeness": "complete", "data": map[string]any{}})
	})
	if _, err := s.detail(context.Background(), "tmdb", "movie", "1"); !errors.Is(err, ErrServiceInvalid) {
		t.Fatalf("expired: %v", err)
	}
	s = testService(t, func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := s.detail(ctx, "tmdb", "movie", "1"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline: %v", err)
	}
}

func TestServiceRetriesBoundedAndIdentity(t *testing.T) {
	for _, status := range []int{429, 503} {
		var calls atomic.Int32
		s := testService(t, func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(status)
		})
		_, err := s.detail(context.Background(), "tmdb", "movie", "1")
		if calls.Load() != 2 || !serviceFallback(context.Background(), err) {
			t.Fatalf("retry %d: %d %v", status, calls.Load(), err)
		}
	}
	s := testService(t, func(w http.ResponseWriter, r *http.Request) { serviceReply(w, "tvmaze", "1", map[string]any{}) })
	if _, err := s.detail(context.Background(), "tmdb", "movie", "1"); !errors.Is(err, ErrServiceInvalid) {
		t.Fatalf("identity: %v", err)
	}
}

func TestServiceOtherAvailabilityFailuresDoNotRetry(t *testing.T) {
	for _, status := range []int{408, 500, 502, 504} {
		var calls atomic.Int32
		s := testService(t, func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			w.WriteHeader(status)
		})
		_, err := s.detail(context.Background(), "tmdb", "movie", "1")
		if calls.Load() != 1 || !errors.Is(err, ErrServiceUnavailable) || !serviceFallback(context.Background(), err) {
			t.Fatalf("status %d: calls=%d error=%v", status, calls.Load(), err)
		}
	}
}

func TestServiceRejectsNonSearchCursorAndInvalidTimestamps(t *testing.T) {
	for _, change := range []string{"cursor", "zero", "future", "stale"} {
		for _, season := range []bool{false, true} {
			s := testService(t, func(w http.ResponseWriter, r *http.Request) {
				fetched := time.Now().Add(-time.Second)
				cursor := ""
				if change == "cursor" {
					cursor = "2"
				}
				if change == "zero" {
					fetched = time.Time{}
				}
				if change == "future" {
					fetched = time.Now().Add(2 * time.Minute)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"schema_version": 1, "source": "tmdb", "source_id": "1", "fetched_at": fetched,
					"expires_at": time.Now().Add(time.Hour), "stale": change == "stale", "completeness": "complete", "next_cursor": cursor,
					"data": map[string]any{"source": "tmdb", "source_id": "1", "kind": "movie", "source_data": map[string]any{"id": 1}}})
			})
			var err error
			if season {
				_, err = s.season(context.Background(), "1", 1)
			} else {
				_, err = s.detail(context.Background(), "tmdb", "movie", "1")
			}
			if !errors.Is(err, ErrServiceInvalid) {
				t.Fatalf("%s season=%v: %v", change, season, err)
			}
		}
	}
}

func TestServiceRetryTerminalOutcomeDoesNotOpenBreaker(t *testing.T) {
	for _, mode := range []string{"access", "oversize"} {
		var calls, accesses atomic.Int32
		s := testService(t, func(w http.ResponseWriter, r *http.Request) {
			if calls.Add(1)%2 == 1 {
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(503)
				return
			}
			_, _ = w.Write([]byte(strings.Repeat(" ", (8<<20)+1)))
		})
		if mode == "access" {
			s.access = func(context.Context) (string, error) {
				if accesses.Add(1)%2 == 0 {
					return "", errors.New("private failure")
				}
				return "fixture-installation", nil
			}
		}
		for range 4 {
			if mode == "access" {
				calls.Store(0)
			}
			_, err := s.detail(context.Background(), "tmdb", "movie", "1")
			want := ErrServiceInvalid
			if mode == "access" {
				want = ErrServiceAccess
			}
			if !errors.Is(err, want) || serviceFallback(context.Background(), err) {
				t.Fatalf("%s terminal outcome: %v", mode, err)
			}
		}
	}
}

func TestServiceClearsCookieJarAndRedactsPrivateErrors(t *testing.T) {
	const secret = "PRIVATE_TEST_INSTALLATION"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "" {
			t.Error("cookie jar leaked")
		}
		w.WriteHeader(401)
		_, _ = w.Write([]byte(secret))
	}))
	defer srv.Close()
	cl := srv.Client()
	cl.Jar, _ = cookiejar.New(nil)
	u, _ := url.Parse(srv.URL)
	cl.Jar.SetCookies(u, []*http.Cookie{{Name: "provider", Value: secret}})
	s, err := NewService(ServiceOptions{URL: srv.URL, HTTP: cl, AllowLocal: true, Access: func(context.Context) (string, error) { return secret, nil }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.detail(context.Background(), "tmdb", "movie", "1"); !errors.Is(err, ErrServiceAccess) || strings.Contains(err.Error(), secret) {
		t.Fatalf("body leak: %v", err)
	}
	s.access = func(context.Context) (string, error) { return "", errors.New(secret) }
	if _, err = s.detail(context.Background(), "tmdb", "movie", "1"); !errors.Is(err, ErrServiceAccess) || strings.Contains(err.Error(), secret) {
		t.Fatalf("callback leak: %v", err)
	}
}
