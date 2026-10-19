package metadata

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestNetflixTop10ReadsList(t *testing.T) {
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/lists/netflix-top10" || r.Header.Get("Authorization") == "" {
			t.Error("bad request " + r.URL.String())
		}
		switch r.URL.RawQuery {
		case "kind=movie&lang=en&region=global":
			_, _ = w.Write([]byte(`{"schema_version":1,"kind":"movie","region":"global","lang":"en","week":"2026-10-04","regions":["global","US"],"items":[{"rank":1,"tmdb_id":11,"title":"UNABOMBER","poster_path":"/u.jpg","views":24900000,"weeks_in_top10":2},{"rank":2,"title":"Unknown","weeks_in_top10":1}]}`))
		default:
			w.WriteHeader(404)
		}
	})
	l, err := s.NetflixTop10(context.Background(), "movie", "global", "en")
	if err != nil || l.Week != "2026-10-04" || len(l.Regions) != 2 || len(l.Items) != 2 || l.Items[0].TMDBID != 11 || l.Items[0].PosterPath != "/u.jpg" || l.Items[0].Views != 24900000 || l.Items[1].TMDBID != 0 {
		t.Fatalf("%+v %v", l, err)
	}
	if _, err := s.NetflixTop10(context.Background(), "tv", "ZZ", ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing list: %v", err)
	}
}
