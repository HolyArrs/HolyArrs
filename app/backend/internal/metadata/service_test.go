package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"slices"
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

// A Japanese show reads the anime map, so a TMDB season of two cours
// carries the release-name numbers; a failed map call says so.
func TestServiceShowReleaseNumbers(t *testing.T) {
	for _, tc := range []struct {
		name, lang string
		mapStatus  int
		alt        [2]int
		stale      bool
		mapAsked   bool
	}{
		{"mapped", "ja", 200, [2]int{2, 5}, false, true},
		{"not anime", "ja", 404, [2]int{}, false, true},
		{"server error", "ja", 500, [2]int{}, true, true},
		{"not Japanese", "en", 200, [2]int{}, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var asked atomic.Int32
			s := testService(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/anime/map":
					asked.Add(1)
					if r.URL.Query().Get("tmdb") != "42" {
						t.Errorf("map query %s", r.URL.RawQuery)
					}
					if tc.mapStatus != 200 {
						w.WriteHeader(tc.mapStatus)
						return
					}
					_, _ = w.Write([]byte(`{"entries":[{"tmdb_tv":42,"tmdb_season":1,"tvdb_season":1},{"tmdb_tv":42,"tmdb_season":1,"tmdb_offset":12,"tvdb_season":2}]}`))
				case "/v1/shows/tmdb/42":
					serviceReply(w, "tmdb", "42", normalizedItem("tmdb", "42", "show", map[string]any{
						"id": 42, "name": "Series", "original_language": tc.lang, "number_of_seasons": 1, "number_of_episodes": 24,
						"seasons": []any{map[string]any{"id": 421, "season_number": 1, "episode_count": 24}},
					}))
				case "/v1/shows/tmdb/42/seasons/1":
					eps := []any{}
					for i := 1; i <= 24; i++ {
						eps = append(eps, map[string]any{"id": 1000 + i, "season_number": 1, "episode_number": i})
					}
					serviceReply(w, "tmdb", "42", normalizedItem("tmdb", "421", "show", map[string]any{"id": 421, "season_number": 1, "episodes": eps}))
				default:
					http.NotFound(w, r)
				}
			})
			got, err := s.Shows("tmdb", nil).Show(context.Background(), "tmdb:42")
			if err != nil || len(got.Episodes) != 24 {
				t.Fatalf("show: %+v, %v", got, err)
			}
			e17 := got.Episodes[16]
			if e17.Number != 17 || [2]int{e17.AltSeason, e17.AltEpisode} != tc.alt || got.AltStale != tc.stale || (asked.Load() > 0) != tc.mapAsked {
				t.Fatalf("E17 %+v, stale %v, map asked %d", e17, got.AltStale, asked.Load())
			}
		})
	}
}

// When the server fails and TMDB is asked directly, no map was read, so a
// Japanese show's stored release numbers are kept.
func TestServiceShowFallbackKeepsReleaseNumbers(t *testing.T) {
	direct := &titleDirect{show: Show{Name: "direct", OriginalLanguage: "ja", IDs: map[string]string{"tmdb": "42"}}}
	s := testService(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusBadGateway) })
	got, err := s.Shows("tmdb", direct).Show(context.Background(), "tmdb:42")
	if err != nil || got.Name != "direct" || !got.AltStale {
		t.Fatalf("fallback: %+v, %v", got, err)
	}
}

// An anime the map cannot number takes TVMaze's season sizes from the
// server, found by the show's TVDB id; a failed call keeps stored numbers.
func TestServiceShowReleaseNumbersFromTVMaze(t *testing.T) {
	for _, tc := range []struct {
		name         string
		searchStatus int
		tvdb         int
		alt          [2]int
		stale        bool
		animeMap     string
	}{
		{"split", 200, 999, [2]int{2, 5}, false, ""},
		{"other show", 200, 1000, [2]int{}, false, ""},
		{"server error", 500, 999, [2]int{}, true, ""},
		{"map agrees", 200, 999, [2]int{}, false, `{"entries":[{"tmdb_tv":42,"tmdb_season":1,"tvdb_season":1}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testService(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/anime/map":
					if tc.animeMap == "" {
						http.NotFound(w, r)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(tc.animeMap))
				case "/v1/search":
					if tc.animeMap != "" {
						t.Error("TVMaze searched although the anime map knows the show")
					}
					if q := r.URL.Query(); q.Get("provider") != "tvmaze" || q.Get("kind") != "show" {
						t.Errorf("search query %s", r.URL.RawQuery)
					}
					if tc.searchStatus != 200 {
						w.WriteHeader(tc.searchStatus)
						return
					}
					serviceReply(w, "tvmaze", "", map[string]any{"items": []any{
						normalizedItem("tvmaze", "77", "show", map[string]any{"id": 77, "name": "Series", "externals": map[string]any{"thetvdb": tc.tvdb}}),
					}, "pagination": map[string]any{"page": 1, "total_pages": 1, "total_results": 1}})
				case "/v1/shows/tvmaze/77":
					eps := []any{}
					for i := 1; i <= 24; i++ {
						eps = append(eps, map[string]any{"id": 5000 + i, "season": 1 + (i-1)/12, "number": 1 + (i-1)%12, "airdate": "2023-04-01"})
					}
					serviceReply(w, "tvmaze", "77", normalizedItem("tvmaze", "77", "show", map[string]any{"id": 77, "name": "Series", "_embedded": map[string]any{"episodes": eps}}))
				case "/v1/shows/tmdb/42":
					serviceReply(w, "tmdb", "42", normalizedItem("tmdb", "42", "show", map[string]any{
						"id": 42, "name": "Series", "original_language": "ja", "number_of_seasons": 1, "number_of_episodes": 24,
						"genres": []any{map[string]any{"id": 16, "name": "Animation"}}, "external_ids": map[string]any{"tvdb_id": 999},
						"seasons": []any{map[string]any{"id": 421, "season_number": 1, "episode_count": 24}},
					}))
				case "/v1/shows/tmdb/42/seasons/1":
					eps := []any{}
					for i := 1; i <= 24; i++ {
						eps = append(eps, map[string]any{"id": 1000 + i, "season_number": 1, "episode_number": i, "air_date": "2023-04-01"})
					}
					serviceReply(w, "tmdb", "42", normalizedItem("tmdb", "421", "show", map[string]any{"id": 421, "season_number": 1, "episodes": eps}))
				default:
					http.NotFound(w, r)
				}
			})
			got, err := s.Shows("tmdb", nil).Show(context.Background(), "tmdb:42")
			if err != nil || len(got.Episodes) != 24 {
				t.Fatalf("show: %+v, %v", got, err)
			}
			e17 := got.Episodes[16]
			if [2]int{e17.AltSeason, e17.AltEpisode} != tc.alt || got.AltStale != tc.stale {
				t.Fatalf("E17 %+v, stale %v", e17, got.AltStale)
			}
		})
	}
}

type fakeShowSource struct{ show Show }

func (f fakeShowSource) SearchShows(context.Context, string) ([]Show, error) { return nil, nil }
func (f fakeShowSource) Show(context.Context, string) (Show, error) {
	f.show.Episodes = slices.Clone(f.show.Episodes)
	return f.show, nil
}

// The live TMDB source reads TMDB itself (through /v1/tmdb), not the
// server's show detail, so release numbers are added around it. Mashle's
// live shape: TMDB season 1 holds 25 episodes (the last undated), the map
// names TMDB seasons 2 and 3 that TMDB no longer has, TVMaze lists 12 + 12.
func TestAnimeShowsNumbersTheTMDBSource(t *testing.T) {
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/anime/map":
			_, _ = w.Write([]byte(`{"entries":[{"tmdb_tv":204832,"tmdb_season":1,"tvdb_season":1,"tvdb":421737},{"tmdb_tv":204832,"tmdb_season":2,"tvdb_season":2,"tvdb":421737},{"tmdb_tv":204832,"tmdb_season":3,"tvdb_season":3,"tvdb":421737}]}`))
		case "/v1/search":
			serviceReply(w, "tvmaze", "", map[string]any{"items": []any{
				normalizedItem("tvmaze", "67396", "show", map[string]any{"id": 67396, "name": "Mashle: Magic and Muscles", "externals": map[string]any{"thetvdb": 421737}}),
			}, "pagination": map[string]any{"page": 1, "total_pages": 1, "total_results": 1}})
		case "/v1/shows/tvmaze/67396":
			eps := []any{}
			for i := 1; i <= 24; i++ {
				eps = append(eps, map[string]any{"id": 5000 + i, "season": 1 + (i-1)/12, "number": 1 + (i-1)%12, "airdate": "2023-04-08"})
			}
			serviceReply(w, "tvmaze", "67396", normalizedItem("tvmaze", "67396", "show", map[string]any{"id": 67396, "name": "Mashle: Magic and Muscles", "_embedded": map[string]any{"episodes": eps}}))
		default:
			http.NotFound(w, r)
		}
	})
	show := Show{Source: "tmdb", Name: "MASHLE: MAGIC AND MUSCLES", OriginalLanguage: "ja", Genres: []string{"Animation", "Comedy"},
		IDs: map[string]string{"tmdb": "204832", "tvdb": "421737"}, Seasons: []Season{{Number: 0}, {Number: 1}}}
	for i := 1; i <= 25; i++ {
		e := Episode{Season: 1, Number: i, AirDay: "2023-04-08"}
		if i == 25 {
			e.AirDay = ""
		}
		show.Episodes = append(show.Episodes, e)
	}
	got, err := s.AnimeShows(fakeShowSource{show}).Show(context.Background(), "tmdb:204832")
	if err != nil || len(got.Episodes) != 25 || got.AltStale {
		t.Fatalf("show: %+v, %v", got, err)
	}
	for _, c := range []struct{ i, season, number int }{{11, 0, 0}, {16, 2, 5}, {24, 2, 13}} {
		if e := got.Episodes[c.i]; e.AltSeason != c.season || e.AltEpisode != c.number {
			t.Errorf("E%d: %+v", c.i+1, e)
		}
	}
}
