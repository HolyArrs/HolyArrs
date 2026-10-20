package metadata

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestMusicSimilarReadsArtists(t *testing.T) {
	const ref = "a74b1b7f-71a5-4011-9441-d0b5e4122711"
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			t.Error("no credential")
		}
		switch r.URL.Path {
		case "/v1/music/similar/" + ref:
			_, _ = w.Write([]byte(`{"schema_version":1,"mbid":"` + ref + `","source":"listenbrainz","artists":[{"mbid":"5B11F4CE-A62D-471E-81FC-A69A8278C7DA","name":"Nirvana","score":11},{"mbid":"bad","name":"x"}]}`))
		case "/v1/music/similar/11111111-1111-4111-8111-111111111111":
			_, _ = w.Write([]byte(`{}`))
		default:
			w.WriteHeader(404)
		}
	})
	got, err := s.MusicSimilar(context.Background(), ref)
	if err != nil || len(got) != 1 || got[0].Name != "Nirvana" || got[0].MBID != "5b11f4ce-a62d-471e-81fc-a69a8278c7da" {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err := s.MusicSimilar(context.Background(), "11111111-1111-4111-8111-111111111111"); !errors.Is(err, ErrServiceInvalid) {
		t.Fatalf("odd answer %v", err)
	}
	if _, err := s.MusicSimilar(context.Background(), "22222222-2222-4222-8222-222222222222"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown %v", err)
	}
	if _, err := s.MusicSimilar(context.Background(), "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bad id %v", err)
	}
}
