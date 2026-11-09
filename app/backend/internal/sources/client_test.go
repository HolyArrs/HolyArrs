package sources

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func serve(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(h)
	t.Cleanup(s.Close)
	return s
}

func file(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestCaps_ReadsAndRemembers(t *testing.T) {
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("t") != "caps" {
			t.Errorf("t = %q", r.URL.Query().Get("t"))
		}
		_, _ = w.Write(file(t, "caps.xml"))
	})
	c := New(Config{URL: srv.URL + "/api", Protocol: "torrent", AllowLocal: true})
	caps, err := c.Caps(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !caps.MovieSearch || !caps.Supports("movie", "imdbid") || caps.Supports("movie", "season") {
		t.Fatalf("movie search caps wrong: %+v", caps)
	}
	if caps.TVSearch {
		t.Fatal("tv search should be off")
	}
	if caps.Max != 50 || len(caps.Categories) != 3 {
		t.Fatalf("limits or categories wrong: %+v", caps)
	}
}

func TestSearch_ByIMDbParsesResults(t *testing.T) {
	var got string
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("t") == "caps" {
			_, _ = w.Write(file(t, "caps.xml"))
			return
		}
		got = r.URL.RawQuery
		_, _ = w.Write(file(t, "results.xml"))
	})
	c := New(Config{URL: srv.URL + "/api", Protocol: "torrent", AllowLocal: true, APIKey: "k3y"})
	items, err := c.Search(context.Background(), Query{Title: "The Lantern Keepers", Year: 2021, IMDb: "tt0123456", Categories: []int{2000}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"t=movie", "imdbid=0123456", "cat=2000", "apikey=k3y"} {
		if !strings.Contains(got, want) {
			t.Errorf("query %q lacks %q", got, want)
		}
	}
	if len(items) != 2 {
		t.Fatalf("got %d items", len(items))
	}
	a := items[0]
	if a.Title != "The.Lantern.Keepers.2021.1080p.WEB-DL.DDP5.1.H.264-QUILL" || a.Link != "http://src/a.torrent" ||
		a.Size != 4294967296 || a.Protocol != "torrent" || a.Seeders == nil || *a.Seeders != 12 ||
		a.Peers == nil || *a.Peers != 15 || a.InfoHash != "abcdef0123456789abcdef0123456789abcdef01" {
		t.Fatalf("item a wrong: %+v", a)
	}
	if !a.Published.Equal(time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("published %v", a.Published)
	}
	b := items[1]
	if b.Link != "http://src/b.torrent" || b.Size != 1073741824 || b.Peers != nil || b.InfoHash != "" {
		t.Fatalf("item b wrong: %+v", b)
	}
}

func TestSearch_FallsBackToText(t *testing.T) {
	var got string
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("t") == "caps" {
			_, _ = w.Write([]byte(`<caps><searching><search available="yes" supportedParams="q"/><movie-search available="no"/></searching></caps>`))
			return
		}
		got = r.URL.RawQuery
		_, _ = w.Write([]byte(`<rss><channel></channel></rss>`))
	})
	c := New(Config{URL: srv.URL, Protocol: "torrent", AllowLocal: true})
	if _, err := c.Search(context.Background(), Query{Title: "Dune: Part Two", Year: 2024, IMDb: "tt15239678"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "t=search") || !strings.Contains(got, "q=Dune+Part+Two+2024") || strings.Contains(got, "apikey") {
		t.Fatalf("query %q", got)
	}
}

func TestError_KeyNotLeaked(t *testing.T) {
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<?xml version="1.0"?><error code="100" description="Incorrect user credentials"/>`))
	})
	c := New(Config{URL: srv.URL, Protocol: "torrent", AllowLocal: true, APIKey: "s3cret"})
	_, err := c.Caps(context.Background())
	var se *Error
	if !errors.As(err, &se) || se.Kind != KindLoginRefused {
		t.Fatalf("want login refused, got %v", err)
	}
	c2 := New(Config{URL: "http://127.0.0.1:1/api", Protocol: "torrent", AllowLocal: true, APIKey: "s3cret"})
	_, err = c2.Caps(context.Background())
	if err == nil || strings.Contains(err.Error(), "s3cret") {
		t.Fatalf("error leaks the key or is nil: %v", err)
	}
	if !errors.As(err, &se) || se.Kind != KindDown {
		t.Fatalf("want down, got %v", err)
	}
}

func TestSearch_RateLimitedResponse(t *testing.T) {
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusTooManyRequests)
	})
	c := New(Config{URL: srv.URL, Protocol: "torrent", AllowLocal: true})
	_, err := c.Caps(context.Background())
	var se *Error
	if !errors.As(err, &se) || se.Kind != KindRateLimited || se.RetryAfter != 120*time.Second {
		t.Fatalf("got %v", err)
	}
}

func TestClient_GapBetweenRequests(t *testing.T) {
	var n atomic.Int32
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		_, _ = w.Write(file(t, "caps.xml"))
	})
	c := New(Config{URL: srv.URL, Protocol: "torrent", AllowLocal: true, GapMS: 80})
	start := time.Now()
	for range 3 {
		if _, err := c.Caps(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if d := time.Since(start); d < 160*time.Millisecond {
		t.Fatalf("3 requests took %v, want at least 2 gaps", d)
	}
}

func TestClient_SearchesPerWindow(t *testing.T) {
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(file(t, "caps.xml"))
	})
	c := New(Config{URL: srv.URL, Protocol: "torrent", AllowLocal: true, PerWindow: 2, Window: time.Hour})
	for range 2 {
		if _, err := c.Caps(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	_, err := c.Caps(context.Background())
	var se *Error
	if !errors.As(err, &se) || se.Kind != KindRateLimited || se.RetryAfter <= 0 {
		t.Fatalf("third request should be rate limited locally, got %v", err)
	}
}

func TestBackoff_DoublesToCap(t *testing.T) {
	want := []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute}
	for i, w := range want {
		if got := Backoff(i + 1); got != w {
			t.Errorf("Backoff(%d) = %v, want %v", i+1, got, w)
		}
	}
	if Backoff(30) != 6*time.Hour {
		t.Errorf("Backoff should cap at 6h, got %v", Backoff(30))
	}
}
