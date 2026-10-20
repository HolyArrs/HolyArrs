package metadata

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestMusicCoverReadsURL(t *testing.T) {
	const ref = "a74b1b7f-71a5-4011-9441-d0b5e4122711"
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			t.Error("no credential")
		}
		switch r.URL.Path {
		case "/v1/music/cover/" + ref:
			if r.URL.Query().Get("artist") != "Taylor Swift" || r.URL.Query().Get("title") != "The Encore" {
				t.Errorf("query %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"schema_version":1,"source":"itunes","url":"https://is1-ssl.mzstatic.com/x/600x600bb.jpg"}`))
		case "/v1/music/cover/11111111-1111-4111-8111-111111111111":
			_, _ = w.Write([]byte(`{"url":"http://plain/x.jpg"}`))
		default:
			w.WriteHeader(404)
		}
	})
	got, err := s.MusicCover(context.Background(), ref, "Taylor Swift", "The Encore")
	if err != nil || got != "https://is1-ssl.mzstatic.com/x/600x600bb.jpg" {
		t.Fatalf("%q %v", got, err)
	}
	if _, err := s.MusicCover(context.Background(), "11111111-1111-4111-8111-111111111111", "", ""); !errors.Is(err, ErrServiceInvalid) {
		t.Fatalf("plain http %v", err)
	}
	if _, err := s.MusicCover(context.Background(), "22222222-2222-4222-8222-222222222222", "a", "b"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("none %v", err)
	}
	if _, err := s.MusicCover(context.Background(), "nope", "a", "b"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bad id %v", err)
	}
}
