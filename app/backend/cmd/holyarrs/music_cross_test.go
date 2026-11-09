package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// trendGroup is the album the kid trending test lists.
const trendGroup = "21159e3f-172e-43f6-aa7d-8e06a81fea49"

// Held, retrying or slow details answer 503 with Retry-After, not 502.
func TestDiscoverFailBusy(t *testing.T) {
	for _, err := range []error{outbound.ErrSuppressed, metadata.ErrServiceUnavailable, metadata.ErrRateLimited, context.DeadlineExceeded} {
		w := httptest.NewRecorder()
		discover.Fail(w, err)
		if w.Code != http.StatusServiceUnavailable || w.Header().Get("Retry-After") == "" {
			t.Fatalf("%v: %d %q", err, w.Code, w.Header().Get("Retry-After"))
		}
		w = httptest.NewRecorder()
		discover.PeopleError(w, err)
		if w.Code != http.StatusServiceUnavailable || w.Header().Get("Retry-After") == "" {
			t.Fatalf("people %v: %d", err, w.Code)
		}
	}
	if !discover.StaleOK(metadata.ErrServiceUnavailable) || discover.StaleOK(metadata.ErrNotFound) || discover.StaleOK(errors.Join(metadata.ErrNoAccess)) {
		t.Fatal("staleOK")
	}
}

// A Kid gets no music trending list (it can hold explicit releases) and no
// picture from one, like /explore/music.
func TestMusicTrendingKidLimited(t *testing.T) {
	t.Parallel()
	k := newKidRig(t)
	k.api.s.MusicTrending = func(_ context.Context, kind, region string) (metadata.MusicTrend, error) {
		return metadata.MusicTrend{Kind: kind, Region: region, Source: "listenbrainz", Items: []metadata.MusicTrendItem{{Rank: 1, MBID: trendGroup, Title: "Explicit Record", Artist: "Loud"}}}, nil
	}
	cover := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("\xff\xd8\xff synthetic"))
	}))
	defer cover.Close()
	k.api.s.Library.Artwork = &library.ArtworkCache{Dir: filepath.Join(t.TempDir(), "art"), Allowed: []string{cover.URL + "/"}, HTTP: cover.Client()}
	k.api.s.ImageBase = cover.URL + "/v1/img"
	items := func(b []byte) int {
		var out struct {
			Items []any `json:"items"`
		}
		_ = json.Unmarshal(b, &out)
		return len(out.Items)
	}
	if n := items(k.ana.call("GET", "/api/v1/music-trending?kind=album", "", 200)); n != 1 {
		t.Fatalf("member sees %d items", n)
	}
	if n := items(k.kai.call("GET", "/api/v1/music-trending?kind=album", "", 200)); n != 0 {
		t.Fatalf("Kid sees %d items", n)
	}
	k.kai.call("GET", "/api/v1/music-trending/art/album/"+trendGroup, "", 204)
	// Stored ahead (the warm does it for every listed id); the route serves disk only.
	k.api.musicTrendArtFile(context.Background(), "album", trendGroup, "500")
	k.ana.call("GET", "/api/v1/music-trending/art/album/"+trendGroup, "", 200)
}

// TestRefreshAllQueuesArtists: Refresh all queues the artist refresh too.
func TestRefreshAllQueuesArtists(t *testing.T) {
	t.Parallel()
	srv, s, _ := setup(t)
	var res struct {
		JobIDs []int64 `json:"job_ids"`
	}
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/titles/refresh", "", 202), &res)
	if len(res.JobIDs) != 3 {
		t.Fatalf("jobs %v", res.JobIDs)
	}
	var n int
	_ = s.Read().QueryRow("SELECT count(*) FROM jobs WHERE kind = 'refresh_artists'").Scan(&n)
	if n != 1 {
		t.Fatalf("refresh_artists jobs %d", n)
	}
}
