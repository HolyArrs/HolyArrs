package metadata

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestArtAndRatingsReadServer(t *testing.T) {
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/art/tv/1399":
			_, _ = w.Write([]byte(`{"logo":"https://assets.fanart.tv/fanart/tv/1/hdtvlogo/a.png","backgrounds":["https://evil.test/x.jpg","https://assets.fanart.tv/b.jpg"]}`))
		case "/v1/ratings/movie/603":
			_, _ = w.Write([]byte(`{"imdb_id":"tt0133093","scores":[{"source":"imdb","value":8.7,"max":10,"votes":2000000}]}`))
		default:
			w.WriteHeader(404)
		}
	})
	a, err := s.Art(context.Background(), "show", "1399")
	if err != nil || a.Logo == "" || len(a.Backgrounds) != 1 {
		t.Fatalf("%+v %v", a, err)
	}
	if _, err := s.Art(context.Background(), "artist", "nope"); !errors.Is(err, ErrServiceInvalid) {
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

// TestArtArtistUsesMusicRoute: the server's route is /v1/art/music; the
// artist kind must never reach it as /v1/art/artist.
func TestArtArtistUsesMusicRoute(t *testing.T) {
	const mbid = "5b11f4ce-a62d-471e-81fc-a69a8278c7da"
	var asked string
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		asked = r.URL.Path
		if r.URL.Path != "/v1/art/music/"+mbid {
			w.WriteHeader(400)
			return
		}
		_, _ = w.Write([]byte(`{"thumbs":["https://assets.fanart.tv/fanart/music/a.jpg"]}`))
	})
	a, err := s.Art(context.Background(), "artist", mbid)
	if err != nil || asked != "/v1/art/music/"+mbid || len(a.Thumbs) != 1 {
		t.Fatalf("asked %q: %+v %v", asked, a, err)
	}
}