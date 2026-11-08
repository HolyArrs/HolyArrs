package music

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// Music pictures come only from our metadata server:
// a card's album the server stored is prefetched from it; one it has not
// stored yet (it redirects) is a miss, and no outside source is ever asked.
func TestMusicCardsPicturesFromServerOnly(t *testing.T) {
	t.Parallel()
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
	art := &library.ArtworkCache{Dir: filepath.Join(t.TempDir(), "art"), Allowed: []string{pics.URL + "/"}, HTTP: pics.Client()}
	api := New(Server{Store: s, Metadata: newDiscoverTMDB(t).provider(), Library: &library.Service{Store: s, Artwork: art}, ImageBase: pics.URL + "/v1/img", Version: "test"})
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
	if _, ok := art.Path(library.ArtworkName(pics.URL + "/v1/img/album/" + fresh + "/500.webp")); !ok {
		t.Fatal("stored album not prefetched from the server")
	}
	if c := code(fresh); c != 200 {
		t.Fatalf("stored album: %d", c)
	}
	if c := code(soundtrack); c != 204 {
		t.Fatalf("album the server has not stored: %d, want 204 (the card shows its placeholder)", c)
	}
	if n := outside.Load(); n != 0 {
		t.Fatalf("%d outside source asks", n)
	}
}

// A top song without a MusicBrainz release has no server copy: its chart
// cover link is never fetched, the picture route answers 404 (placeholder).
func TestTopSongArtNotFetchedFromChart(t *testing.T) {
	t.Parallel()
	_, s := discoverRig(t, newDiscoverTMDB(t).provider())
	var asked atomic.Int32
	pics := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("ÿØÿ synthetic"))
	}))
	defer pics.Close()
	art := &library.ArtworkCache{Dir: filepath.Join(t.TempDir(), "art"), Allowed: []string{pics.URL + "/"}, HTTP: pics.Client()}
	api := New(Server{Store: s, Library: &library.Service{Store: s, Artwork: art}, Version: "test"})
	srv := httptest.NewServer(api.Handler())
	defer srv.Close()
	id := songArtID("Bruno Mars", "Uptown Special", pics.URL+"/images/cover/abc/250x250-000000-80-0-0.jpg")
	api.rememberMusicNames([]nameKey{{kind: "album", mbid: id, name: "Uptown Special", artist: "Bruno Mars", cover: pics.URL + "/images/cover/abc/250x250-000000-80-0-0.jpg"}})
	resp, err := http.Get(srv.URL + artPath("album", id))
	if err != nil || resp.StatusCode != 204 || asked.Load() != 0 {
		t.Fatalf("song art %v %d, outside asks %d", err, resp.StatusCode, asked.Load())
	}
	resp.Body.Close()
}
