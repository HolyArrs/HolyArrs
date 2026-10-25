package api

import (
	"context"
	"github.com/CoderLuii/holyarrs/internal/music"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs/internal/library"
	"github.com/CoderLuii/holyarrs/internal/metadata"
)

// Music pictures come only from our metadata server (Luis, 2026-10-24):
// a card's album the server stored is prefetched from it; one it has not
// stored yet (it redirects) is a miss, and no outside source is ever asked.
func TestMusicCardsPicturesFromServerOnly(t *testing.T) {
	_, s := discoverRig(t, newDiscoverTMDB(t).provider())
	const fresh, soundtrack = "54be6365-1bc0-4825-b87e-505a2b48fade", "192a95dc-d993-4768-9f77-4496b6460400"
	var outside atomic.Int32
	pics := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v1/img/album/"+fresh+"/") {
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("ÿØÿ stored " + r.URL.Path))
			return
		}
		if strings.HasPrefix(r.URL.Path, "/v1/img/") {
			http.Redirect(w, r, "/release-group/x/front-250", http.StatusFound)
			return
		}
		outside.Add(1)
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("ÿØÿ outside"))
	}))
	defer pics.Close()
	cover := func(_ context.Context, mbid, artist, title string) (string, error) {
		outside.Add(1)
		return pics.URL + "/dz/" + mbid + ".jpg", nil
	}
	art := &library.ArtworkCache{Dir: filepath.Join(t.TempDir(), "art"), Allowed: []string{pics.URL + "/"}, HTTP: pics.Client()}
	api := New(Server{Store: s, Metadata: newDiscoverTMDB(t).provider(), Library: &library.Service{Store: s, Artwork: art, CoverArtBase: pics.URL}, ImageBase: pics.URL + "/v1/img", Version: "test", Deps: music.Deps{MusicCover: cover}})
	srv := httptest.NewServer(api.Handler())
	defer srv.Close()
	code := func(id string) int {
		resp, err := http.Get(srv.URL + "/api/v1/music-trending/art/album/" + id)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	rows := []MusicRow{{ID: "new_from", Items: []MusicCard{
		{Kind: "album", MBID: fresh, Name: "No Bad Memories", Artist: "The Avalanches", Image: musicArtPrefix + "album/" + fresh},
		{Kind: "album", MBID: soundtrack, Name: "Man’s Best Friend", Artist: "Sabrina Carpenter", Image: musicArtPrefix + "album/" + soundtrack},
	}}}
	api.rememberMusicCards(rows)
	api.music.prefetching.Wait()
	if _, ok := art.Path(library.ArtworkName(pics.URL + "/v1/img/album/" + fresh + "/250.jpg")); !ok {
		t.Fatal("stored album not prefetched from the server")
	}
	if c := code(fresh); c != 200 {
		t.Fatalf("stored album: %d", c)
	}
	if c := code(soundtrack); c != 404 {
		t.Fatalf("album the server has not stored: %d, want 404 (the card shows its placeholder)", c)
	}
	if n := outside.Load(); n != 0 {
		t.Fatalf("%d outside source asks", n)
	}
}

// No source having a cover is remembered for minutes, not days.
func TestMusicCoverMissIsShort(t *testing.T) {
	if musicArtMissTTL > 15*time.Minute {
		t.Fatalf("miss kept %s", musicArtMissTTL)
	}
	_, s := discoverRig(t, newDiscoverTMDB(t).provider())
	var asked atomic.Int32
	api := New(Server{Store: s, Version: "test", Deps: music.Deps{MusicCover: func(context.Context, string, string, string) (string, error) {
		asked.Add(1)
		return "", metadata.ErrNotFound
	}}})
	it := metadata.MusicTrendItem{Title: "New", Artist: "Someone"}
	api.albumCover(context.Background(), trendGroup, it)
	api.albumCover(context.Background(), trendGroup, it)
	if asked.Load() != 1 {
		t.Fatalf("miss not kept briefly: %d", asked.Load())
	}
	api.music.mu.Lock()
	e := api.music.covers[trendGroup]
	e.at = time.Now().Add(-musicArtMissTTL - time.Second)
	api.music.covers[trendGroup] = e
	api.music.mu.Unlock()
	api.albumCover(context.Background(), trendGroup, it)
	if asked.Load() != 2 {
		t.Fatal("miss kept too long")
	}
}

// A top song without a MusicBrainz release has no server copy: its chart
// cover link is never fetched, the picture route answers 404 (placeholder).
func TestTopSongArtNotFetchedFromChart(t *testing.T) {
	_, s := discoverRig(t, newDiscoverTMDB(t).provider())
	var asked atomic.Int32
	pics := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("ÿØÿ synthetic"))
	}))
	defer pics.Close()
	art := &library.ArtworkCache{Dir: filepath.Join(t.TempDir(), "art"), Allowed: []string{pics.URL + "/"}, HTTP: pics.Client()}
	api := New(Server{Store: s, Library: &library.Service{Store: s, Artwork: art, CoverArtBase: pics.URL}, Version: "test"})
	srv := httptest.NewServer(api.Handler())
	defer srv.Close()
	id := songArtID("Bruno Mars", "Uptown Special", pics.URL+"/images/cover/abc/250x250-000000-80-0-0.jpg")
	api.rememberMusicNames([]nameKey{{kind: "album", mbid: id, name: "Uptown Special", artist: "Bruno Mars", cover: pics.URL + "/images/cover/abc/250x250-000000-80-0-0.jpg"}})
	resp, err := http.Get(srv.URL + artPath("album", id))
	if err != nil || resp.StatusCode != 404 || asked.Load() != 0 {
		t.Fatalf("song art %v %d, outside asks %d", err, resp.StatusCode, asked.Load())
	}
	resp.Body.Close()
}
