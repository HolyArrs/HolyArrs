package metadata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TMDB's find by TVDB 82817 lists TMDB 82817 (another show that carries
// the number as its TVDB id) before the show itself: that one is skipped.
func TestTMDBFindSkipsShowWhoseIDIsTheTVDBNumber(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/3/find/82817":
			_, _ = w.Write([]byte(`{"tv_results":[{"id":82817},{"id":24285}]}`))
		case "/3/tv/24285":
			_, _ = w.Write([]byte(`{"id":24285,"name":"MasterChef: The Professionals","external_ids":{"tvdb_id":82817}}`))
		case "/3/tv/82817":
			_, _ = w.Write([]byte(`{"id":82817,"name":"Go Go Squid!","external_ids":{"tvdb_id":82817}}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	tm := newTMDB(srv, fakeCred)
	if got, err := tm.ShowTMDBID(context.Background(), "tvdb:82817"); err != nil || got != "24285" {
		t.Fatalf("find id %q %v", got, err)
	}
	sh, err := tm.Show(context.Background(), "tvdb:82817")
	if err != nil || sh.IDs["tmdb"] != "24285" {
		t.Fatalf("show %v %v", sh.IDs, err)
	}
}
