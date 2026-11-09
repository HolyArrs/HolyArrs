package metadata

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestMusicTrendingReadsList(t *testing.T) {
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/lists/music-trending" || r.Header.Get("Authorization") == "" {
			t.Error("bad request " + r.URL.String())
		}
		switch r.URL.RawQuery {
		case "kind=album&region=global":
			_, _ = w.Write([]byte(`{"schema_version":1,"kind":"album","region":"global","source":"listenbrainz","regions":["global","FR"],"items":[{"rank":1,"mbid":"21159e3f-172e-43f6-aa7d-8e06a81fea49","title":"ARIRANG","artist_name":"BTS","artist_mbid":"0d79fe8e-ba27-4859-bb8c-2f255f346853","listen_count":5}]}`))
		case "kind=artist&region=FR":
			_, _ = w.Write([]byte(`{}`))
		default:
			w.WriteHeader(404)
		}
	})
	l, err := s.MusicTrending(context.Background(), "album", "global")
	if err != nil || l.Source != "listenbrainz" || len(l.Regions) != 2 || len(l.Items) != 1 || l.Items[0].Title != "ARIRANG" || l.Items[0].ArtistMBID == "" {
		t.Fatalf("%+v %v", l, err)
	}
	if _, err := s.MusicTrending(context.Background(), "artist", "FR"); !errors.Is(err, ErrServiceInvalid) {
		t.Fatalf("empty answer: %v", err)
	}
	if _, err := s.MusicTrending(context.Background(), "artist", "ZZ"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing list: %v", err)
	}
}
