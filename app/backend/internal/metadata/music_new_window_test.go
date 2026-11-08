package metadata

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestMusicNewWindowReads(t *testing.T) {
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/music/new" || r.Header.Get("Authorization") == "" {
			w.WriteHeader(404)
			return
		}
		switch r.URL.Query().Get("window") {
		case "week":
			_, _ = w.Write([]byte(`{"window":"week","items":[{"rank":1,"mbid":"11111111-1111-4111-8111-111111111111","title":"A","artist_name":"B","release_date":"2026-10-19","type":"album","explicit":true,"popularity":3}]}`))
		case "recent":
			_, _ = w.Write([]byte(`{"window":"other"}`))
		default:
			w.WriteHeader(404)
		}
	})
	got, err := s.MusicNewWindow(context.Background(), "week")
	if err != nil || len(got.Items) != 1 || !got.Items[0].Explicit || got.Items[0].Popularity != 3 || got.Items[0].Type != "album" {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err := s.MusicNewWindow(context.Background(), "recent"); !errors.Is(err, ErrServiceInvalid) {
		t.Fatalf("mismatched window %v", err)
	}
	if _, err := s.MusicNewWindow(context.Background(), "upcoming"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing %v", err)
	}
}
