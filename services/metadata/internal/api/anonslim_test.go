package api

import (
	"context"
	"testing"

	"github.com/CoderLuii/holyarrs-metadata/internal/discog"
)

// Security 31: picture, video and album asks without a credential for ids
// the slim copy lacks answer 404 and start no work; a credentialed ask
// still does.
func TestAnonymousSlimMissStartsNoMusicWork(t *testing.T) {
	missing := func(context.Context, string, string) bool { return true }
	var nows, wants, vids []string
	q := &keyQueue{}
	s, _, _ := setup(Options{Burst: 100, SlimMissing: missing,
		Images: &Images{Store: fakeImages{}, Want: func(k, m string) { wants = append(wants, k) },
			Now: func(_ context.Context, k, m string) bool { nows = append(nows, k); return false }},
		Videos: fakeVideos{}, WantVideos: func(m string) { vids = append(vids, m) },
		Discog: &fakeDiscog{albums: map[string]discog.Album{}}})
	s.SetQueue(q)
	for _, p := range []string{"/v1/img/artist/" + queuedArtist + "/500.webp", "/v1/img/album/" + queuedArtist + "/500.webp", "/v1/music/videos/" + queuedArtist, "/v1/music/album/" + discAlbum} {
		if w := req(s, p, ""); w.Code != 404 {
			t.Fatalf("%s anonymous %d", p, w.Code)
		}
	}
	if len(nows)+len(wants)+len(vids)+len(q.keys) != 0 {
		t.Fatalf("work started: %v %v %v %v", nows, wants, vids, q.keys)
	}
	req(s, "/v1/music/videos/"+queuedArtist, "synthetic-test-token")
	if len(vids) != 1 {
		t.Fatalf("credentialed ask not queued %v", vids)
	}
}
