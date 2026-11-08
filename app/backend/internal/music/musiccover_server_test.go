package music

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/music/rowtpl"
)

// An album cover comes only from the metadata server's stored copy; one it
// has not stored (a redirect) is a miss, never an outside fetch; any
// catalog album is asked for at once, listed by a page or not.
func TestAlbumCoverServerCopyFirstAndCatalogAlbums(t *testing.T) {
	t.Parallel()
	_, s := discoverRig(t, newDiscoverTMDB(t).provider())
	const stored, cold, stranger = "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222", "33333333-3333-4333-8333-333333333333"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/img/album/" + stored + "/500.webp":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("\xff\xd8\xff stored"))
		case "/v1/img/album/" + cold + "/500.webp", "/v1/img/album/" + stranger + "/500.webp":
			http.Redirect(w, r, "/release-group/x/front-250", http.StatusFound)
		default:
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("\xff\xd8\xff archive " + r.URL.Path))
		}
	}))
	defer srv.Close()
	art := &library.ArtworkCache{Dir: filepath.Join(t.TempDir(), "art"), Allowed: []string{srv.URL + "/"}, HTTP: srv.Client()}
	lib := &library.Service{Store: s, Artwork: art}
	a := New(Server{Store: s, Metadata: newDiscoverTMDB(t).provider(), Library: lib, ImageBase: srv.URL + "/v1/img", Version: "test"})
	b, err := rowtpl.ParseBundle([]byte(`{"version":"t","items":[{"id":"` + stored + `","k":"a","t":"Kept","ar":"A"},{"id":"` + cold + `","k":"a","t":"Cold","ar":"B"}],"keys":{},"names":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	a.musicPre.catalog.Store(b)
	ctx := context.Background()
	if name, _, ok := a.musicTrendArtFile(ctx, "album", stored, "500"); !ok || name != library.ArtworkName(srv.URL+"/v1/img/album/"+stored+"/500.webp") {
		t.Fatalf("stored album %q %v, want the server copy", name, ok)
	}
	if name, _, ok := a.musicTrendArtFile(ctx, "album", cold, "500"); ok {
		t.Fatalf("cold album %q, want none until the server stores it", name)
	}
	if _, ok := art.Path(library.ArtworkName(srv.URL + "/v1/img/album/" + cold + "/500.webp")); ok {
		t.Fatal("the redirect was kept as the server copy")
	}
	if _, _, ok := a.musicTrendArtFile(ctx, "album", stranger, "500"); ok {
		t.Fatal("an album neither listed nor in the catalog was fetched")
	}
}

// A rate limit (429) or server trouble says nothing about the picture: it
// is never remembered as missing (the next ask tries again), and a 429
// holds the preload back.
func TestTemporaryPictureErrorIsNotAMiss(t *testing.T) {
	t.Parallel()
	_, s := discoverRig(t, newDiscoverTMDB(t).provider())
	const busy = "44444444-4444-4444-8444-444444444444"
	limited := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if limited {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("\xff\xd8\xff stored"))
	}))
	defer srv.Close()
	art := &library.ArtworkCache{Dir: filepath.Join(t.TempDir(), "art"), Allowed: []string{srv.URL + "/"}, HTTP: srv.Client()}
	a := New(Server{Store: s, Metadata: newDiscoverTMDB(t).provider(), Library: &library.Service{Store: s, Artwork: art}, ImageBase: srv.URL + "/v1/img", Version: "test"})
	b, _ := rowtpl.ParseBundle([]byte(`{"version":"t","items":[{"id":"` + busy + `","k":"a","t":"Busy","ar":"B"}],"keys":{},"names":{}}`))
	a.musicPre.catalog.Store(b)
	ctx := context.Background()
	if _, _, ok := a.musicTrendArtFile(ctx, "album", busy, "500"); ok {
		t.Fatal("a 429 answered a picture")
	}
	a.music.mu.Lock()
	_, missed := a.music.artMiss["album|"+busy]
	a.music.mu.Unlock()
	if missed {
		t.Fatal("a 429 was remembered as a missing picture")
	}
	a.musicPre.mu.Lock()
	paused := time.Until(a.musicPre.pauseUntil) > 0
	a.musicPre.mu.Unlock()
	if !paused {
		t.Fatal("a 429 did not hold the preload back")
	}
	limited = false
	if _, _, ok := a.musicTrendArtFile(ctx, "album", busy, "500"); !ok {
		t.Fatal("the next ask did not try again")
	}
}
