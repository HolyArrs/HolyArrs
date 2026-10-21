package api

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/CoderLuii/holyarrs/internal/domain"
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
