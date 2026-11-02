package api

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/CoderLuii/holyarrs-metadata/internal/kworb"
)

type fakePopularity struct {
	wanted []string
	known  map[string]*kworb.Popularity
}

func (f *fakePopularity) Popularity(mbid string) *kworb.Popularity { return f.known[mbid] }

func (f *fakePopularity) Opened(mbid string) { f.wanted = append(f.wanted, mbid) }

func (f *fakePopularity) Rising() ([]byte, bool) {
	return []byte(`{"schema_version":1,"as_of":"2026-10-26","items":[{"mbid":"` + queuedArtist + `","name":"A","listeners":9,"gain":3,"rank":7}]}`), true
}

func TestArtistAnswerCarriesPopularity(t *testing.T) {
	change := int64(-5)
	f := &fakePopularity{known: map[string]*kworb.Popularity{queuedArtist: {MonthlyListeners: 1000, WorldRank: 7, ListenersChange: &change, AsOf: "2026-10-26"}}}
	s, _, _ := setup(Options{Burst: 100, Popularity: f})
	w := req(s, "/v1/music/musicbrainz/artist/"+queuedArtist, "synthetic-test-token")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"popularity":{"monthly_listeners":1000,"world_rank":7,"listeners_change_today":-5,"as_of":"2026-10-26"}`) {
		t.Fatalf("artist answer %d %s", w.Code, w.Body)
	}
	if len(f.wanted) != 1 || f.wanted[0] != queuedArtist {
		t.Fatalf("an opened page was not noted: %v", f.wanted)
	}
	// A fetch ahead never asks kworb; unknown figures leave the field out.
	f.known = map[string]*kworb.Popularity{}
	r := httptest.NewRequest("GET", "/v1/music/musicbrainz/artist/"+queuedArtist, nil)
	r.Header.Set("Authorization", "Bearer synthetic-test-token")
	r.Header.Set("X-Fetch", "ahead")
	w = httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 200 || strings.Contains(w.Body.String(), `"popularity"`) || len(f.wanted) != 1 || w.Header().Get(partialHeader) != "" {
		t.Fatalf("ahead %d %v %v %s", w.Code, f.wanted, w.Header(), w.Body)
	}
	// The rising row is a shared chart route.
	w = req(s, "/v1/music/charts/rising", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"gain":3`) || !strings.HasPrefix(w.Header().Get("Cache-Control"), "public") {
		t.Fatalf("rising %d %v %s", w.Code, w.Header(), w.Body)
	}
	if w := req(s, "/v1/music/charts/rising?x=1", ""); w.Code != 400 {
		t.Fatalf("query accepted %d", w.Code)
	}
}
