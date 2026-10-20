package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/CoderLuii/holyarrs/internal/library"
	"github.com/CoderLuii/holyarrs/internal/metadata"
)

const trendGroup = "21159e3f-172e-43f6-aa7d-8e06a81fea49"

func TestMusicTrendingListsAndPictures(t *testing.T) {
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
		if r.URL.Path != "/release-group/"+trendGroup+"/front-250" {
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("\xff\xd8\xff synthetic"))
	}))
	defer cover.Close()
	art := &library.ArtworkCache{Dir: filepath.Join(t.TempDir(), "art"), Allowed: []string{cover.URL + "/release-group/"}, HTTP: cover.Client()}
	srv := httptest.NewServer(New(Server{Store: s, Metadata: newDiscoverTMDB(t).provider(), MusicTrending: trend, Library: &library.Service{Store: s, Artwork: art, CoverArtBase: cover.URL}, Version: "test"}).Handler())
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
	get("/api/v1/music-trending/art/album/"+trendGroup, 404)
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
	resp, err := http.Get(srv.URL + "/api/v1/music-trending/art/album/" + trendGroup)
	if err != nil || resp.StatusCode != 200 || resp.Header.Get("Cache-Control") != "private, max-age=31536000, immutable" {
		t.Fatalf("cover %v %d", err, resp.StatusCode)
	}
	resp.Body.Close()
	get("/api/v1/music-trending/art/artist/0d79fe8e-ba27-4859-bb8c-2f255f346853", 404) // no artist picture source here
	get("/api/v1/music-trending?kind=album&region=ZZ", 404)
	get("/api/v1/music-trending?kind=track", 400)
	get("/api/v1/music-trending?kind=artist&region=1A", 400)
	srv2 := httptest.NewServer(New(Server{Store: s, Version: "test"}).Handler())
	defer srv2.Close()
	if resp, _ := http.Get(srv2.URL + "/api/v1/music-trending?kind=artist"); resp.StatusCode != 404 {
		t.Fatalf("without a server %d", resp.StatusCode)
	}
}

func TestMusicTrendArtUsesCoverChain(t *testing.T) {
	_, s := discoverRig(t, newDiscoverTMDB(t).provider())
	const none, down = "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"
	trend := func(_ context.Context, kind, region string) (metadata.MusicTrend, error) {
		return metadata.MusicTrend{Kind: kind, Region: region, Items: []metadata.MusicTrendItem{
			{Rank: 1, MBID: trendGroup, Title: "ARIRANG", Artist: "BTS", ArtistMBID: "0d79fe8e-ba27-4859-bb8c-2f255f346853"},
			{Rank: 2, MBID: none, Title: "Nothing", Artist: "BTS", ArtistMBID: "0d79fe8e-ba27-4859-bb8c-2f255f346853"},
			{Rank: 3, MBID: down, Title: "Down", Artist: "BTS", ArtistMBID: "0d79fe8e-ba27-4859-bb8c-2f255f346853"},
		}}, nil
	}
	pics := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/apple/600x600bb.jpg" && r.URL.Path != "/release-group/"+down+"/front-250" {
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("\xff\xd8\xff synthetic"))
	}))
	defer pics.Close()
	var asked atomic.Int32
	cover := func(_ context.Context, mbid, artist, title string) (string, error) {
		asked.Add(1)
		switch mbid {
		case trendGroup:
			if artist != "BTS" || title != "ARIRANG" {
				t.Errorf("asked %q %q", artist, title)
			}
			return pics.URL + "/apple/600x600bb.jpg", nil
		case down:
			return "", metadata.ErrServiceUnavailable
		}
		return "", metadata.ErrNotFound
	}
	art := &library.ArtworkCache{Dir: filepath.Join(t.TempDir(), "art"), Allowed: []string{pics.URL + "/"}, HTTP: pics.Client()}
	srv := httptest.NewServer(New(Server{Store: s, Metadata: newDiscoverTMDB(t).provider(), MusicTrending: trend, MusicCover: cover, Library: &library.Service{Store: s, Artwork: art, CoverArtBase: pics.URL}, Version: "test"}).Handler())
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
	if c := code(trendGroup); c != 200 {
		t.Fatalf("chain cover %d", c)
	}
	if c := code(none); c != 404 {
		t.Fatalf("no cover anywhere %d", c)
	}
	if c := code(down); c != 200 {
		t.Fatalf("server down falls back to the archive %d", c)
	}
	n := asked.Load()
	code(trendGroup)
	code(none)
	if asked.Load() != n {
		t.Fatal("covers not kept")
	}
}
