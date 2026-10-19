package metadata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTMDBEpisodeStills(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/3/tv/9":
			_, _ = w.Write([]byte(`{"id":9,"name":"Nine","seasons":[{"season_number":1}]}`))
		case "/3/tv/9/season/1":
			_, _ = w.Write([]byte(`{"episodes":[{"id":1,"season_number":1,"episode_number":1,"runtime":44,"still_path":"/s1.jpg"},{"id":2,"season_number":1,"episode_number":2}]}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	sh, err := newTMDB(srv, fakeCred).Show(context.Background(), "tmdb:9")
	if err != nil {
		t.Fatal(err)
	}
	if len(sh.Episodes) != 2 || sh.Episodes[0].Still != ImageBase+"/w780/s1.jpg" || sh.Episodes[0].Runtime != 44 || sh.Episodes[1].Still != "" {
		t.Fatalf("episodes %+v", sh.Episodes)
	}
}
