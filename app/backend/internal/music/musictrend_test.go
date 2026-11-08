package music

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

const trendGroup = "21159e3f-172e-43f6-aa7d-8e06a81fea49"

func TestMusicTrendingListsAndPictures(t *testing.T) {
	t.Parallel()
	_, s := discoverRig(t, newDiscoverTMDB(t).provider())
	var calls atomic.Int32
	trend := func(_ context.Context, kind, region string) (metadata.MusicTrend, error) {
		calls.Add(1)
		if region == "ZZ" {
			return metadata.MusicTrend{}, metadata.ErrNotFound
		}
		if kind == "album" {
			return metadata.MusicTrend{Kind: kind, Region: region, Source: "listenbrainz", Regions: []string{"global", "FR"}, Items: []metadata.MusicTrendItem{
				{Rank: 1, MBID: trendGroup, Title: "ARIRANG", Artist: "BTS", ArtistMBID: "0d79fe8e-ba27-4859-bb8c-2f255f346853"},
				{Rank: 2, Title: "No id"},
			}}, nil
		}
		return metadata.MusicTrend{Kind: kind, Region: region, Source: "deezer", Items: []metadata.MusicTrendItem{{Rank: 1, MBID: "0d79fe8e-ba27-4859-bb8c-2f255f346853", Name: "BTS"}}}, nil
	}
	cover := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/img/album/"+trendGroup+"/500.webp" {
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("\xff\xd8\xff synthetic"))
	}))
	defer cover.Close()
	art := &library.ArtworkCache{Dir: filepath.Join(t.TempDir(), "art"), Allowed: []string{cover.URL + "/"}, HTTP: cover.Client()}
	srv := httptest.NewServer(New(Server{Store: s, Metadata: newDiscoverTMDB(t).provider(), Library: &library.Service{Store: s, Artwork: art}, ImageBase: cover.URL + "/v1/img", Version: "test", Deps: Deps{MusicTrending: trend}}).Handler())
	defer srv.Close()
	get := func(q string, want int) map[string]any {
		t.Helper()
		resp, err := http.Get(srv.URL + q)
		if err != nil || resp.StatusCode != want {
			t.Fatalf("%s: %v %d", q, err, resp.StatusCode)
		}
		defer resp.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return out
	}
	// A picture is only served for an id on a list read earlier.
	get("/api/v1/music-trending/art/album/"+trendGroup, 204)
	a := get("/api/v1/music-trending?kind=album", 200)
	items := a["items"].([]any)
	first := items[0].(map[string]any)
	if a["source"] != "listenbrainz" || a["region"] != "global" || len(items) != 1 || first["name"] != "ARIRANG" || first["artist_name"] != "BTS" || first["mbid"] != trendGroup {
		t.Fatalf("answer %+v", a)
	}
	get("/api/v1/music-trending?kind=album&region=GLOBAL", 200)
	if calls.Load() != 1 {
		t.Fatal("list not cached")
	}
	if b := get("/api/v1/music-trending?kind=artist&region=fr", 200); b["items"].([]any)[0].(map[string]any)["name"] != "BTS" {
		t.Fatalf("artists %+v", b)
	}
	// The route answers from disk: the cover is stored behind the list
	// (or behind a first 204) and then served.
	var resp *http.Response
	var err error
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		resp, err = http.Get(srv.URL + "/api/v1/music-trending/art/album/" + trendGroup)
		if err != nil || resp.StatusCode != 204 || time.Now().After(deadline) {
			break
		}
		resp.Body.Close()
	}
	if err != nil || resp.StatusCode != 200 || resp.Header.Get("Cache-Control") != "private, max-age=86400" {
		t.Fatalf("cover %v %d", err, resp.StatusCode)
	}
	resp.Body.Close()
	get("/api/v1/music-trending/art/artist/0d79fe8e-ba27-4859-bb8c-2f255f346853", 204) // no artist picture source here
	get("/api/v1/music-trending?kind=album&region=ZZ", 404)
	get("/api/v1/music-trending?kind=track", 400)
	get("/api/v1/music-trending?kind=artist&region=1A", 400)
	srv2 := httptest.NewServer(New(Server{Store: s, Version: "test"}).Handler())
	defer srv2.Close()
	if resp, _ := http.Get(srv2.URL + "/api/v1/music-trending?kind=artist"); resp.StatusCode != 404 {
		t.Fatalf("without a server %d", resp.StatusCode)
	}
}

// A trending list's covers come only from the server's stored copies: an
// album it has not stored is a miss.
func TestMusicTrendArtServerOnly(t *testing.T) {
	t.Parallel()
	_, s := discoverRig(t, newDiscoverTMDB(t).provider())
	const none = "11111111-1111-4111-8111-111111111111"
	trend := func(_ context.Context, kind, region string) (metadata.MusicTrend, error) {
		return metadata.MusicTrend{Kind: kind, Region: region, Items: []metadata.MusicTrendItem{
			{Rank: 1, MBID: trendGroup, Title: "ARIRANG", Artist: "BTS", Cover: "https://elsewhere.test/a.jpg"},
			{Rank: 2, MBID: none, Title: "Nothing", Artist: "BTS"},
		}}, nil
	}
	pics := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v1/img/album/"+trendGroup+"/") {
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("\xff\xd8\xff synthetic"))
			return
		}
		http.Redirect(w, r, "/release-group/x/front-250", http.StatusFound)
	}))
	defer pics.Close()
	art := &library.ArtworkCache{Dir: filepath.Join(t.TempDir(), "art"), Allowed: []string{pics.URL + "/"}, HTTP: pics.Client()}
	api := New(Server{Store: s, Metadata: newDiscoverTMDB(t).provider(), Library: &library.Service{Store: s, Artwork: art}, ImageBase: pics.URL + "/v1/img", Version: "test", Deps: Deps{MusicTrending: trend}})
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
	if resp, err := http.Get(srv.URL + "/api/v1/music-trending?kind=album"); err != nil || resp.StatusCode != 200 {
		t.Fatalf("list %v", err)
	}
	api.music.prefetching.Wait()
	if _, ok := art.Path(library.ArtworkName(pics.URL + "/v1/img/album/" + trendGroup + "/500.webp")); !ok {
		t.Fatal("stored cover not prefetched")
	}
	if c := code(trendGroup); c != 200 {
		t.Fatalf("stored cover %d", c)
	}
	if c := code(none); c != 204 {
		t.Fatalf("not stored %d", c)
	}
}
