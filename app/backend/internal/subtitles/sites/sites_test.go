package sites

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/sites/fakesite"
)

func recorded(t *testing.T, routes map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for prefix, file := range routes {
			if strings.HasPrefix(r.URL.Path, prefix) {
				if text, ok := strings.CutPrefix(file, "text:"); ok {
					_, _ = w.Write([]byte(text))
					return
				}
				b, err := os.ReadFile(filepath.Join("testdata", file))
				if err != nil {
					t.Error(err)
				}
				_, _ = w.Write(b)
				return
			}
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func cfg(url string) Config {
	return Config{Name: "test", URL: url, GapMS: 1, AllowLocal: true}
}

func TestGestdownRecorded(t *testing.T) {
	srv := recorded(t, map[string]string{
		"/shows/search/":         "gestdown-show-search.json",
		"/subtitles/get/":        "gestdown-subtitles.json",
		"/subtitles/download/a1": "text:1\n00:00:01,000 --> 00:00:02,000\nmade up\n",
	})
	s, err := New("gestdown", cfg(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	got, err := s.Search(ctx, Query{Kind: "episode", Title: "Quiet Harbour", Season: 1, Episode: 2, TVDB: "555", Languages: []string{"en", "xx"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 { // the unfinished subtitle is left out
		t.Fatalf("%+v", got)
	}
	if got[0].Key != "a1" || !got[0].MatchedByID || got[0].Season != 1 || got[0].Episode != 2 || !got[0].Corrected || got[0].Downloads != 120 ||
		got[0].ReleaseNames[0] != "Quiet Harbour.S01E02.1080p.WEB.h264-GRP" {
		t.Errorf("first: %+v", got[0])
	}
	if !got[1].HearingImpaired {
		t.Errorf("second: %+v", got[1])
	}
	b, err := s.Download(ctx, got[0])
	if err != nil || !strings.Contains(string(b), "made up") {
		t.Fatalf("download %q %v", b, err)
	}
	// This site has no movies.
	if got, _ := s.Search(ctx, Query{Kind: "movie", Title: "The Lantern"}); len(got) != 0 {
		t.Error("movie answered")
	}
	// An unknown show gives no results rather than an error.
	srv2 := recorded(t, map[string]string{})
	s2, _ := New("gestdown", cfg(srv2.URL))
	if got, err := s2.Search(ctx, Query{Kind: "episode", Title: "Nope", Season: 1, Episode: 1, Languages: []string{"en"}}); err != nil || len(got) != 0 {
		t.Errorf("unknown: %+v %v", got, err)
	}
}

func TestPodnapisiRecorded(t *testing.T) {
	var zbuf bytes.Buffer
	zw := zip.NewWriter(&zbuf)
	w, _ := zw.Create("The.Lantern.srt")
	_, _ = w.Write([]byte("1\n00:00:01,000 --> 00:00:02,000\nmade up\n"))
	_ = zw.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/subtitles/search/advanced":
			if r.Header.Get("Accept") != "application/json" || r.URL.Query().Get("keywords") != "The Lantern" || r.URL.Query().Get("movie_type") != "movie" {
				t.Errorf("query %s", r.URL)
			}
			b, _ := os.ReadFile(filepath.Join("testdata", "podnapisi-search.json"))
			_, _ = w.Write(b)
		case "/subtitles/pN1/download":
			_, _ = w.Write(zbuf.Bytes())
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	s, _ := New("podnapisi", cfg(srv.URL))
	got, err := s.Search(context.Background(), Query{Kind: "movie", Title: "The Lantern", Year: 2010, IMDb: "tt0101", Languages: []string{"en"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 { // the French answer to an English search is dropped
		t.Fatalf("%+v", got)
	}
	if !got[0].MatchedByID || !got[0].Trusted || got[0].FPS != 23.976 || got[0].Year != 2010 || got[0].Uploader != "anna" {
		t.Errorf("first %+v", got[0])
	}
	if !got[1].HearingImpaired || got[1].MatchedByID {
		t.Errorf("second %+v", got[1])
	}
	b, err := s.Download(context.Background(), got[0])
	if err != nil || !bytes.HasPrefix(b, []byte("PK")) {
		t.Fatalf("download %v", err)
	}
}

func TestFakeSite(t *testing.T) {
	srv := httptest.NewServer(fakesite.Lab())
	t.Cleanup(srv.Close)
	s, _ := New("fake", cfg(srv.URL))
	got, err := s.Search(context.Background(), Query{Kind: "movie", Title: "Big Buck Bunny", Year: 2008, Languages: []string{"en", "es"}})
	if err != nil || len(got) != 2 {
		t.Fatalf("%+v %v", got, err)
	}
	b, err := s.Download(context.Background(), got[1])
	if err != nil || !bytes.HasPrefix(b, []byte("PK")) {
		t.Fatalf("zip download %v", err)
	}
	echo, _ := s.Search(context.Background(), Query{Kind: "episode", Title: "Quiet Harbour", Season: 1, Episode: 2, TVDB: "555", Languages: []string{"en"}})
	if len(echo) != 1 || !echo[0].MatchedByID || echo[0].Episode != 2 {
		t.Fatalf("echo %+v", echo)
	}
}

func TestErrorsAndPauses(t *testing.T) {
	status := http.StatusTooManyRequests
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if status == http.StatusTooManyRequests {
			w.Header().Set("Retry-After", "120")
		}
		if status == http.StatusOK {
			_, _ = w.Write(bytes.Repeat([]byte("x"), maxSearchBody+10))
			return
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	s, _ := New("fake", cfg(srv.URL))
	q := Query{Kind: "movie", Title: "x", Languages: []string{"en"}}
	now := time.Date(2026, 10, 14, 18, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		status int
		kind   string
		pause  time.Duration
	}{
		{http.StatusTooManyRequests, KindRateLimited, 2 * time.Minute},
		{http.StatusUnauthorized, KindLoginRefused, 12 * time.Hour},
		{http.StatusServiceUnavailable, KindDown, 20 * time.Minute},
		{http.StatusTeapot, KindUnreadable, 6 * time.Hour},
		{http.StatusOK, KindUnreadable, 6 * time.Hour}, // larger than the answer cap
	} {
		status = c.status
		_, err := s.Search(context.Background(), q)
		var se *Error
		if !errors.As(err, &se) || se.Kind != c.kind {
			t.Errorf("%d: %v", c.status, err)
			continue
		}
		if kind, d := PauseFor(err, 1, now); kind != c.kind || d != c.pause {
			t.Errorf("%d: pause %s %v", c.status, kind, d)
		}
	}
	if _, d := PauseFor(errors.New("x"), 3, now); d != 80*time.Minute {
		t.Errorf("down doubles: %v", d)
	}
	if _, d := PauseFor(errors.New("x"), 30, now); d != 6*time.Hour {
		t.Errorf("down caps: %v", d)
	}
	if _, d := PauseFor(&Error{Kind: KindDownloadLimit}, 1, now); d != 6*time.Hour {
		t.Errorf("download limit waits for the day: %v", d)
	}
}

func TestNetworkRule(t *testing.T) {
	srv := httptest.NewServer(fakesite.Lab())
	t.Cleanup(srv.Close)
	// Without the local-network switch, a site on this machine is refused after
	// DNS and before anything is sent.
	s, _ := New("fake", Config{Name: "x", URL: srv.URL, GapMS: 1})
	_, err := s.Search(context.Background(), Query{Kind: "movie", Title: "x", Languages: []string{"en"}})
	var se *Error
	if !errors.As(err, &se) || se.Kind != KindBlocked {
		t.Fatalf("local refused: %v", err)
	}
	// Redirects to another host are refused, and so are links off the site.
	other := httptest.NewServer(fakesite.Lab())
	t.Cleanup(other.Close)
	red := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/api/search", http.StatusFound)
	}))
	t.Cleanup(red.Close)
	s, _ = New("fake", cfg(red.URL))
	if _, err := s.Search(context.Background(), Query{Kind: "movie", Title: "x"}); err == nil {
		t.Error("redirect followed")
	}
	if _, err := s.Download(context.Background(), Candidate{Link: other.URL + "/api/download/x"}); err == nil {
		t.Error("foreign link followed")
	}
	if _, err := New("fake", Config{URL: "ftp://x"}); err == nil {
		t.Error("ftp accepted")
	}
	if _, err := New("fake", Config{URL: "http://user:pw@x"}); err == nil {
		t.Error("user in url accepted")
	}
	if _, err := New("nope", cfg(srv.URL)); err == nil {
		t.Error("unknown kind")
	}
}

func TestLimits(t *testing.T) {
	srv := httptest.NewServer(fakesite.Lab())
	t.Cleanup(srv.Close)
	s, _ := New("fake", Config{Name: "x", URL: srv.URL, GapMS: 1, PerWindow: 2, Window: time.Hour, AllowLocal: true})
	q := Query{Kind: "movie", Title: "Big Buck Bunny", Languages: []string{"en"}}
	for range 2 {
		if _, err := s.Search(context.Background(), q); err != nil {
			t.Fatal(err)
		}
	}
	_, err := s.Search(context.Background(), q)
	var se *Error
	if !errors.As(err, &se) || se.Kind != KindRateLimited || se.RetryAfter <= 0 {
		t.Fatalf("third: %v", err)
	}
	// The gap spaces out calls.
	g, _ := New("fake", Config{Name: "g", URL: srv.URL, GapMS: 150, AllowLocal: true})
	start := time.Now()
	for range 3 {
		_, _ = g.Search(context.Background(), q)
	}
	if time.Since(start) < 300*time.Millisecond {
		t.Error("gap not kept")
	}
}

// A site that repeats our own release name back gives no release name of its
// own, so it does not count as an exact release match. The lab site gives its
// made-up subtitles a release name of their own.
func TestEchoedReleaseIsNotAMatch(t *testing.T) {
	rel := "Quiet.Harbour.S01E02.1080p.WEB-DL-GRP"
	srv := recorded(t, map[string]string{"/api/search": `text:{"results": [{"id": "a", "language": "en", "echoed": true, "release": "` + rel + `"}, {"id": "b", "language": "en", "release": "` + rel + `"}]}`})
	s, _ := New("fake", cfg(srv.URL))
	got, err := s.Search(context.Background(), Query{Kind: "episode", Title: "Quiet Harbour", Season: 1, Episode: 2, ReleaseName: rel, Languages: []string{"en"}})
	if err != nil || len(got) != 2 {
		t.Fatalf("%+v %v", got, err)
	}
	if len(got[0].ReleaseNames) != 0 || len(got[1].ReleaseNames) != 1 {
		t.Fatalf("echo kept or own name lost: %+v", got)
	}
	lab := httptest.NewServer(fakesite.Lab())
	t.Cleanup(lab.Close)
	s, _ = New("fake", cfg(lab.URL))
	got, err = s.Search(context.Background(), Query{Kind: "episode", Title: "Quiet Harbour", Season: 1, Episode: 2, ReleaseName: rel, Languages: []string{"en"}})
	if err != nil || len(got) != 1 || len(got[0].ReleaseNames) != 1 || got[0].ReleaseNames[0] == rel {
		t.Fatalf("lab echo %+v %v", got, err)
	}
}
