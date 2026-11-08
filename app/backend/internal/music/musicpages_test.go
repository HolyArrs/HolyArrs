package music

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// route: GET /api/v1/music/artists/{mbid}
// route: GET /api/v1/music/albums/{mbid}
func TestMusicPages(t *testing.T) {
	t.Parallel()
	srv, _, lib, _ := musicSetup(t)
	u := srv.URL + "/api/v1/music"

	do(t, "GET", u+"/artists/nope", "", 400)
	do(t, "GET", u+"/artists/"+apiAlbum, "", 404)
	var ap ArtistPageView
	_ = json.Unmarshal(do(t, "GET", u+"/artists/"+apiArtist, "", 200), &ap)
	if ap.Name != "Lab Band" || len(ap.Albums) != 3 || ap.TitleID != 0 || ap.Albums[0].Name != "Soon" || !ap.Albums[0].Upcoming ||
		ap.Latest == nil || ap.Latest.MBID != apiAlbum || ap.Total != 2 || ap.Have != 0 {
		t.Fatalf("artist page %+v", ap)
	}
	shelves := map[string]string{}
	for _, a := range ap.Albums {
		shelves[a.MBID] = a.Shelf
		if a.State != "" {
			t.Fatalf("not in the Library, yet a state: %+v", a)
		}
	}
	if shelves[apiAlbum] != "albums" || shelves[apiLive] != "live" || shelves[apiSoon] != "singles" {
		t.Fatalf("shelves %v", shelves)
	}

	// The hero answers on its own; the shelves skip similar on ?similar=0.
	do(t, "GET", u+"/artists/nope?part=core", "", 400)
	do(t, "GET", u+"/artists/"+apiAlbum+"?part=core", "", 404)
	var core ArtistCoreView
	_ = json.Unmarshal(do(t, "GET", u+"/artists/"+apiArtist+"?part=core", "", 200), &core)
	if core.Name != "Lab Band" || core.MBID != apiArtist || core.Image == "" || core.TitleID != 0 {
		t.Fatalf("artist core %+v", core)
	}
	var noSim struct {
		Similar []PageArtist `json:"similar"`
	}
	if err := json.Unmarshal(do(t, "GET", u+"/artists/"+apiArtist+"?similar=0", "", 200), &noSim); err != nil || noSim.Similar == nil || len(noSim.Similar) != 0 {
		t.Fatalf("similar=0 %+v %v", noSim, err)
	}
	if err := json.Unmarshal(do(t, "GET", u+"/artists/"+apiArtist+"?part=similar", "", 200), &noSim); err != nil || noSim.Similar == nil {
		t.Fatalf("similar %+v %v", noSim, err)
	}
	do(t, "GET", u+"/albums/"+apiAlbum+"?part=core", "", 400)
	var acore AlbumCoreView
	_ = json.Unmarshal(do(t, "GET", u+"/albums/"+apiAlbum+"?part=core&artist="+apiArtist, "", 200), &acore)
	if acore.Name != "First Light" || acore.Artist.Name != "Lab Band" || acore.Image == "" {
		t.Fatalf("album core %+v", acore)
	}

	// A bare album link finds its artist; an unknown album is not found.
	var bare AlbumPageView
	_ = json.Unmarshal(do(t, "GET", u+"/albums/"+apiAlbum, "", 200), &bare)
	if bare.Name != "First Light" || bare.Artist.MBID != apiArtist || bare.Artist.Name != "Lab Band" {
		t.Fatalf("bare album page %+v", bare)
	}
	do(t, "GET", u+"/albums/f0000000-0000-0000-0000-0000000000ff", "", 404)
	// A wrong, bad or missing ?artist= never blocks the page: the album names its own artist.
	for _, q := range []string{"?artist=nope", "?artist=" + apiAlbum, "?artist=f0000000-0000-0000-0000-0000000000ee", ""} {
		var m AlbumPageView
		_ = json.Unmarshal(do(t, "GET", u+"/albums/"+apiAlbum+q, "", 200), &m)
		if m.Name != "First Light" || m.Artist.MBID != apiArtist {
			t.Fatalf("mismatched link %q: %+v", q, m)
		}
	}
	var al AlbumPageView
	_ = json.Unmarshal(do(t, "GET", u+"/albums/"+apiAlbum+"?artist="+apiArtist, "", 200), &al)
	if al.Name != "First Light" || al.Artist.Name != "Lab Band" || len(al.Tracks) != 2 || al.LengthMS != 360000 || len(al.Editions) != 2 ||
		len(al.MoreBy) != 1 || al.Tracks[0].Have != "" {
		t.Fatalf("album page %+v", al)
	}

	// In the Library: states per album and have per track.
	var added struct {
		Title struct {
			ID int64 `json:"id"`
		} `json:"title"`
	}
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/artists", `{"id":"mbid:`+apiArtist+`","albums":"latest","new_albums":"new"}`, 201), &added)
	if _, err := lib.RefreshArtist(context.Background(), added.Title.ID, domain.System, nil); err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(do(t, "GET", u+"/artists/"+apiArtist, "", 200), &ap)
	if ap.TitleID != added.Title.ID {
		t.Fatalf("title id %+v", ap)
	}
	_ = json.Unmarshal(do(t, "GET", u+"/artists/"+apiArtist+"?part=core", "", 200), &core)
	if core.TitleID != added.Title.ID {
		t.Fatalf("core title id %+v", core)
	}
	for _, a := range ap.Albums {
		if a.MBID == apiAlbum && (a.State != "missing" || a.PartID == 0) {
			t.Fatalf("First Light state %+v", a)
		}
	}
	_ = json.Unmarshal(do(t, "GET", u+"/albums/"+apiAlbum+"?artist="+apiArtist, "", 200), &al)
	if al.State != "missing" || al.Tracks[0].Have != "missing" {
		t.Fatalf("album in the Library %+v", al)
	}
}

func TestExplicitAlbums(t *testing.T) {
	t.Parallel()
	is := explicitAlbums([]metadata.DeezerAlbum{
		{Title: "Aerodynamic (Slum Village Remix)", Explicit: true},
		{Title: "Aerodynamic", Explicit: false},
		{Title: "Pablo Honey", Explicit: true},
		{Title: "Pablo Honey (Collector's Edition)", Explicit: true},
		{Title: "Discovery (Remastered)", Explicit: true},
	})
	for name, want := range map[string]bool{
		"Aerodynamic (Slum Village remix)": true, // exact, brackets kept
		"Aerodynamic":                      false,
		"Aerodynamic (special edition)":    false, // base has a clean album
		"Pablo Honey":                      true,
		"Discovery":                        true, // only explicit albums share the base
		"Homework":                         false,
	} {
		if got := is(name); got != want {
			t.Errorf("%q explicit = %v, want %v", name, got, want)
		}
	}
}

func TestEditionsFoldPressings(t *testing.T) {
	t.Parallel()
	tr := func(n int) []metadata.Track { return make([]metadata.Track, n) }
	got := editions([]metadata.Pressing{
		{MBID: "a", Name: "OK Computer", Status: "official", Date: "1997-07-01", Tracks: tr(12)},
		{MBID: "b", Name: "OK Computer", Status: "official", Date: "1997-06-16", Disambiguation: "EMI UDEN", Tracks: tr(12)},
		{MBID: "c", Name: "OK Computer", Status: "official", Date: "2016-05-01", Disambiguation: "AG31715-01 manufactured by optimal media", Tracks: tr(12)},
		{MBID: "d", Name: "OK Computer", Status: "official", Date: "2009-03-24", Disambiguation: "special collector’s edition", Tracks: tr(27)},
		{MBID: "e", Name: "OK Computer: OKNOTOK 1997 2017", Status: "official", Date: "2017-07-07", Tracks: tr(23)},
		{MBID: "f", Name: "OK Computer OKNOTOK 1997 2017", Status: "official", Date: "2017-06-23", Tracks: tr(23)},
		{MBID: "g", Name: "OK Computer", Status: "bootleg", Tracks: tr(12)},
	})
	want := "b OK Computer 12 1997-06-16|d OK Computer (special collector’s edition) 27 2009-03-24|f OK Computer: OKNOTOK 1997 2017 23 2017-06-23|"
	var s string
	for _, e := range got {
		s += e.MBID + " " + e.Label + " " + strconv.Itoa(e.Tracks) + " " + e.Date + "|"
	}
	if s != want {
		t.Fatalf("editions\n got %s\nwant %s", s, want)
	}
}

func TestSimilarArtistArtPrefetched(t *testing.T) {
	t.Parallel()
	_, s := discoverRig(t, newDiscoverTMDB(t).provider())
	const seed, like = "33333333-3333-4333-8333-333333333333", "44444444-4444-4444-8444-444444444444"
	pics := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("\xff\xd8\xff synthetic"))
	}))
	defer pics.Close()
	similar := func(_ context.Context, mbid string) ([]metadata.SimilarArtist, error) {
		return []metadata.SimilarArtist{{MBID: like, Name: "Like Band", Score: 9}}, nil
	}
	extra := func(_ context.Context, kind, id string) (metadata.ServerArt, error) {
		if kind != "artist" || id != like {
			return metadata.ServerArt{}, metadata.ErrNotFound
		}
		return metadata.ServerArt{Thumb: pics.URL + "/thumb.jpg"}, nil
	}
	_ = extra
	art := &library.ArtworkCache{Dir: filepath.Join(t.TempDir(), "art"), Allowed: []string{pics.URL + "/"}, HTTP: pics.Client()}
	a := New(Server{Store: s, Metadata: newDiscoverTMDB(t).provider(), ImageBase: pics.URL + "/v1/img",
		Library: &library.Service{Store: s, Artwork: art}, Version: "test", Deps: Deps{MusicSimilar: similar}})
	list := a.similarArtists(context.Background(), seed, 14)
	if len(list) != 1 || list[0].Image != "/api/v1/music-trending/art/artist/"+like {
		t.Fatalf("similar %+v", list)
	}
	a.music.prefetching.Wait()
	if _, ok := art.Path(library.ArtworkName(pics.URL + "/v1/img/artist/" + like + "/500.webp")); !ok {
		t.Fatal("similar artist picture not prefetched from the server")
	}
	if _, ok := art.Path(library.ArtworkName(pics.URL + "/thumb.jpg")); ok {
		t.Fatal("an outside picture was fetched")
	}
}

// The album page's "Fans also like" and the artist page's "Appears on"
// albums come from no list the picture route knew: they are listed and
// their covers prefetched from the server's stored copies.
func TestCreditCoversPrefetched(t *testing.T) {
	t.Parallel()
	_, s := discoverRig(t, newDiscoverTMDB(t).provider())
	const al = "55555555-5555-4555-8555-555555555555"
	pics := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("\xff\xd8\xff synthetic"))
	}))
	defer pics.Close()
	art := &library.ArtworkCache{Dir: filepath.Join(t.TempDir(), "art"), Allowed: []string{pics.URL + "/"}, HTTP: pics.Client()}
	a := New(Server{Store: s, Metadata: newDiscoverTMDB(t).provider(), ImageBase: pics.URL + "/v1/img",
		Library: &library.Service{Store: s, Artwork: art}, Version: "test"})
	c := creditOf(metadata.MusicTopAlbum{MBID: al, Title: "OK Computer", Type: "album"})
	c.ArtistName = "Radiohead"
	a.warmCredits([]PageCredit{c})
	a.music.prefetching.Wait()
	if _, ok := art.Path(library.ArtworkName(pics.URL + "/v1/img/album/" + al + "/500.webp")); !ok {
		t.Fatal("credit cover not prefetched from the server")
	}
}
