package metadata

import (
	"context"
	"net/http"
	"testing"
)

func TestMusicShelvesReadsRegion(t *testing.T) {
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/music/lists/shelves" || r.URL.Query().Get("region") != "us" {
			t.Errorf("asked %s", r.URL)
		}
		_, _ = w.Write([]byte(`{"region":"us","blocks":{"top_now":[{"kind":"album","mbid":"m1","title":"One","rank":1,"streams":9}],"new_friday":[{"kind":"artist","mbid":"m2","title":"Two","rank":2}]}}`))
	})
	got, err := s.MusicShelves(context.Background(), "us")
	if err != nil || len(got.Blocks) != 2 || got.Region != "us" {
		t.Fatalf("shelves %+v %v", got, err)
	}
	if it := got.Blocks["top_now"]; len(it) != 1 || it[0].Title != "One" || it[0].Streams != 9 || it[0].Rank != 1 {
		t.Fatalf("items %+v", it)
	}
	nf := testService(t, func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	if _, err := nf.MusicShelves(context.Background(), "us"); err == nil {
		t.Fatal("404 gave no error")
	}
}
