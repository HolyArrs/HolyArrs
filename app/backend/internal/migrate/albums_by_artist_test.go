package migrate

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// A Lidarr whose whole album list is too big is read one artist at a time.
func TestAlbumsReadPerArtist(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := r.URL.Query().Get("artistId")
		if id == "" {
			t.Fatal("asked for every album at once")
		}
		fmt.Fprintf(w, `[{"id":%s0,"artistId":%s,"title":"a"}]`, id, id)
	}))
	defer srv.Close()
	c, err := newAPIClient(Source{Kind: Lidarr, URL: srv.URL, AllowLocal: true, Limits: Limits{MaxAnswer: 4096}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := (arrReader{kind: Lidarr}).albumsByArtist(context.Background(), c, []map[string]any{{"id": float64(1)}, {"id": float64(2)}})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(got.([]any)); n != 2 {
		t.Fatalf("want 2 albums, got %d", n)
	}
}
