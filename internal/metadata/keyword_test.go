package metadata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// A keyword is found by its exact name (any case); a near match is not one.
func TestKeywordIDExactName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/3/search/keyword" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"results":[{"id":1,"name":"slasher film"},{"id":12339,"name":"Slasher"}]}`))
	}))
	defer srv.Close()
	tm := &TMDB{BaseURL: srv.URL + "/3", Access: func(context.Context) (string, error) { return "x", nil }, Gap: time.Millisecond}
	if id, err := tm.KeywordID(context.Background(), "slasher"); err != nil || id != 12339 {
		t.Fatalf("slasher = %d, %v", id, err)
	}
	if id, _ := tm.KeywordID(context.Background(), "giallo"); id != 0 {
		t.Fatalf("unknown keyword = %d", id)
	}
}

// Horror's mood rows are our own labels over TMDB keyword names.
func TestSubMoodsHorror(t *testing.T) {
	got := SubMoods("Horror")
	want := []string{"Supernatural", "Psychological", "Slasher", "Found footage"}
	for i, w := range want {
		if i >= len(got) || got[i].Label != w || len(got[i].Keywords) == 0 {
			t.Fatalf("Horror moods %v", got)
		}
	}
	if SubMoods("Western") != nil {
		t.Fatal("a genre without moods has none")
	}
}
