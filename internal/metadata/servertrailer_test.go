package metadata

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestTrailerPickReadsServer(t *testing.T) {
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path + "?" + r.URL.RawQuery {
		case "/v1/trailers/tv/12?language=de":
			_, _ = w.Write([]byte(`{"key":"deTrailer01","name":"Trailer","language":"de","checked_at":"2026-10-19T00:00:00Z","status":"ok"}`))
		case "/v1/trailers/movie/13?language=en":
			_, _ = w.Write([]byte(`{"key":"bad key!"}`))
		default:
			w.WriteHeader(404)
		}
	})
	p, err := s.TrailerPick(context.Background(), "show", "12", "de")
	if err != nil || p.Key != "deTrailer01" || p.Status != "ok" {
		t.Fatalf("%+v %v", p, err)
	}
	if _, err := s.TrailerPick(context.Background(), "movie", "11", "en"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := s.TrailerPick(context.Background(), "movie", "13", "en"); !errors.Is(err, ErrServiceInvalid) {
		t.Fatal(err)
	}
}
