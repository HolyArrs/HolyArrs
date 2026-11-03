package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs/internal/jobs"
	"github.com/CoderLuii/holyarrs/internal/library"
	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/store"
)

func videoSetup(t *testing.T, vids func(context.Context, string) ([]metadata.ArtistVideo, error)) string {
	t.Helper()
	s, err := store.Open(t.TempDir(), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	q := jobs.New(s, jobs.Options{})
	lib := &library.Service{Store: s, Queue: q, Artists: apiMusic()}
	srv := httptest.NewServer(New(Server{Store: s, Queue: q, Library: lib, Metadata: &metadata.Static{}, ArtistVideos: vids}).Handler())
	t.Cleanup(srv.Close)
	do(t, "POST", srv.URL+"/api/v1/library-folders", fmt.Sprintf(`{"path":%q,"kind":"music","label":"Music"}`, t.TempDir()), 201)
	return srv.URL + "/api/v1/music/artists/" + apiArtist + "?part=video"
}

// route: GET /api/v1/music/artists/{mbid}?part=video
func TestArtistVideoStartPoints(t *testing.T) {
	// The server's most replayed point wins.
	fakeReader(t, "heat", "dQw4w9WgXcQ")
	u := videoSetup(t, func(context.Context, string) ([]metadata.ArtistVideo, error) {
		return []metadata.ArtistVideo{{ID: "dQw4w9WgXcQ", MostReplayed: 42}}, nil
	})
	var v ArtistVideoView
	_ = json.Unmarshal(videoReady(t, u), &v)
	if v.Key != "dQw4w9WgXcQ" || v.Start != 42 || !strings.Contains(v.ProxyURL, "/api/v1/trailer/stream/") || !strings.HasSuffix(v.ProxyURL, "/manifest.m3u8") {
		t.Fatalf("video %+v", v)
	}
	// Else the "most replayed" peak from the reader (its opening skipped).
	u = videoSetup(t, func(context.Context, string) ([]metadata.ArtistVideo, error) {
		return []metadata.ArtistVideo{{ID: "dQw4w9WgXcQ"}}, nil
	})
	_ = json.Unmarshal(videoReady(t, u), &v)
	if v.Start != 80 || v.Duration != 200 {
		t.Fatalf("heatmap start %+v", v)
	}
}

// ?id= plays that one of the artist's videos (a card in the Music videos
// row: label videos refuse YouTube's embedded player, so it goes through the
// relay), even with "Autoplay trailers" off since a click is no autoplay. An
// id the artist does not have is 404, and one that will not play stays in
// the row for the embed to try.
func TestArtistVideoPicked(t *testing.T) {
	fakeReader(t, "plain", "9bZkp7q19f0")
	u := videoSetup(t, func(context.Context, string) ([]metadata.ArtistVideo, error) {
		return []metadata.ArtistVideo{{ID: "dQw4w9WgXcQ"}, {ID: "9bZkp7q19f0"}, {ID: "kJQP7kiw5Fk"}}, nil
	})
	base, _, _ := strings.Cut(u, "/api/v1/")
	do(t, "PUT", base+"/api/v1/settings/ui", `{"autoplay_trailers":false}`, 200)
	do(t, "GET", u, "", 404)
	var v ArtistVideoView
	_ = json.Unmarshal(videoReady(t, u+"&id=9bZkp7q19f0"), &v)
	if v.Key != "9bZkp7q19f0" || !strings.Contains(v.ProxyURL, "/api/v1/trailer/stream/") || !strings.HasSuffix(v.ProxyURL, "/manifest.m3u8") {
		t.Fatalf("picked %+v", v)
	}
	do(t, "GET", u+"&id=aaaaaaaaaaa", "", 404)
	do(t, "GET", u+"&id=kJQP7kiw5Fk", "", 404)
	if b := string(do(t, "GET", strings.Replace(u, "part=video", "part=videos", 1), "", 200)); !strings.Contains(b, "kJQP7kiw5Fk") {
		t.Fatalf("a video that would not play left the row: %s", b)
	}
}

func TestArtistVideoFallbacks(t *testing.T) {
	fakeReader(t, "plain", "dQw4w9WgXcQ")
	u := videoSetup(t, func(context.Context, string) ([]metadata.ArtistVideo, error) {
		return []metadata.ArtistVideo{{ID: "dQw4w9WgXcQ"}}, nil
	})
	var v ArtistVideoView
	_ = json.Unmarshal(videoReady(t, u), &v)
	if v.Start != 60 {
		t.Fatalf("no heatmap: start at 30%% of 200 s, got %+v", v)
	}
	// No videos (an older server): 404, the hero keeps its photo.
	do(t, "GET", videoSetup(t, func(context.Context, string) ([]metadata.ArtistVideo, error) { return nil, nil }), "", 404)
	do(t, "GET", videoSetup(t, nil), "", 404)
}

func TestVideoStart(t *testing.T) {
	if got := videoStart(metadata.ArtistVideo{}, TrailerStream{}); got != 0 {
		t.Fatalf("nothing known: %v", got)
	}
	if got := videoStart(metadata.ArtistVideo{Duration: 100}, TrailerStream{Peak: 150}); got != 30 {
		t.Fatalf("a peak past the end is ignored: %v", got)
	}
}

// videoReady asks u until it answers 200: a stream is looked up behind the
// first answer (404 at once), never while the request waits.
func videoReady(t *testing.T, u string) []byte {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		resp, err := http.Get(u)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == 200 {
			return b
		}
		if resp.StatusCode != 404 || time.Now().After(deadline) {
			t.Fatalf("GET %s: %d %s", u, resp.StatusCode, b)
		}
		time.Sleep(30 * time.Millisecond)
	}
}

// The first ask for an artist whose stream is not looked up answers 404 at
// once (no lookup in the request) and the lookup runs behind it.
func TestArtistVideoNeverWaitsOnLookup(t *testing.T) {
	yt := fakeReader(t, "plain", "dQw4w9WgXcQ")
	u := videoSetup(t, func(context.Context, string) ([]metadata.ArtistVideo, error) {
		return []metadata.ArtistVideo{{ID: "dQw4w9WgXcQ"}}, nil
	})
	start := time.Now()
	do(t, "GET", u, "", 404)
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Fatalf("first ask waited %v", d)
	}
	videoReady(t, u)
	before := yt.runs.Load()
	do(t, "GET", u, "", 200)
	if yt.runs.Load() != before {
		t.Fatalf("a ready stream was looked up again: %d -> %d", before, yt.runs.Load())
	}
}
