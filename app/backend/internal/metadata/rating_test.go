package metadata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// TestTMDBRatingUsesLightCalls checks a rating comes from release_dates or
// content_ratings alone, with the same US choice as the details pages.
func TestTMDBRatingUsesLightCalls(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		switch r.URL.Path {
		case "/3/movie/11/release_dates":
			_, _ = w.Write([]byte(`{"id":11,"results":[{"iso_3166_1":"GB","release_dates":[{"type":3,"certification":"15"}]},
				{"iso_3166_1":"US","release_dates":[{"type":4,"certification":"R"},{"type":3,"certification":"PG-13"}]}]}`))
		case "/3/tv/12/content_ratings":
			_, _ = w.Write([]byte(`{"id":12,"results":[{"iso_3166_1":"DE","rating":"16"},{"iso_3166_1":"US","rating":"TV-PG"}]}`))
		case "/3/movie/13/release_dates":
			_, _ = w.Write([]byte(`{"id":13,"results":[]}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	tm := &TMDB{BaseURL: srv.URL + "/3", Access: func(context.Context) (string, error) { return "fake", nil }, Gap: time.Millisecond}
	for _, c := range []struct{ kind, id, want string }{{"movie", "11", "PG-13"}, {"show", "12", "TV-PG"}, {"movie", "13", ""}} {
		got, err := tm.Rating(context.Background(), c.kind, c.id)
		if err != nil || got != c.want {
			t.Fatalf("%s %s: %q %v, want %q", c.kind, c.id, got, err, c.want)
		}
	}
	if _, err := tm.Rating(context.Background(), "movie", "99"); err == nil {
		t.Fatal("unknown movie gave no error")
	}
	want := []string{"/3/movie/11/release_dates", "/3/tv/12/content_ratings", "/3/movie/13/release_dates", "/3/movie/99/release_dates"}
	if len(paths) != len(want) {
		t.Fatalf("paths %v", paths)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("paths %v, want %v", paths, want)
		}
	}
	var p Provider = &Switch{Use: func(context.Context) bool { return true }, Then: tm, Else: tm}
	if _, ok := RaterFor(context.Background(), p); !ok {
		t.Fatal("a switch to TMDB has no rater")
	}
}
