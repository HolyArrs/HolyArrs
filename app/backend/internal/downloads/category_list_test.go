package downloads

import (
	"net/http/httptest"
	"slices"
	"testing"
)

func TestQbitListCategories(t *testing.T) {
	f := newFake()
	f.listCats = true
	f.cats = []string{"tv-sonarr", "radarr"}
	srv := httptest.NewServer(f)
	defer srv.Close()
	got, err := NewQbittorrent(srv.URL, nil).ListCategories(t.Context())
	if err != nil || !slices.Equal(got, []string{"radarr", "tv-sonarr"}) {
		t.Fatalf("%v %v", got, err)
	}
}

func TestSabListCategoriesDropsStar(t *testing.T) {
	key := "0123456789abcdef0123456789abcdef"
	sab, _ := sabCats(t, key, "tv", "movies")
	got, err := NewSabnzbd(sab.URL, key, nil).ListCategories(t.Context())
	if err != nil || !slices.Equal(got, []string{"tv", "movies"}) {
		t.Fatalf("%v %v", got, err)
	}
}

func TestNZBGetListCategories(t *testing.T) {
	f, base := startNZBGet(t)
	f.config = []nzbParam{{"MainDir", "/data"}, {"Category1.Name", "Movies"}, {"Category2.Name", " "}, {"Category3.Name", "Series"}, {"Category5.Name", "after a gap"}}
	got, err := NewNZBGet(base, f.user, f.pass, nil).ListCategories(t.Context())
	if err != nil || !slices.Equal(got, []string{"Movies", "Series"}) {
		t.Fatalf("%v %v", got, err)
	}
	if PickCategory("show", got) != "Series" || PickCategory("movie", got) != "Movies" {
		t.Fatal("picks")
	}
}
