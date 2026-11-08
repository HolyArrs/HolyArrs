package metadata

import (
	"context"
	"net/http"
	"testing"
)

func TestMusicArtistBatchCalls(t *testing.T) {
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/music/listeners":
			if r.URL.Query().Get("artists") != "A,B" {
				t.Errorf("artists param %q", r.URL.Query().Get("artists"))
			}
			_, _ = w.Write([]byte(`{"items":[{"mbid":"ABC","listeners":5}]}`))
		case "/v1/music/similar-set":
			if r.URL.Query().Get("seeds") != "S1,S2" {
				t.Errorf("seeds param %q", r.URL.Query().Get("seeds"))
			}
			_, _ = w.Write([]byte(`{"seeds":[{"mbid":"SEED","artists":[{"mbid":"x","name":"X","s":1,"source":"lb"}]}]}`))
		case "/v1/music/artist-data":
			if r.URL.Query().Get("artists") != "A" {
				t.Errorf("artists param %q", r.URL.Query().Get("artists"))
			}
			_, _ = w.Write([]byte(`{"artists":[{"mbid":"a","genres":[{"name":"rock","count":3}],"albums":[]}]}`))
		default:
			http.Error(w, "no", http.StatusInternalServerError)
		}
	})
	ctx := context.Background()
	l, err := s.MusicListeners(ctx, []string{"A", "B"})
	if err != nil || len(l) != 1 || l["abc"] != 5 {
		t.Fatalf("listeners %v %v", l, err)
	}
	ss, err := s.MusicSimilarSet(ctx, []string{"S1", "S2"})
	if err != nil || len(ss["seed"]) != 1 || ss["seed"][0].Name != "X" {
		t.Fatalf("similar %v %v", ss, err)
	}
	ad, err := s.MusicArtistData(ctx, []string{"A"})
	if err != nil || len(ad) != 1 || ad[0].Genres[0].Count != 3 {
		t.Fatalf("artist data %v %v", ad, err)
	}

	bad := testService(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) })
	if m, err := bad.MusicListeners(ctx, []string{"A"}); err == nil || m != nil {
		t.Fatalf("500 gave %v %v", m, err)
	}
	if m, err := bad.MusicSimilarSet(ctx, []string{"A"}); err == nil || m != nil {
		t.Fatalf("500 gave %v %v", m, err)
	}
}
