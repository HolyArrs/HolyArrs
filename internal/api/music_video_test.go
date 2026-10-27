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

// ?id= plays that one of the artist's videos (a card in the Music videos
// row: label videos refuse YouTube's embedded player, so it goes through the
// relay), even with "Autoplay trailers" off since a click is no autoplay. An
// id the artist does not have is 404, and one that will not play stays in
// the row for the embed to try.
func TestArtistVideoPicked(t *testing.T) {
	t.Setenv("FAKE_YTDLP_KEY", "9bZkp7q19f0")
	fakeYTDLP(t, "plain")
	u := videoSetup(t, func(context.Context, string) ([]metadata.ArtistVideo, error) {
		return []metadata.ArtistVideo{{ID: "dQw4w9WgXcQ"}, {ID: "9bZkp7q19f0"}, {ID: "kJQP7kiw5Fk"}}, nil
	})
	base, _, _ := strings.Cut(u, "/api/v1/")
	do(t, "PUT", base+"/api/v1/settings/ui", `{"autoplay_trailers":false}`, 200)
	do(t, "GET", u, "", 404)
	var v ArtistVideoView
	_ = json.Unmarshal(do(t, "GET", u+"&id=9bZkp7q19f0", "", 200), &v)
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
