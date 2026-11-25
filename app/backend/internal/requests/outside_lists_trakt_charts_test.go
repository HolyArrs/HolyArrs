package requests

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestNormalListRefTraktCharts(t *testing.T) {
	cases := []struct {
		in, want string
		bad      bool
	}{
		{in: "trending", want: "chart:trending"},
		{in: " Popular ", want: "chart:popular"},
		{in: "chart:anticipated", want: "chart:anticipated"},
		{in: "boxoffice", want: "chart:boxoffice"},
		{in: "watched-weekly", want: "chart:watched-weekly"},
		{in: "https://trakt.tv/movies/trending", want: "chart:trending"},
		{in: "https://app.trakt.tv/shows/popular?x=1", want: "chart:popular"},
		{in: "https://trakt.tv/movies/watched/monthly", want: "chart:watched-monthly"},
		{in: "https://trakt.tv/movies/boxoffice", want: "chart:boxoffice"},
		{in: "kim/favourites", want: "kim/favourites"},
		{in: "chart:nope", bad: true},
		{in: "https://trakt.tv/movies/nope", bad: true},
	}
	for _, c := range cases {
		got, err := NormalListRef(ListTrakt, c.in)
		if c.bad {
			if !errors.Is(err, ErrRef) {
				t.Errorf("%q: want ErrRef, got %q %v", c.in, got, err)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("%q: got %q %v, want %q", c.in, got, err, c.want)
		}
	}
}

// fakeTraktCharts serves Trakt's public charts. Popular answers bare titles;
// the others wrap each title with a count.
func fakeTraktCharts(t *testing.T, seen *[]string) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("trakt-api-key") != fakeTraktClient || r.Header.Get("Authorization") != "" {
			w.WriteHeader(403)
			return
		}
		*seen = append(*seen, r.URL.Path)
		movie := func(title string, tmdb int) map[string]any {
			return map[string]any{"title": title, "ids": map[string]any{"tmdb": tmdb}}
		}
		var out any
		switch r.URL.Path {
		case "/movies/trending":
			out = []map[string]any{{"watchers": 9, "movie": movie("Glass Meridian", 900004)}}
		case "/movies/popular":
			out = []map[string]any{movie("Quiet Orchard", 900003)}
		case "/movies/watched/weekly":
			out = []map[string]any{{"watcher_count": 3, "movie": movie("Glass Meridian", 900004)}, {"watcher_count": 2, "movie": movie("Quiet Orchard", 900003)}}
		default:
			w.WriteHeader(404)
			return
		}
		w.Header().Set("X-Pagination-Page-Count", "1")
		_ = json.NewEncoder(w).Encode(out)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestSync_TraktCharts(t *testing.T) {
	cases := []struct {
		ref, path string
		added     int
	}{
		{ref: "trending", path: "/movies/trending", added: 1},
		{ref: "popular", path: "/movies/popular", added: 1},
		{ref: "https://trakt.tv/movies/watched/weekly", path: "/movies/watched/weekly", added: 2},
	}
	for _, c := range cases {
		t.Run(c.ref, func(t *testing.T) {
			e := setup(t)
			var seen []string
			e.svc.TraktBase = fakeTraktCharts(t, &seen).URL
			ctx := context.Background()
			l, err := AddOutsideList(ctx, e.s, OutsideListInput{Name: "Chart", Kind: ListTrakt, Ref: c.ref}, domain.System)
			if err != nil {
				t.Fatal(err)
			}
			res, err := e.svc.Sync(ctx, l.ID, domain.System)
			if err != nil || res.Added != c.added {
				t.Fatalf("result %+v %v", res, err)
			}
			if len(seen) != 1 || seen[0] != c.path {
				t.Fatalf("asked %v", seen)
			}
		})
	}
}

func TestTraktChartPath(t *testing.T) {
	cases := []struct {
		ref, kind, want string
		bad             bool
	}{
		{ref: "chart:trending", kind: "show", want: "/shows/trending"},
		{ref: "chart:anticipated", kind: "movie", want: "/movies/anticipated"},
		{ref: "chart:watched-all", kind: "show", want: "/shows/watched/all"},
		{ref: "chart:boxoffice", kind: "movie", want: "/movies/boxoffice"},
		{ref: "chart:boxoffice", kind: "show", bad: true},
	}
	for _, c := range cases {
		got, err := traktChartPath(c.ref, c.kind)
		if c.bad != (err != nil) || got != c.want {
			t.Errorf("%s %s: got %q %v", c.ref, c.kind, got, err)
		}
	}
}

func TestAddTraktBoxOfficeShowsRefused(t *testing.T) {
	e := setup(t)
	_, err := AddOutsideList(context.Background(), e.s, OutsideListInput{Name: "Box", Kind: ListTrakt, TitleKind: "show", Ref: "boxoffice"}, domain.System)
	if err == nil || !strings.Contains(err.Error(), "box office") {
		t.Fatalf("err %v", err)
	}
}
