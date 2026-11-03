package metadata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestTMDBReleaseDates(t *testing.T) {
	cases := []struct {
		name, body        string
		cinema, dig, phys string
	}{
		{"us first", `{"id":1,"title":"A","release_date":"2024-02-27","release_dates":{"results":[
			{"iso_3166_1":"FR","release_dates":[{"release_date":"2024-02-20T00:00:00.000Z","type":3},{"release_date":"2024-04-01T00:00:00.000Z","type":4}]},
			{"iso_3166_1":"US","release_dates":[{"release_date":"2024-03-01T00:00:00.000Z","type":3},{"release_date":"2024-04-16T00:00:00.000Z","type":4},{"release_date":"2024-05-14T00:00:00.000Z","type":5}]}]}}`,
			"2024-03-01", "2024-04-16", "2024-05-14"},
		{"earliest elsewhere", `{"id":1,"title":"A","release_date":"2024-02-27","release_dates":{"results":[
			{"iso_3166_1":"DE","release_dates":[{"release_date":"2024-06-02T00:00:00.000Z","type":4}]},
			{"iso_3166_1":"GB","release_dates":[{"release_date":"2024-05-30T00:00:00.000Z","type":4},{"release_date":"2024-03-10T00:00:00.000Z","type":2}]}]}}`,
			"2024-03-10", "2024-05-30", ""},
		{"main date only", `{"id":1,"title":"A","release_date":"2025-07-04"}`, "2025-07-04", "", ""},
		{"no dates", `{"id":1,"title":"A","release_date":""}`, "", "", ""},
	}
	day := func(p *time.Time) string {
		if p == nil {
			return ""
		}
		return p.Format("2006-01-02")
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("append_to_response") != "release_dates,images,credits,translations,alternative_titles" {
					t.Errorf("query %v", r.URL.Query())
				}
				_, _ = w.Write([]byte(c.body))
			}))
			defer srv.Close()
			m, err := newTMDB(srv, fakeCred).Movie(context.Background(), "tmdb:1")
			if err != nil {
				t.Fatal(err)
			}
			if day(m.CinemaDate) != c.cinema || day(m.DigitalDate) != c.dig || day(m.PhysicalDate) != c.phys {
				t.Fatalf("got %s %s %s", day(m.CinemaDate), day(m.DigitalDate), day(m.PhysicalDate))
			}
		})
	}
}

func TestTMDBChangedListAndNoCache(t *testing.T) {
	var details atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/3/movie/changes":
			q := r.URL.Query()
			if q.Get("start_date") != "2026-10-01" || q.Get("end_date") == "" {
				t.Errorf("changes query %v", q)
			}
			if q.Get("page") == "1" {
				_, _ = w.Write([]byte(`{"results":[{"id":11},{"id":12}],"page":1,"total_pages":2}`))
			} else {
				_, _ = w.Write([]byte(`{"results":[{"id":13}],"page":2,"total_pages":2}`))
			}
		case "/3/movie/11":
			details.Add(1)
			_, _ = w.Write([]byte(`{"id":11,"title":"Eleven"}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	p := newTMDB(srv, fakeCred)
	var _ ChangeLister = p
	ctx := context.Background()
	ids, err := p.Changed(ctx, time.Date(2026, 10, 1, 13, 0, 0, 0, time.UTC))
	if err != nil || strings.Join(ids, ",") != "tmdb:11,tmdb:12,tmdb:13" {
		t.Fatalf("changed %v %v", ids, err)
	}
	_, _ = p.Movie(ctx, "tmdb:11")
	_, _ = p.Movie(ctx, "tmdb:11")
	if details.Load() != 1 {
		t.Fatalf("cached lookups made %d calls", details.Load())
	}
	_, _ = p.Movie(NoCache(ctx), "tmdb:11")
	if details.Load() != 2 {
		t.Fatalf("NoCache lookup made %d calls in all, want 2", details.Load())
	}
	sw := &Switch{Use: func(context.Context) bool { return false }, Then: p, Else: Lab()}
	if _, err := sw.Changed(ctx, time.Now()); err != ErrNoChangeList {
		t.Fatalf("lab changed list: %v", err)
	}
}
