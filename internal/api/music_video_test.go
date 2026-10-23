package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

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
	t.Setenv("FAKE_YTDLP_KEY", "dQw4w9WgXcQ")
	// The server's most replayed point wins.
	fakeYTDLP(t, "heat")
	u := videoSetup(t, func(context.Context, string) ([]metadata.ArtistVideo, error) {
		return []metadata.ArtistVideo{{ID: "dQw4w9WgXcQ", MostReplayed: 42}}, nil
	})
	var v ArtistVideoView
	_ = json.Unmarshal(do(t, "GET", u, "", 200), &v)
	if v.Key != "dQw4w9WgXcQ" || v.Start != 42 || !strings.Contains(v.ProxyURL, "/api/v1/trailer/stream/") || !strings.HasSuffix(v.ProxyURL, "/manifest.m3u8") {
		t.Fatalf("video %+v", v)
	}
	// Else the heatmap peak through yt-dlp.
	u = videoSetup(t, func(context.Context, string) ([]metadata.ArtistVideo, error) {
		return []metadata.ArtistVideo{{ID: "dQw4w9WgXcQ"}}, nil
	})
	_ = json.Unmarshal(do(t, "GET", u, "", 200), &v)
	if v.Start != 80 || v.Duration != 200 {
		t.Fatalf("heatmap start %+v", v)
	}
}

func TestArtistVideoFallbacks(t *testing.T) {
	t.Setenv("FAKE_YTDLP_KEY", "dQw4w9WgXcQ")
	fakeYTDLP(t, "plain")
	u := videoSetup(t, func(context.Context, string) ([]metadata.ArtistVideo, error) {
		return []metadata.ArtistVideo{{ID: "dQw4w9WgXcQ"}}, nil
	})
	var v ArtistVideoView
	_ = json.Unmarshal(do(t, "GET", u, "", 200), &v)
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
