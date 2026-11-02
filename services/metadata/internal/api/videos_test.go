package api

import (
	"context"
	"strings"
	"testing"
)

type fakeVideos map[string]string

func (f fakeVideos) Get(_ context.Context, mbid string) ([]byte, bool) {
	b, ok := f[mbid]
	return []byte(b), ok
}

func TestVideosRoute(t *testing.T) {
	var wanted []string
	s, _, _ := setup(Options{Burst: 100, Videos: fakeVideos{queuedArtist: `{"videos":[{"youtube_id":"AAAAAAAAAAA"}]}`}, WantVideos: func(m string) { wanted = append(wanted, m) }})
	w := req(s, "/v1/music/videos/"+queuedArtist, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "AAAAAAAAAAA") || !strings.HasPrefix(w.Header().Get("Cache-Control"), "public") {
		t.Fatalf("stored %d %v", w.Code, w.Header())
	}
	other := "c1392450-e666-3926-a536-22c65f834433"
	w = req(s, "/v1/music/videos/"+other, "")
	if w.Code != 202 || w.Header().Get("Cache-Control") != "no-store" || len(wanted) != 1 || wanted[0] != other {
		t.Fatalf("missing %d %v %v", w.Code, w.Header(), wanted)
	}
	if w := req(s, "/v1/music/videos/nope", ""); w.Code != 400 {
		t.Fatalf("bad id %d", w.Code)
	}
}

func TestArtistAnswerCarriesVideos(t *testing.T) {
	s, _, _ := setup(Options{Burst: 100, Videos: fakeVideos{queuedArtist: `{"videos":[{"youtube_id":"AAAAAAAAAAA","most_replayed":63.5,"duration":212}]}`}})
	w := req(s, "/v1/music/musicbrainz/artist/"+queuedArtist, "synthetic-test-token")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"videos":[{"youtube_id":"AAAAAAAAAAA","most_replayed":63.5,"duration":212}]`) {
		t.Fatalf("artist answer %d %s", w.Code, w.Body)
	}
}
