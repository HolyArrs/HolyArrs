package imgstore

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// TestMusicCapStopsNewMusicPictures: while the music guard reports the
// data over the cap, a music picture is not written (a retry, never a
// miss); once back under, it is.
func TestMusicCapStopsNewMusicPictures(t *testing.T) {
	g := newRig(t)
	g.album("Frank Ocean", "Blonde", "")
	g.f.set("/v1/catalog/us/search?Frank Ocean Blonde", appleAlbums(g.f, "Blonde", "Frank Ocean", 1000))
	over := errors.New("over")
	capErr := over
	g.s.o.MusicCap = func(context.Context) error { return capErr }
	ctx := context.Background()
	if !g.s.CheckMusicCap(ctx) {
		t.Fatal("guard not full")
	}
	err := g.s.Fetch(ctx, KindAlbum, albumID)
	if !errors.Is(err, ErrMusicFull) || !errors.Is(err, ErrRetry) {
		t.Fatalf("fetch over the cap: %v", err)
	}
	if _, ok := g.s.Record(ctx, KindAlbum, albumID); ok {
		t.Fatal("music picture written over the cap")
	}
	capErr = nil
	if g.s.CheckMusicCap(ctx) {
		t.Fatal("guard still full")
	}
	if err := g.s.Fetch(ctx, KindAlbum, albumID); err != nil {
		t.Fatal(err)
	}
	if _, ok := g.s.Record(ctx, KindAlbum, albumID); !ok {
		t.Fatal("not written under the cap")
	}
}

// TestCleanupEvictsMusicKindsOnly: over MaxBytes, cleanup removes music
// pictures only; movie, TV and person pictures stay.
func TestCleanupEvictsMusicKindsOnly(t *testing.T) {
	g := newRig(t)
	g.album("Frank Ocean", "Blonde", "")
	g.f.set("/v1/catalog/us/search?Frank Ocean Blonde", appleAlbums(g.f, "Blonde", "Frank Ocean", 1000))
	ctx := context.Background()
	if err := g.s.Fetch(ctx, KindAlbum, albumID); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"title-banner", "person"} {
		if _, err := g.s.db.Exec(`INSERT INTO images(kind,mbid,source,source_url,version,bytes,fetched,checked) VALUES(?,?,?,?,?,?,0,0)`,
			k, "x1", "tmdb", "u", "v", int64(1)<<40); err != nil {
			t.Fatal(err)
		}
	}
	g.s.o.MaxBytes = 1
	if n := g.s.Cleanup(ctx); n != 1 {
		t.Fatalf("removed %d, want the one album", n)
	}
	var left int
	_ = g.s.db.QueryRow(`SELECT COUNT(*) FROM images WHERE kind IN ('title-banner','person')`).Scan(&left)
	if left != 2 {
		t.Fatalf("non-music pictures evicted: %d left", left)
	}
	if _, ok := g.s.Record(ctx, KindAlbum, albumID); ok {
		t.Fatal("album not evicted")
	}
}

func TestPictureURL(t *testing.T) {
	for u, want := range map[string]bool{
		"https://is1-ssl.mzstatic.com/a.jpg": true, "https://e-cdns-images.dzcdn.net/x": true, "https://i.scdn.co/image/x": true,
		"https://assets.fanart.tv/x": true, "https://coverartarchive.org/x": true, "https://ia800.us.archive.org/f.jpg": true,
		"https://i.ytimg.com/vi/x/0.jpg": true, "https://upload.wikimedia.org/x": true,
		"http://is1-ssl.mzstatic.com/a.jpg": false, "https://169.254.169.254/latest": false, "https://127.0.0.1/x": false,
		"https://evilmzstatic.com/x": false, "https://user@i.scdn.co/x": false,
	} {
		p, _ := url.Parse(u)
		if PictureURL(p) != want {
			t.Errorf("%s: want %v", u, want)
		}
	}
}

// Security L1: an address that is not allowed is never fetched, and a
// redirect to one is not followed.
func TestDownloadHeldToAllowedHosts(t *testing.T) {
	g := newRig(t)
	g.s.o.AllowURL = func(u *url.URL) bool { return !strings.Contains(u.Path, "evil") }
	// Rebuild the client with the redirect check, as Open does.
	s2, err := Open(Options{Dir: t.TempDir(), HTTP: g.f.srv.Client(), AllowURL: g.s.o.AllowURL, Logf: func(string, ...any) {}})
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	ctx := context.Background()
	if _, _, st, err := s2.download(ctx, g.f.srv.URL+"/pic/evil/1.jpg", ""); st != http.StatusForbidden || err != nil || g.f.hit("/pic/evil/1.jpg") != 0 {
		t.Fatalf("blocked address fetched: %d %v", st, err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/go", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, g.f.srv.URL+"/pic/evil/2.jpg", http.StatusFound)
	})
	red := httptest.NewServer(mux)
	defer red.Close()
	if _, _, st, err := s2.download(ctx, red.URL+"/go", ""); st != http.StatusForbidden || err != nil || g.f.hit("/pic/evil/2.jpg") != 0 {
		t.Fatalf("redirect to a blocked address followed: %d %v", st, err)
	}
}
