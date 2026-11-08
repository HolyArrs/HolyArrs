package metadata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestExploreListAndPersonCredits(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/3/genre/movie/list", "/3/genre/tv/list":
			_, _ = w.Write([]byte(`{"genres":[{"id":18,"name":"Drama"}]}`))
		case "/3/movie/7/similar":
			_, _ = w.Write([]byte(`{"results":[{"id":1,"title":"A","release_date":"2024-05-01","genre_ids":[18],"popularity":12.5,"vote_average":7.9,"vote_count":800}]}`))
		case "/3/search/person":
			if r.URL.Query().Get("query") != "Dee" {
				t.Errorf("person query %v", r.URL.Query())
			}
			_, _ = w.Write([]byte(`{"results":[{"id":55}]}`))
		case "/3/person/55/combined_credits":
			_, _ = w.Write([]byte(`{"cast":[{"id":2,"media_type":"movie","title":"Low","popularity":1},{"id":3,"media_type":"tv","name":"High","popularity":9}],
				"crew":[{"id":2,"media_type":"movie","title":"Low","popularity":1}]}`))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	p := newTMDB(srv, fakeCred)
	var _ ExploreLister = p
	ctx := context.Background()
	items, err := p.ExploreList(ctx, "movie", "/movie/7/similar", nil)
	if err != nil || len(items) != 1 {
		t.Fatalf("similar: %v %v", items, err)
	}
	it := items[0]
	if it.Popularity != 12.5 || it.VoteAverage != 7.9 || it.VoteCount != 800 || it.Released != "2024-05-01" || it.Genres[0] != "Drama" {
		t.Fatalf("scoring fields not read: %+v", it)
	}
	if _, err := p.ExploreList(ctx, "movie", "/person/1", url.Values{}); err == nil {
		t.Fatal("a path outside the list allowlist was read")
	}
	cr, err := p.PersonCredits(ctx, "Dee")
	if err != nil || len(cr) != 2 || cr[0].Name != "High" || cr[0].Kind != "show" || cr[1].Kind != "movie" {
		t.Fatalf("credits: %+v %v", cr, err)
	}
}
