package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"
)

// fakeShowSource answers fixed shows by "<kind>:<id>".
type fakeShowSource struct {
	mu    sync.Mutex
	shows map[string]metadata.Show
	err   error
}

func (f *fakeShowSource) SearchShows(_ context.Context, q string) ([]metadata.Show, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	seen := map[string]bool{}
	out := []metadata.Show{}
	for _, sh := range f.shows {
		if strings.Contains(strings.ToLower(sh.Name), strings.ToLower(q)) && !seen[sh.Name] {
			seen[sh.Name] = true
			out = append(out, sh)
		}
	}
	return out, nil
}

func (f *fakeShowSource) Show(_ context.Context, id string) (metadata.Show, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return metadata.Show{}, f.err
	}
	if sh, ok := f.shows[id]; ok {
		return sh, nil
	}
	return metadata.Show{}, metadata.ErrShowNotFound
}

func at(s string) *time.Time { t, _ := time.Parse(time.RFC3339, s); return &t }

// apiShow is a show with a finished first season and a second one far in
// the future, so the test does not depend on today's date.
func apiShow(source, id string) metadata.Show {
	return metadata.Show{Source: source, Name: "Lab Show", Year: 2025, Status: "running", Network: "Lab TV",
		IDs:     map[string]string{source: id, "tvdb": "200", "imdb": "tt0100"},
		Seasons: []metadata.Season{{Number: 0, Name: "Specials"}, {Number: 1, Name: "Season 1"}, {Number: 2, Name: "Season 2"}},
		Episodes: []metadata.Episode{
			{Season: 0, Number: 1, Name: "Behind the Lab", AirDate: at("2025-03-01T12:00:00Z"), Timed: true, AirDay: "2025-03-01"},
			{Season: 1, Number: 1, Name: "Pilot", AirDate: at("2025-01-06T02:00:00Z"), Timed: true, AirDay: "2025-01-05", ID: "e101"},
			{Season: 1, Number: 2, Name: "Second", AirDate: at("2025-01-13T02:00:00Z"), Timed: true, AirDay: "2025-01-12", ID: "e102"},
			{Season: 2, Number: 1, Name: "Return", AirDate: at("2099-01-05T02:00:00Z"), Timed: true, AirDay: "2099-01-04", ID: "e201"},
			{Season: 2, Number: 2, Name: "TBA", ID: "e202"},
		}}
}

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
	srv := httptest.NewServer(New(Server{Store: s, Queue: q, Library: lib, Metadata: &metadata.Static{}}).Handler())
	t.Cleanup(srv.Close)
	do(t, "POST", srv.URL+"/api/v1/library-folders", fmt.Sprintf(`{"path":%q,"kind":"show","label":"Shows"}`, t.TempDir()), 201)
	return srv, s, tmdb, maze
}

type addedShow struct {
	Title struct {
		ID    int64  `json:"id"`
		Kind  string `json:"kind"`
		Slots []any  `json:"slots"`
		Show  struct {
			DetailsSource string                `json:"details_source"`
			EpisodeOrder  string                `json:"episode_order"`
			SeasonChoice  string                `json:"season_choice"`
			NewSeasons    string                `json:"new_seasons"`
			Seasons       []shows.SeasonSummary `json:"seasons"`
			Episodes      int                   `json:"episodes"`
			Missing       int                   `json:"missing"`
			NotOutYet     int                   `json:"not_out_yet"`
		} `json:"show"`
	} `json:"title"`
	JobID int64 `json:"job_id"`
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

// ShowResult is a looked-up show as the lookup route answers it, with the
// open ask read back as an askBrief.
type ShowResult struct {
	metadata.Show
	TitleID  int64     `json:"title_id,omitempty"`
	NeverAdd bool      `json:"never_add,omitempty"`
	Ask      *askBrief `json:"ask,omitempty"`
}
