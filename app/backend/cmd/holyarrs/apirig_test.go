package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"
)

// Helpers for the full-server tests.

func day(s string) *time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return &t
}

// libSetup is an API with two TMDB-like movies, no Library folder yet and a
// running queue with the library's handlers and a fake search.
func libSetup(t *testing.T) (*httptest.Server, *database.Store, chan int64) {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	md := &metadata.Static{Movies: []metadata.Movie{
		{Name: "Dune: Part Two", Year: 2024, IDs: map[string]string{"tmdb": "693134", "imdb": "tt15239678"},
			CinemaDate: day("2024-03-01"), DigitalDate: day("2024-04-16")},
		{Name: "The Lantern Keepers", Year: 2031, IDs: map[string]string{"tmdb": "900001"}, CinemaDate: day("2031-02-01")},
	}}
	q := jobs.New(s, jobs.Options{Workers: 2, PollInterval: 5 * time.Millisecond})
	svc := &library.Service{Store: s, Queue: q, Metadata: md}
	svc.Register(q)
	searched := make(chan int64, 10)
	q.Handle(library.KindSearchSlot, func(ctx context.Context, r *jobs.Run) error { searched <- r.Job.Subject.ID; return nil })
	if err := q.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(q.Stop)
	srv := httptest.NewServer(buildHandlers(services{Store: s, Queue: q, Metadata: md, Version: "test", Library: svc}).Handler())
	t.Cleanup(srv.Close)
	return srv, s, searched
}

type addAnswer struct {
	Title TitleView `json:"title"`
	JobID int64     `json:"job_id"`
}

func addMovie(t *testing.T, base, body string) addAnswer {
	t.Helper()
	var a addAnswer
	_ = json.Unmarshal(do(t, "POST", base+"/api/v1/titles", body, 201), &a)
	return a
}
func folder(t *testing.T, base, kind string) library.FolderView {
	t.Helper()
	var lf library.FolderView
	_ = json.Unmarshal(do(t, "POST", base+"/api/v1/library-folders", fmt.Sprintf(`{"path":%q,"kind":%q,"label":%q}`, t.TempDir(), kind, kind+t.Name()), 201), &lf)
	return lf
}

func execRow(t *testing.T, st *database.Store, q string, args ...any) {
	t.Helper()
	if err := st.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(q, args...)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

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

type liveFrame struct {
	id, topic, data string
}

func (x liveSetup) streamWith(t *testing.T, ctx context.Context, c *http.Client, path string) <-chan liveFrame {
	t.Helper()
	req, _ := http.NewRequestWithContext(ctx, "GET", path, nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("live: %d", resp.StatusCode)
	}
	out := make(chan liveFrame, 64)
	go func() {
		defer resp.Body.Close()
		sc := bufio.NewScanner(resp.Body)
		var f liveFrame
		for sc.Scan() {
			l := sc.Text()
			switch {
			case l == "":
				if f.data != "" {
					out <- f
				}
				f = liveFrame{}
			case strings.HasPrefix(l, "id: "):
				f.id = l[4:]
			case strings.HasPrefix(l, "data: "):
				var env struct {
					Topic string          `json:"topic"`
					Data  json.RawMessage `json:"data"`
				}
				_ = json.Unmarshal([]byte(l[6:]), &env)
				f.topic, f.data = env.Topic, string(env.Data)
			}
		}
	}()
	return out
}
func frames(ch <-chan liveFrame, wait time.Duration) []liveFrame {
	var out []liveFrame
	deadline := time.After(wait)
	for {
		select {
		case f := <-ch:
			out = append(out, f)
		case <-deadline:
			return out
		}
	}
}

// showResult is a looked-up show as the lookup route answers it, with the
// open ask read back.
type showResult struct {
	metadata.Show
	TitleID  int64              `json:"title_id,omitempty"`
	NeverAdd bool               `json:"never_add,omitempty"`
	Ask      *requests.AskBrief `json:"ask,omitempty"`
}
