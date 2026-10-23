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

// Explore's personal rows (New from your artists, Finish the set) point at
// the picture route; their albums must be known to it, with the artist, so
// the cover chain's name searches run, and their art is prefetched.
func TestMusicCardsUseCoverChain(t *testing.T) {
	_, s := discoverRig(t, newDiscoverTMDB(t).provider())
	const fresh, soundtrack = "54be6365-1bc0-4825-b87e-505a2b48fade", "192a95dc-d993-4768-9f77-4496b6460400"
	pics := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/dz/") {
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("\xff\xd8\xff synthetic " + r.URL.Path))
	}))
	defer pics.Close()
	var withNames, bare atomic.Int32
	cover := func(_ context.Context, mbid, artist, title string) (string, error) {
		if artist == "" || title == "" {
			bare.Add(1)
			return "", metadata.ErrNotFound // only the Cover Art Archive, which has none
		}
		withNames.Add(1)
		return pics.URL + "/dz/" + mbid + ".jpg", nil
	}
	art := &library.ArtworkCache{Dir: filepath.Join(t.TempDir(), "art"), Allowed: []string{pics.URL + "/"}, HTTP: pics.Client()}
	api := New(Server{Store: s, Metadata: newDiscoverTMDB(t).provider(), Library: &library.Service{Store: s, Artwork: art, CoverArtBase: pics.URL}, Version: "test", Deps: music.Deps{MusicCover: cover}})
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

	// A soundtrack row names the album first, without its artist: no
	// answer is kept, and the artist given later is used.
	api.rememberMusicNames([]nameKey{{kind: "album", mbid: soundtrack, name: "Man’s Best Friend"}})
	if c := code(soundtrack); c != 404 {
		t.Fatalf("no names, no cover: %d", c)
	}
	api.music.mu.Lock()
	delete(api.music.artMiss, "album|"+soundtrack)
	api.music.mu.Unlock()

	rows := []MusicRow{{ID: "new_from", Items: []MusicCard{
		{Kind: "album", MBID: fresh, Name: "No Bad Memories", Artist: "The Avalanches", ArtistMBID: "0d79fe8e-ba27-4859-bb8c-2f255f346853", Image: musicArtPrefix + "album/" + fresh},
	}}, {ID: "finish_set", Items: []MusicCard{
		{Kind: "album", MBID: soundtrack, Name: "Man’s Best Friend", Artist: "Sabrina Carpenter", Image: musicArtPrefix + "album/" + soundtrack},
	}}}
	api.rememberMusicCards(rows)
	api.music.prefetching.Wait()
	for _, id := range []string{fresh, soundtrack} {
		if _, ok := art.Path(library.ArtworkName(pics.URL + "/dz/" + id + ".jpg")); !ok {
			t.Fatalf("%s not prefetched", id)
		}
		if c := code(id); c != 200 {
			t.Fatalf("%s: %d", id, c)
		}
	}
	if api.music.names["album|"+soundtrack].Artist != "Sabrina Carpenter" {
		t.Fatalf("artist not filled in: %+v", api.music.names["album|"+soundtrack])
	}
	// Shown again within minutes: not queued again.
	n := withNames.Load()
	api.rememberMusicCards(rows)
	api.music.prefetching.Wait()
	if withNames.Load() != n {
		t.Fatal("prefetched twice")
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

// A top song whose release is not in the discography gets its picture
// through the picture route, from its own cover link.
func TestTopSongArtThroughRoute(t *testing.T) {
	_, s := discoverRig(t, newDiscoverTMDB(t).provider())
	pics := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("\xff\xd8\xff synthetic"))
	}))
	defer pics.Close()
	art := &library.ArtworkCache{Dir: filepath.Join(t.TempDir(), "art"), Allowed: []string{pics.URL + "/"}, HTTP: pics.Client()}
	api := New(Server{Store: s, Library: &library.Service{Store: s, Artwork: art, CoverArtBase: pics.URL}, Version: "test"})
	srv := httptest.NewServer(api.Handler())
	defer srv.Close()
	id := songArtID("Bruno Mars", "Uptown Special", pics.URL+"/images/cover/abc/250x250-000000-80-0-0.jpg")
	if !strings.HasPrefix(id, "song-") || metadata.ValidMBID(id) {
		t.Fatalf("id %q", id)
	}
	api.rememberMusicNames([]nameKey{{kind: "album", mbid: id, name: "Uptown Special", artist: "Bruno Mars", cover: pics.URL + "/images/cover/abc/250x250-000000-80-0-0.jpg"}})
	resp, err := http.Get(srv.URL + artPath("album", id))
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("song art %v %d", err, resp.StatusCode)
	}
	resp.Body.Close()
}
