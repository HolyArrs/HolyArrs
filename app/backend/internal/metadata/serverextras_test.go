package metadata

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestArtAndRatingsReadServer(t *testing.T) {
	var s *Service
	s = testService(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/art/tv/1399":
			img := s.Base() + "/v1/img/"
			_, _ = w.Write([]byte(`{"logo":"` + img + `title-logo/tv-1399/1200.png","banner":"` + img + `title-banner/tv-1399/1000.webp",
				"clearart":"` + img + `title-clearart/tv-1399/1000.png",
				"backgrounds":["https://assets.fanart.tv/b.jpg","` + img + `title-wide/tv-1399/1600.webp?v=a1","` + img + `title-wide/../../x/1.jpg"]}`))
		case "/v1/ratings/movie/603":
			_, _ = w.Write([]byte(`{"imdb_id":"tt0133093","scores":[{"source":"imdb","value":8.7,"max":10,"votes":2000000}]}`))
		default:
			w.WriteHeader(404)
		}
	})
	a, err := s.Art(context.Background(), "show", "1399")
	if err != nil || !strings.HasSuffix(a.Logo, "/v1/img/title-logo/tv-1399/1200.png") || a.Banner == "" ||
		len(a.Backgrounds) != 1 || !strings.HasSuffix(a.Backgrounds[0], "/1600.webp?v=a1") {
		t.Fatalf("%+v %v", a, err)
	}
	if _, err := s.Art(context.Background(), "artist", "5b11f4ce-a62d-471e-81fc-a69a8278c7da"); !errors.Is(err, ErrServiceInvalid) {
		t.Fatal(err)
	}
	r, err := s.Ratings(context.Background(), "movie", "603")
	if err != nil || len(r.Scores) != 1 || r.Scores[0].Votes != 2000000 {
		t.Fatalf("%+v %v", r, err)
	}
	if _, err := s.Ratings(context.Background(), "show", "5"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}
