package music

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// slowArtists holds Artist and Pressings until release closes, counting
// the calls; err makes them fail instead.
type slowArtists struct {
	metadata.ArtistProvider
	release            chan struct{}
	artists, pressings atomic.Int32
	err                error
}

func (s *slowArtists) Artist(ctx context.Context, mbid string) (metadata.Artist, error) {
	s.artists.Add(1)
	if s.err != nil {
		return metadata.Artist{}, s.err
	}
	select {
	case <-s.release:
	case <-ctx.Done():
		return metadata.Artist{}, ctx.Err()
	}
	return s.ArtistProvider.Artist(ctx, mbid)
}

func (s *slowArtists) Pressings(ctx context.Context, mbid string) ([]metadata.Pressing, error) {
	s.pressings.Add(1)
	if s.err != nil {
		return nil, s.err
	}
	select {
	case <-s.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return s.ArtistProvider.Pressings(ctx, mbid)
}

func getTimed(t *testing.T, url string) (*http.Response, []byte, time.Duration) {
	t.Helper()
	start := time.Now()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var b json.RawMessage
	_ = json.NewDecoder(resp.Body).Decode(&b)
	return resp, b, time.Since(start)
}

// A slow music source never fails the artist or album page: each answers
// by musicPageWait with what it has (incomplete, not kept by the browser),
// a re-ask joins the running lookup, and once it ends the full page shows.
func TestMusicPagesSlowSourceIncomplete(t *testing.T) {
	old := musicPageWait
	musicPageWait = 200 * time.Millisecond
	t.Cleanup(func() { musicPageWait = old })
	srv, _, lib, fa := musicSetup(t)
	slow := &slowArtists{ArtistProvider: fa, release: make(chan struct{})}
	lib.Artists = slow
	u := srv.URL + "/api/v1/music"

	for range 2 {
		resp, b, took := getTimed(t, u+"/artists/"+apiArtist)
		var v ArtistPageView
		_ = json.Unmarshal(b, &v)
		if resp.StatusCode != 200 || !v.Incomplete || v.MBID != apiArtist || resp.Header.Get("Cache-Control") != "no-store" || took > 2*time.Second {
			t.Fatalf("slow artist page: %d %+v %q in %v", resp.StatusCode, v, resp.Header.Get("Cache-Control"), took)
		}
	}
	if n := slow.artists.Load(); n != 1 {
		t.Fatalf("a re-ask started another artist lookup: %d", n)
	}
	resp, b, _ := getTimed(t, u+"/albums/"+apiAlbum+"?artist="+apiArtist)
	var al AlbumPageView
	_ = json.Unmarshal(b, &al)
	if resp.StatusCode != 200 || !al.Incomplete || al.MBID != apiAlbum {
		t.Fatalf("slow album page: %d %s", resp.StatusCode, b)
	}
	_, _, _ = getTimed(t, u+"/albums/"+apiAlbum+"?artist="+apiArtist)
	if n := slow.pressings.Load(); n != 1 {
		t.Fatalf("a re-ask started another pressings lookup: %d", n)
	}

	close(slow.release)
	time.Sleep(50 * time.Millisecond)
	resp, b, _ = getTimed(t, u+"/artists/"+apiArtist)
	var v ArtistPageView
	_ = json.Unmarshal(b, &v)
	if resp.StatusCode != 200 || v.Incomplete || v.Name != "Lab Band" || len(v.Albums) != 3 || resp.Header.Get("Cache-Control") != "private, max-age=60" {
		t.Fatalf("artist page after the lookup: %d %+v", resp.StatusCode, v)
	}
	resp, b, _ = getTimed(t, u+"/albums/"+apiAlbum+"?artist="+apiArtist)
	al = AlbumPageView{}
	_ = json.Unmarshal(b, &al)
	if resp.StatusCode != 200 || al.Incomplete || al.Name != "First Light" || len(al.Tracks) != 2 {
		t.Fatalf("album page after the lookup: %d %s", resp.StatusCode, b)
	}
}

// A failing music source answers 200 incomplete, never 502.
func TestMusicPagesFailingSourceNo502(t *testing.T) {
	srv, _, lib, fa := musicSetup(t)
	lib.Artists = &slowArtists{ArtistProvider: fa, err: metadata.ErrServiceUnavailable}
	u := srv.URL + "/api/v1/music"
	for _, path := range []string{"/artists/" + apiArtist, "/albums/" + apiAlbum + "?artist=" + apiArtist} {
		resp, b, _ := getTimed(t, u+path)
		var v struct {
			Incomplete bool `json:"incomplete"`
		}
		_ = json.Unmarshal(b, &v)
		if resp.StatusCode != 200 || !v.Incomplete {
			t.Fatalf("%s: %d %s", path, resp.StatusCode, b)
		}
	}
}

// Top songs start with the page (not after the artist) and show on a cold
// artist page; a failure leaves them out (Deezer's list stands in).
func TestArtistPageTopSongsCold(t *testing.T) {
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	q := jobs.New(s, jobs.Options{})
	lib := &library.Service{Store: s, Queue: q, Artists: apiMusic()}
	top := func(ctx context.Context, mbid string) (metadata.MusicTopSongs, error) {
		return metadata.MusicTopSongs{Songs: []metadata.MusicTopSong{{Title: "Glow"}}}, nil
	}
	srv := httptest.NewServer(New(Server{Store: s, Queue: q, Library: lib, Metadata: &metadata.Static{}, Deps: Deps{MusicTopSongs: top}}).Handler())
	t.Cleanup(srv.Close)
	var v ArtistPageView
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/music/artists/"+apiArtist, "", 200), &v)
	if len(v.Top) != 1 || v.Top[0].Title != "Glow" {
		t.Fatalf("top songs %+v", v.Top)
	}
}

// Side data survives a restart (lookup_cache), an old answer is served at
// once and asked again in the background, and a miss is remembered.
func TestMusicSideKeptStaleAndMiss(t *testing.T) {
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	var calls atomic.Int32
	var mu sync.Mutex
	answer, found := "one", true
	fetch := func(context.Context) (string, bool, error) {
		calls.Add(1)
		mu.Lock()
		defer mu.Unlock()
		return answer, found, nil
	}
	ctx := context.Background()
	a := New(Server{Store: s})
	if v, ok := SideGet(a.Handlers, ctx, "t", "k", time.Hour, fetch); !ok || v != "one" {
		t.Fatalf("first %q %v", v, ok)
	}
	b := New(Server{Store: s}) // a restart
	if v, ok := SideGet(b.Handlers, ctx, "t", "k", time.Hour, fetch); !ok || v != "one" || calls.Load() != 1 {
		t.Fatalf("after restart %q %v, %d calls", v, ok, calls.Load())
	}
	mu.Lock()
	answer = "two"
	mu.Unlock()
	if v, ok := SideGet(b.Handlers, ctx, "t", "k", 0, fetch); !ok || v != "one" {
		t.Fatalf("stale not served at once: %q %v", v, ok)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if v, _ := SideGet(b.Handlers, ctx, "t", "k", time.Hour, fetch); v == "two" {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if v, _ := SideGet(b.Handlers, ctx, "t", "k", time.Hour, fetch); v != "two" {
		t.Fatalf("not refreshed in the background: %q", v)
	}
	mu.Lock()
	found = false
	mu.Unlock()
	n := calls.Load()
	if _, ok := SideGet(b.Handlers, ctx, "t", "miss", time.Hour, fetch); ok {
		t.Fatal("a miss answered")
	}
	c := New(Server{Store: s})
	if _, ok := SideGet(c.Handlers, ctx, "t", "miss", time.Hour, fetch); ok || calls.Load() != n+1 {
		t.Fatalf("miss not remembered across a restart: %d calls", calls.Load()-n)
	}
}
