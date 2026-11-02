package extras

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const fakeKey = "synthetic-fake-key"

func TestFanartPicksEnglishThenNone(t *testing.T) {
	var calls atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("api-key") != fakeKey {
			w.WriteHeader(401)
			return
		}
		switch r.URL.Path {
		case "/movies/603":
			w.Write([]byte(`{"hdmovielogo":[{"url":"https://a/de.png","lang":"de","likes":"9"},{"url":"https://a/none.png","lang":"00","likes":"5"},{"url":"https://a/en.png","lang":"en","likes":"1"}],"moviebackground":[{"url":"https://a/bg1.jpg","lang":"","likes":"1"},{"url":"https://a/bg2.jpg","lang":"","likes":"3"}]}`))
		case "/music/5b11f4ce-a62d-471e-81fc-a69a8278c7da":
			w.Write([]byte(`{"artistthumb":[{"url":"https://a/t.jpg","lang":"","likes":"1"}],"musiclogo":[{"url":"https://a/l.png","lang":"en","likes":"1"}]}`))
		case "/tv/1":
			w.Write([]byte(`{"hdtvlogo":[{"url":"https://a/fr.png","lang":"fr","likes":"1"}]}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer up.Close()
	f := &Fanart{Key: fakeKey, URL: up.URL}
	body, cache, err := f.Art(context.Background(), "movie", "603")
	var a Art
	if err != nil || cache != "miss" || json.Unmarshal(body, &a) != nil || a.Logo != "https://a/en.png" || len(a.Backgrounds) != 2 || a.Backgrounds[0] != "https://a/bg2.jpg" {
		t.Fatalf("movie %v %s %s", err, cache, body)
	}
	if _, cache, _ := f.Art(context.Background(), "movie", "603"); cache != "hit" || calls.Load() != 1 {
		t.Fatalf("cache %s %d", cache, calls.Load())
	}
	body, _, err = f.Art(context.Background(), "music", "5b11f4ce-a62d-471e-81fc-a69a8278c7da")
	if err != nil || json.Unmarshal(body, &a) != nil || a.Thumb != "https://a/t.jpg" || a.Logo != "https://a/l.png" {
		t.Fatalf("music %v %s", err, body)
	}
	f.TVDB = func(_ context.Context, tmdb string) (string, error) {
		if tmdb == "1399" {
			return "1", nil
		}
		return "", ErrNotFound
	}
	if _, _, err := f.Art(context.Background(), "tv", "1399"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other language only %v", err)
	}
	if _, _, err := f.Art(context.Background(), "tv", "2"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing %v", err)
	}
	if s := f.Status(); s.State != StateOK || s.Cached == 0 {
		t.Fatalf("status %+v", s)
	}
}

func TestDisabledWithoutKey(t *testing.T) {
	var f *Fanart
	if f.Status().State != StateDisabledNoKey {
		t.Fatal("nil fanart")
	}
	if _, _, err := (&Fanart{}).Art(context.Background(), "movie", "1"); !errors.Is(err, ErrDisabled) {
		t.Fatal(err)
	}
	m := &MDBList{}
	if _, _, err := m.Ratings(context.Background(), "movie", "1"); !errors.Is(err, ErrDisabled) || m.Status().State != StateDisabledNoKey {
		t.Fatal(err)
	}
}

// mdbUp is a fake MDBList: 603 found (released as given), 404 otherwise,
// 500 while fail is set; it counts the asks.
type mdbUp struct {
	srv      *httptest.Server
	asks     atomic.Int32
	fail     atomic.Bool
	released atomic.Value
}

func newMDBUp(t *testing.T) *mdbUp {
	u := &mdbUp{}
	u.released.Store("1999-03-31")
	u.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u.asks.Add(1)
		if u.fail.Load() {
			w.WriteHeader(500)
			return
		}
		if r.URL.Query().Get("apikey") != fakeKey || r.URL.Path != "/tmdb/movie/603" {
			w.WriteHeader(404)
			return
		}
		w.Write([]byte(`{"ids":{"imdb":"tt0133093"},"released":"` + u.released.Load().(string) + `","ratings":[{"source":"imdb","value":8.7,"votes":2000000},{"source":"tomatoes","value":83,"votes":150},{"source":"popcorn","value":85,"votes":250000},{"source":"metacritic","value":73,"votes":35},{"source":"letterboxd","value":4.2,"votes":900000},{"source":"trakt","value":87,"votes":60000},{"source":"rogerebert","value":3},{"source":"tmdb","value":null}]}`))
	}))
	t.Cleanup(u.srv.Close)
	return u
}

func openMDB(t *testing.T, up *mdbUp, path string, now *time.Time) *MDBList {
	t.Helper()
	m := &MDBList{Key: fakeKey, URL: up.srv.URL, Now: func() time.Time { return *now }}
	if err := m.Open(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close() })
	return m
}

func TestMDBListStoredForGood(t *testing.T) {
	up := newMDBUp(t)
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "ratings.sqlite")
	m := openMDB(t, up, path, &now)
	body, cache, err := m.Ratings(context.Background(), "movie", "603")
	var r Ratings
	if err != nil || cache != "miss" || json.Unmarshal(body, &r) != nil || len(r.Scores) != 6 || r.IMDbID != "tt0133093" || r.Scores[0].Votes != 2000000 {
		t.Fatalf("ratings %v %s %s", err, cache, body)
	}
	if strings.Contains(string(body), fakeKey) {
		t.Fatal("key leaked")
	}
	if _, _, err := m.Ratings(context.Background(), "tv", "603"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown %v", err)
	}
	m.Close()
	// After a restart nothing is fetched again: both answers are stored.
	m2 := openMDB(t, up, path, &now)
	asks := up.asks.Load()
	if b, cache, err := m2.Ratings(context.Background(), "movie", "603"); err != nil || cache != "hit" || string(b) != string(body) {
		t.Fatalf("after restart %v %s", err, cache)
	}
	if _, _, err := m2.Ratings(context.Background(), "tv", "603"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a not found is kept: %v", err)
	}
	if up.asks.Load() != asks {
		t.Fatal("fetched again after a restart")
	}
}

func TestMDBListDueAnswerServedAtOnceRefreshedOnce(t *testing.T) {
	up := newMDBUp(t)
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	m := openMDB(t, up, filepath.Join(t.TempDir(), "r.sqlite"), &now)
	first, _, _ := m.Ratings(context.Background(), "movie", "603")
	now = now.Add(100 * 24 * time.Hour) // past the 90 days of an old title
	asks := up.asks.Load()
	for range 5 {
		if b, cache, err := m.Ratings(context.Background(), "movie", "603"); err != nil || cache != "hit" || string(b) != string(first) {
			t.Fatalf("stale answer at once: %v %s", err, cache)
		}
	}
	deadline := time.Now().Add(2 * time.Second)
	for up.asks.Load() == asks && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	if n := up.asks.Load() - asks; n != 1 {
		t.Fatalf("one background refresh, got %d", n)
	}
}

func TestMDBListDueTimesByReleaseAge(t *testing.T) {
	now := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	for released, want := range map[string]time.Duration{"2026-10-10": 2 * 24 * time.Hour, "2026-03-01": 14 * 24 * time.Hour, "1999-03-31": 90 * 24 * time.Hour, "": 90 * 24 * time.Hour} {
		got := dueAfter([]byte(`{"released":"`+released+`"}`), now)
		if got < want*9/10 || got > want*11/10 {
			t.Fatalf("%q: %v, want %v (10%% jitter)", released, got, want)
		}
	}
}

func TestMDBListErrorKeepsTheStoredCopy(t *testing.T) {
	up := newMDBUp(t)
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	m := openMDB(t, up, filepath.Join(t.TempDir(), "r.sqlite"), &now)
	first, _, _ := m.Ratings(context.Background(), "movie", "603")
	up.fail.Store(true)
	now = now.Add(100 * 24 * time.Hour)
	// The refresh fails (as a spent budget does): the stored copy stays.
	if b, err := m.load(context.Background(), "movie", "603"); err != nil || string(b) != string(first) {
		t.Fatalf("failure keeps the copy: %v", err)
	}
	if b, cache, err := m.Ratings(context.Background(), "movie", "603"); err != nil || cache != "hit" || string(b) != string(first) {
		t.Fatalf("served after a failure: %v %s", err, cache)
	}
	// A failure on a title never stored is no answer, and nothing is kept.
	if _, _, err := m.Ratings(context.Background(), "movie", "604"); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("a failure is not a not found: %v", err)
	}
	if s := m.Status(); s.Failures < 2 || strings.Contains(s.LastError, fakeKey) || s.Cached != 1 {
		t.Fatalf("status %+v", s)
	}
	up.fail.Store(false)
	if _, _, err := m.Ratings(context.Background(), "movie", "604"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("asked again once the source answers: %v", err)
	}
}

func TestPickFanartNewestFirst(t *testing.T) {
	got := pickFanart([]fanartImage{
		{URL: "https://assets.fanart.tv/fanart/drake-4fd17958f1286.jpg", Likes: "50"},
		{URL: "https://assets.fanart.tv/fanart/drake-6aa417465e288.jpg", Likes: "1"},
		{URL: "https://assets.fanart.tv/fanart/drake-54df9a92f2dd8.jpg", Likes: "9"},
	})
	want := []string{"https://assets.fanart.tv/fanart/drake-6aa417465e288.jpg", "https://assets.fanart.tv/fanart/drake-54df9a92f2dd8.jpg", "https://assets.fanart.tv/fanart/drake-4fd17958f1286.jpg"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%v", got)
	}
}
