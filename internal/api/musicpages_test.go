package api

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/metadata"
)

// route: GET /api/v1/music/artists/{mbid}
// route: GET /api/v1/music/albums/{mbid}
func TestMusicPages(t *testing.T) {
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

	do(t, "GET", u+"/albums/"+apiAlbum, "", 400)
	do(t, "GET", u+"/albums/"+apiSoon+"?artist="+apiAlbum, "", 404)
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
