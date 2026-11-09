package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

func showSetup(t *testing.T) (*httptest.Server, *database.Store, *fakeShowSource, *fakeShowSource) {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	tmdb := &fakeShowSource{shows: map[string]metadata.Show{"tmdb:100": apiShow("tmdb", "100"), "tvdb:200": apiShow("tmdb", "100")}}
	maze := &fakeShowSource{shows: map[string]metadata.Show{"tvmaze:300": apiShow("tvmaze", "300"), "tvdb:200": apiShow("tvmaze", "300")}}
	q := jobs.New(s, jobs.Options{})
	lib := &library.Service{Store: s, Queue: q, Shows: metadata.ShowSources{"tmdb": tmdb, "tvmaze": maze,
		"tvdb": &fakeShowSource{err: metadata.ErrNoTVDBKey}}}
	srv := httptest.NewServer(routes(buildHandlers(services{Store: s, Queue: q, Library: lib, Metadata: &metadata.Static{}}), config{}))
	t.Cleanup(srv.Close)
	do(t, "POST", srv.URL+"/api/v1/library-folders", fmt.Sprintf(`{"path":%q,"kind":"show","label":"Shows"}`, t.TempDir()), 201)
	return srv, s, tmdb, maze
}

func animeTMDB(t *testing.T) metadata.Provider {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/3/genre/movie/list", "/3/genre/tv/list":
			_, _ = w.Write([]byte(`{"genres":[{"id":16,"name":"Animation"}]}`))
		case "/3/tv/9000":
			_, _ = w.Write([]byte(`{"id":9000,"name":"Synthetic Show","genres":[{"id":16,"name":"Animation"}],"origin_country":["JP"],"original_language":"ja",
				"seasons":[{"season_number":1,"episode_count":12},{"season_number":2,"episode_count":24}]}`))
		case "/3/tv/9100":
			_, _ = w.Write([]byte(`{"id":9100,"name":"Other Cartoon","genres":[{"id":16,"name":"Animation"}],"origin_country":["JP"],"original_language":"ja"}`))
		case "/3/tv/9300":
			_, _ = w.Write([]byte(`{"id":9300,"name":"Kid Show","genres":[{"id":16,"name":"Animation"}],"origin_country":["JP"],"content_ratings":{"results":[{"iso_3166_1":"US","rating":"TV-Y"}]}}`))
		case "/3/tv/9400":
			_, _ = w.Write([]byte(`{"id":9400,"name":"Grown Show","genres":[{"id":16,"name":"Animation"}],"origin_country":["JP"],"content_ratings":{"results":[{"iso_3166_1":"US","rating":"TV-MA"}]}}`))
		case "/3/tv/9200":
			_, _ = w.Write([]byte(`{"id":9200,"name":"Drama","genres":[],"origin_country":["US"],"original_language":"en"}`))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return &metadata.TMDB{BaseURL: srv.URL + "/3", Access: func(context.Context) (string, error) { return "fake-read-value", nil }, Gap: time.Millisecond}
}

func addEvent(t *testing.T, s *database.Store, titleID, slotID int64) {
	t.Helper()
	if err := s.AddEvent(context.Background(), &domain.Event{Actor: domain.System, Kind: "test_version", TitleID: &titleID, SlotID: &slotID,
		Summary: "test", Data: map[string]any{}}); err != nil {
		t.Fatal(err)
	}
}

// showsStore is an empty Store closed when the test ends.
func showsStore(t *testing.T) *database.Store {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}
