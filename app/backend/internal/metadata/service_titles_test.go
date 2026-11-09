package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type titleDirect struct {
	movies []Movie
	movie  Movie
	shows  []Show
	show   Show
	err    error
	calls  atomic.Int32
	check  func(context.Context)
}

type changedTitleDirect struct {
	titleDirect
	ids []string
}

func (d *changedTitleDirect) Changed(ctx context.Context, _ time.Time) ([]string, error) {
	d.calls.Add(1)
	if d.check != nil {
		d.check(ctx)
	}
	return d.ids, d.err
}

func (d *titleDirect) Search(ctx context.Context, _ string, _ int) ([]Movie, error) {
	d.calls.Add(1)
	if d.check != nil {
		d.check(ctx)
	}
	return d.movies, d.err
}

func (d *titleDirect) Movie(ctx context.Context, _ string) (Movie, error) {
	d.calls.Add(1)
	if d.check != nil {
		d.check(ctx)
	}
	return d.movie, d.err
}

func (d *titleDirect) SearchShows(ctx context.Context, _ string) ([]Show, error) {
	d.calls.Add(1)
	if d.check != nil {
		d.check(ctx)
	}
	return d.shows, d.err
}

func (d *titleDirect) Show(ctx context.Context, _ string) (Show, error) {
	d.calls.Add(1)
	if d.check != nil {
		d.check(ctx)
	}
	return d.show, d.err
}

func normalizedItem(source, id, kind string, raw any) map[string]any {
	return map[string]any{"source": source, "source_id": id, "kind": kind, "source_data": raw}
}

func TestServiceMoviesSearchAndDetail(t *testing.T) {
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/search":
			if r.URL.Query().Get("provider") != "tmdb" || r.URL.Query().Get("kind") != "movie" || r.URL.Query().Get("page") != "1" || r.URL.Query().Get("limit") != "20" {
				t.Errorf("search query: %s", r.URL.RawQuery)
			}
			serviceReply(w, "tmdb", "", map[string]any{"items": []any{
				normalizedItem("tmdb", "41", "movie", map[string]any{"id": 41, "title": "Other Year", "release_date": "2019-01-01"}),
				normalizedItem("tmdb", "42", "movie", map[string]any{"id": 42, "title": "Wanted", "release_date": "2020-01-02", "poster_path": "/p.jpg"}),
			}, "pagination": map[string]any{"page": 1, "total_pages": 1, "total_results": 2}})
		case "/v1/movies/tmdb/42":
			serviceReply(w, "tmdb", "42", normalizedItem("tmdb", "42", "movie", map[string]any{
				"id": 42, "title": "Wanted", "overview": "<b>plain text</b>", "release_date": "2020-01-02", "poster_path": "/p.jpg",
				"belongs_to_collection": map[string]any{"id": 7, "name": "Set"},
			}))
		default:
			http.NotFound(w, r)
		}
	})
	p := s.Movies(nil)
	got, err := p.Search(context.Background(), "wanted", 0)
	if err != nil || len(got) != 2 || got[1].IDs["tmdb"] != "42" || got[1].Poster != ImageBase+"/w500/p.jpg" {
		t.Fatalf("search: %+v, %v", got, err)
	}
	m, err := p.Movie(context.Background(), "tmdb:42")
	if err != nil || !m.CollectionKnown || m.Collection == nil || m.Collection.ID != "7" || strings.Contains(m.Overview, "<") {
		t.Fatalf("detail: %+v, %v", m, err)
	}
}

func TestServiceMoviesYearSearchStaysDirect(t *testing.T) {
	var serviceCalls atomic.Int32
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		serviceCalls.Add(1)
		t.Error("year search reached service")
	})
	direct := &titleDirect{movies: []Movie{{Name: "year match", Year: 2020}}}
	got, err := s.Movies(direct).Search(context.Background(), "wanted", 2020)
	if err != nil || len(got) != 1 || got[0].Year != 2020 || direct.calls.Load() != 1 || serviceCalls.Load() != 0 {
		t.Fatalf("year direct: %+v, %v, direct %d, service %d", got, err, direct.calls.Load(), serviceCalls.Load())
	}
	if _, err := s.Movies(nil).Search(context.Background(), "wanted", 2020); !errors.Is(err, ErrServiceIncomplete) {
		t.Fatalf("year without direct: %v", err)
	}
}

func TestServiceMoviesFallbackAndInvalidNoFallback(t *testing.T) {
	direct := &titleDirect{movies: []Movie{{Name: "direct"}}, movie: Movie{Name: "direct"}}
	s := testService(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) })
	got, err := s.Movies(direct).Search(context.Background(), "x", 0)
	if err != nil || len(got) != 1 || got[0].Name != "direct" || direct.calls.Load() != 1 {
		t.Fatalf("fallback: %+v, %v, calls %d", got, err, direct.calls.Load())
	}
	direct.calls.Store(0)
	s = testService(t, func(w http.ResponseWriter, r *http.Request) {
		serviceReply(w, "tmdb", "9", normalizedItem("tmdb", "8", "movie", map[string]any{"id": 8, "title": "wrong"}))
	})
	_, err = s.Movies(direct).Movie(context.Background(), "tmdb:9")
	if !errors.Is(err, ErrServiceInvalid) || direct.calls.Load() != 0 {
		t.Fatalf("invalid fallback: %v, calls %d", err, direct.calls.Load())
	}
	if got, err := s.Movies(direct).Movie(context.Background(), "imdb:tt1"); err != nil || got.Name != "direct" || direct.calls.Load() != 1 {
		t.Fatalf("alternate id direct: %+v, %v", got, err)
	}
}

func TestServiceTitlesAccessRefusalDoesNotFallback(t *testing.T) {
	direct := &titleDirect{movies: []Movie{{Name: "direct"}}, shows: []Show{{Name: "direct"}}}
	s := testService(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) })
	if _, err := s.Movies(direct).Search(context.Background(), "x", 0); !errors.Is(err, ErrServiceAccess) {
		t.Fatalf("movie access: %v", err)
	}
	if _, err := s.Shows("tmdb", direct).SearchShows(context.Background(), "x"); !errors.Is(err, ErrServiceAccess) {
		t.Fatalf("show access: %v", err)
	}
	if direct.calls.Load() != 0 {
		t.Fatalf("access refusal fell back: %d calls", direct.calls.Load())
	}
}

func TestServiceMoviesChangedStaysDirect(t *testing.T) {
	direct := &changedTitleDirect{ids: []string{"tmdb:42"}}
	p := (&Service{}).Movies(direct)
	changed, err := p.(ChangeLister).Changed(context.Background(), time.Now())
	if err != nil || len(changed) != 1 || changed[0] != "tmdb:42" || direct.calls.Load() != 1 {
		t.Fatalf("changed passthrough: %v, %v, calls %d", changed, err, direct.calls.Load())
	}
	without := (&Service{}).Movies(&Static{})
	if _, err := without.(ChangeLister).Changed(context.Background(), time.Now()); !errors.Is(err, ErrNoChangeList) {
		t.Fatalf("missing changed list: %v", err)
	}
}

func TestServiceDirectBranchesKeepOperationDeadline(t *testing.T) {
	checks := 0
	direct := &changedTitleDirect{titleDirect: titleDirect{
		movie: Movie{Name: "direct"}, show: Show{Name: "direct"},
		check: func(ctx context.Context) {
			checks++
			deadline, ok := ctx.Deadline()
			left := time.Until(deadline)
			if !ok || left <= 0 || left > 1100*time.Millisecond {
				t.Errorf("direct deadline = %v, present %v", deadline, ok)
			}
		},
	}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	s := &Service{}
	if _, err := s.Movies(direct).Movie(ctx, "imdb:tt1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Shows("tmdb", direct).Show(ctx, "tvdb:1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Movies(direct).(ChangeLister).Changed(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	if checks != 3 {
		t.Fatalf("deadline checks = %d", checks)
	}
}

func TestServiceTMDBShowRequiresCompleteSeasons(t *testing.T) {
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/shows/tmdb/42":
			serviceReply(w, "tmdb", "42", normalizedItem("tmdb", "42", "show", map[string]any{
				"id": 42, "name": "Series", "number_of_seasons": 1, "number_of_episodes": 2,
				"seasons": []any{map[string]any{"id": 420, "season_number": 0, "name": "Specials", "episode_count": 1}, map[string]any{"id": 421, "season_number": 1, "name": "One", "episode_count": 1}},
			}))
		case "/v1/shows/tmdb/42/seasons/0":
			serviceReply(w, "tmdb", "42", normalizedItem("tmdb", "420", "show", map[string]any{
				"id": 420, "season_number": 0,
				"episodes": []any{map[string]any{"id": 1, "season_number": 0, "episode_number": 1, "name": "Special"}},
			}))
		case "/v1/shows/tmdb/42/seasons/1":
			serviceReply(w, "tmdb", "42", normalizedItem("tmdb", "421", "show", map[string]any{
				"id": 421, "season_number": 1,
				"episodes": []any{map[string]any{"id": 2, "season_number": 1, "episode_number": 1, "name": "Episode"}},
			}))
		default:
			http.NotFound(w, r)
		}
	})
	got, err := s.Shows("tmdb", nil).Show(context.Background(), "tmdb:42")
	if err != nil || got.Source != "tmdb" || got.IDs["tmdb"] != "42" || len(got.Seasons) != 2 || len(got.Episodes) != 2 || got.Episodes[0].Season != 0 {
		t.Fatalf("show: %+v, %v", got, err)
	}
}

func TestServiceTMDBShowIncompleteFallsBack(t *testing.T) {
	direct := &titleDirect{show: Show{Name: "direct", IDs: map[string]string{"tmdb": "42"}}}
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/seasons/1") {
			serviceReply(w, "tmdb", "42", normalizedItem("tmdb", "421", "show", map[string]any{"id": 421, "season_number": 1, "episodes": []any{}}))
			return
		}
		serviceReply(w, "tmdb", "42", normalizedItem("tmdb", "42", "show", map[string]any{
			"id": 42, "name": "Series", "number_of_episodes": 1,
			"seasons": []any{map[string]any{"id": 421, "season_number": 1, "episode_count": 1}},
		}))
	})
	got, err := s.Shows("tmdb", direct).Show(context.Background(), "tmdb:42")
	if err != nil || got.Name != "direct" || direct.calls.Load() != 1 {
		t.Fatalf("incomplete fallback: %+v, %v, calls %d", got, err, direct.calls.Load())
	}
}

func TestServiceTMDBShowRejectsGapsAndMismatchedSeasonIdentity(t *testing.T) {
	for _, tc := range []struct {
		name    string
		seasons []any
		season  map[string]any
	}{
		{
			name: "season gap",
			seasons: []any{
				map[string]any{"id": 421, "season_number": 1, "episode_count": 1},
				map[string]any{"id": 423, "season_number": 3, "episode_count": 1},
			},
		},
		{
			name:    "episode gap",
			seasons: []any{map[string]any{"id": 421, "season_number": 1, "episode_count": 2}},
			season: map[string]any{"id": 421, "season_number": 1, "episodes": []any{
				map[string]any{"id": 1, "season_number": 1, "episode_number": 1},
				map[string]any{"id": 3, "season_number": 1, "episode_number": 3},
			}},
		},
		{
			name:    "season identity",
			seasons: []any{map[string]any{"id": 421, "season_number": 1, "episode_count": 1}},
			season: map[string]any{"id": 422, "season_number": 1, "episodes": []any{
				map[string]any{"id": 1, "season_number": 1, "episode_number": 1},
			}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			direct := &titleDirect{show: Show{Name: "direct", IDs: map[string]string{"tmdb": "42"}}}
			s := testService(t, func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "/seasons/") {
					if tc.season == nil {
						t.Fatal("season route called for a gapped season list")
					}
					id := strconv.Itoa(tc.season["id"].(int))
					serviceReply(w, "tmdb", "42", normalizedItem("tmdb", id, "show", tc.season))
					return
				}
				serviceReply(w, "tmdb", "42", normalizedItem("tmdb", "42", "show", map[string]any{
					"id": 42, "name": "Series", "number_of_seasons": len(tc.seasons), "number_of_episodes": 2, "seasons": tc.seasons,
				}))
			})
			got, err := s.Shows("tmdb", direct).Show(context.Background(), "tmdb:42")
			if err != nil || got.Name != "direct" || direct.calls.Load() != 1 {
				t.Fatalf("incomplete fallback: %+v, %v, calls %d", got, err, direct.calls.Load())
			}
		})
	}
}

func TestServiceTVMazeEmbeddedEpisodesAndUnsafePoster(t *testing.T) {
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		serviceReply(w, "tvmaze", "7", normalizedItem("tvmaze", "7", "show", map[string]any{
			"id": 7, "name": "Maze", "summary": "<p>Safe words</p>", "image": map[string]any{"original": "https://images.example/poster.jpg"},
			"_embedded": map[string]any{"episodes": []any{
				map[string]any{"id": 70, "season": 4, "number": nil, "name": "Special", "airdate": "2020-01-01"},
				map[string]any{"id": 71, "season": 1, "number": 1, "name": "Episode", "airdate": "2020-01-02"},
			}},
		}))
	})
	got, err := s.Shows("tvmaze", nil).Show(context.Background(), "tvmaze:7")
	if err != nil || got.Source != "tvmaze" || got.Poster != "https://images.example/poster.jpg" || got.Overview != "Safe words" || len(got.Episodes) != 2 || got.Episodes[0].Season != 0 || got.Episodes[0].Number != 1 {
		t.Fatalf("tvmaze: %+v, %v", got, err)
	}

	s = testService(t, func(w http.ResponseWriter, r *http.Request) {
		bad := normalizedItem("tvmaze", "7", "show", map[string]any{"id": 7, "name": "Maze", "image": map[string]any{"original": "javascript:alert(1)"}, "_embedded": map[string]any{"episodes": []any{}}})
		serviceReply(w, "tvmaze", "7", bad)
	})
	if _, err := s.Shows("tvmaze", nil).Show(context.Background(), "tvmaze:7"); !errors.Is(err, ErrServiceInvalid) {
		t.Fatalf("unsafe poster: %v", err)
	}
}

func TestServiceShowSearchIdentityAndMalformedProvider(t *testing.T) {
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		data := map[string]any{"items": []any{normalizedItem("tvmaze", "5", "show", map[string]any{"id": 5, "name": "Found"})}, "pagination": map[string]any{"page": 1, "total_pages": 1, "total_results": 1}}
		serviceReply(w, "tvmaze", "", data)
	})
	got, err := s.Shows("tvmaze", nil).SearchShows(context.Background(), "found")
	if err != nil || len(got) != 1 || got[0].Source != "tvmaze" || got[0].IDs["tvmaze"] != "5" {
		t.Fatalf("search: %+v, %v", got, err)
	}
	if _, err := s.Shows("other", nil).SearchShows(context.Background(), "found"); !errors.Is(err, ErrUnknownSource) {
		t.Fatalf("unknown source: %v", err)
	}
}

func TestNormalizedFixtureRemainsJSON(t *testing.T) {
	// Keep the helper constrained to JSON-compatible fixture values.
	if _, err := json.Marshal(normalizedItem("tmdb", "1", "movie", map[string]any{"id": 1})); err != nil {
		t.Fatal(err)
	}
}
