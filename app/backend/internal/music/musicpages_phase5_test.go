package music

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Music Phase 5: the artist and album pages.

// route: GET /api/v1/music/artists/{mbid}
// route: POST /api/v1/asks
// Ask for all missing: the page says N and about how big; asking with its
// list makes one ask that carries exactly those N releases.
func TestArtistAskAllMissing(t *testing.T) {
	t.Parallel()
	r := newMusicAskRig(t)
	var ap ArtistPageView
	_ = json.Unmarshal(r.ana.call("GET", "/api/v1/music/artists/"+apiArtist, "", 200), &ap)
	// First Light only: Live Night is on the live shelf and Soon is not out yet.
	if ap.Missing.Count != 1 || len(ap.Missing.Albums) != 1 || ap.Missing.Albums[0] != apiAlbum || ap.Missing.SizeBytes != missingAlbumBytes {
		t.Fatalf("missing %+v", ap.Missing)
	}
	body, _ := json.Marshal(map[string]any{"kind": "artist", "id": "mbid:" + apiArtist, "albums": ap.Missing.Albums})
	a := askOf(t, r.ana.call("POST", "/api/v1/asks", string(body), 201))
	if len(a.Albums) != ap.Missing.Count {
		t.Fatalf("ask carries %d releases, want %d: %+v", len(a.Albums), ap.Missing.Count, a)
	}
}

// route: POST /api/v1/asks
// route: POST /api/v1/asks/{id}/approve
// The edition picked on the album page travels with the ask, and the
// approval chooses that edition for the album.
func TestAskEditionChangesTheRelease(t *testing.T) {
	t.Parallel()
	r := newMusicAskRig(t)
	// An edition for an album not asked, or a bad id, is refused.
	r.ana.call("POST", "/api/v1/asks", `{"kind":"artist","id":"mbid:`+apiArtist+`","albums":["`+apiLive+`"],"editions":{"`+apiAlbum+`":"`+apiRelB+`"}}`, 400)
	r.ana.call("POST", "/api/v1/asks", `{"kind":"artist","id":"mbid:`+apiArtist+`","albums":["`+apiAlbum+`"],"editions":{"`+apiAlbum+`":"nope"}}`, 400)
	r.ana.call("POST", "/api/v1/asks", `{"id":"tmdb:1","editions":{"`+apiAlbum+`":"`+apiRelB+`"}}`, 400)

	a := askOf(t, r.ana.call("POST", "/api/v1/asks", `{"kind":"artist","id":"mbid:`+apiArtist+`","albums":["`+apiAlbum+`"],"editions":{"`+apiAlbum+`":"`+apiRelB+`"}}`, 201))
	if len(a.Albums) != 1 || a.Albums[0].Edition != apiRelB {
		t.Fatalf("ask %+v", a)
	}
	got := askOf(t, r.owner.call("POST", fmt.Sprintf("/api/v1/asks/%d/approve", a.ID), `{}`, 200))
	if got.TitleID == nil {
		t.Fatalf("approved %+v", got)
	}
	var chosen string
	if err := r.st.Read().QueryRow(`SELECT p.mbid FROM pressings p JOIN parts a ON a.id = p.part_id WHERE a.title_id = ? AND p.chosen = 1 AND a.name = 'First Light'`, *got.TitleID).Scan(&chosen); err != nil {
		t.Fatal(err)
	}
	if chosen != apiRelB {
		t.Fatalf("chosen pressing %s, want the asked edition %s", chosen, apiRelB)
	}
}

// route: GET /api/v1/music/artists/{mbid}
// route: GET /api/v1/music/albums/{mbid}
// No video kind in any page answer; each edition carries its own tracks.
func TestMusicPagesNoVideoAndEditionTracks(t *testing.T) {
	t.Parallel()
	srv, _, _, fa := musicSetup(t)
	ar := fa.artists[apiArtist]
	ar.Albums = append(ar.Albums, metadata.Album{MBID: "10000000-0000-0000-0000-0000000000ad", Name: "The Clip", PrimaryType: "video", ReleaseDate: "2022-01-01"},
		metadata.Album{MBID: "10000000-0000-0000-0000-0000000000ae", Name: "Hits", PrimaryType: "album", SecondaryTypes: []string{"compilation"}, ReleaseDate: "2023"})
	fa.artists[apiArtist] = ar
	u := srv.URL + "/api/v1/music"
	for _, path := range []string{"/artists/" + apiArtist, "/artists/" + apiArtist + "?part=more", "/albums/" + apiAlbum + "?artist=" + apiArtist} {
		b := strings.ToLower(string(do(t, "GET", u+path, "", 200)))
		if strings.Contains(b, `"video"`) || strings.Contains(b, "the clip") {
			t.Fatalf("%s answers a video: %s", path, b)
		}
	}
	var ap ArtistPageView
	_ = json.Unmarshal(do(t, "GET", u+"/artists/"+apiArtist, "", 200), &ap)
	shelf := map[string]string{}
	for _, a := range ap.Albums {
		shelf[a.Name] = a.Shelf
	}
	if shelf["Hits"] != "compilations" || shelf["Live Night"] != "live" {
		t.Fatalf("shelves %v", shelf)
	}
	var al AlbumPageView
	_ = json.Unmarshal(do(t, "GET", u+"/albums/"+apiAlbum+"?artist="+apiArtist, "", 200), &al)
	if len(al.Editions) != 2 || len(al.Editions[0].List)+len(al.Editions[1].List) != 5 {
		t.Fatalf("editions %+v", al.Editions)
	}
	var more struct {
		Essential []string     `json:"essential"`
		AppearsOn []PageCredit `json:"appears_on"`
	}
	if err := json.Unmarshal(do(t, "GET", u+"/artists/"+apiArtist+"?part=more", "", 200), &more); err != nil || more.Essential == nil || more.AppearsOn == nil {
		t.Fatalf("more %+v %v", more, err)
	}
	var near struct {
		Near []PageCredit `json:"near"`
	}
	if err := json.Unmarshal(do(t, "GET", u+"/albums/"+apiAlbum+"?part=near&artist="+apiArtist, "", 200), &near); err != nil || near.Near == nil {
		t.Fatalf("near %+v %v", near, err)
	}
}

// Essential and Appears on from the server's artist data.
func TestArtistMoreFromArtistData(t *testing.T) {
	t.Parallel()
	r := newMusicAskRig(t)
	other := "a0000000-0000-0000-0000-0000000000bb"
	r.api.s.MusicArtistData = func(_ context.Context, ids []string) ([]metadata.MusicArtistData, error) {
		return []metadata.MusicArtistData{{MBID: apiArtist, Albums: []metadata.MusicTopAlbum{
			{MBID: apiLive, Title: "Live Night", Type: "album", Listeners: 10, Artists: []string{apiArtist}},
			{MBID: apiAlbum, Title: "First Light", Type: "album", Listeners: 90, Artists: []string{apiArtist}},
			{MBID: "10000000-0000-0000-0000-0000000000cc", Title: "Duet", Type: "single", Listeners: 50, Artists: []string{other, apiArtist}, ArtistNames: []string{"Other", "Lab Band"}},
			{MBID: "10000000-0000-0000-0000-0000000000cd", Title: "Clip", Type: "video", Artists: []string{other, apiArtist}},
		}}}, nil
	}
	var more struct {
		Essential []string     `json:"essential"`
		AppearsOn []PageCredit `json:"appears_on"`
	}
	_ = json.Unmarshal(r.ana.call("GET", "/api/v1/music/artists/"+apiArtist+"?part=more", "", 200), &more)
	if strings.Join(more.Essential, ",") != apiAlbum+","+apiLive || len(more.AppearsOn) != 1 || more.AppearsOn[0].ArtistName != "Other" || more.AppearsOn[0].ArtistMBID != other {
		t.Fatalf("more %+v", more)
	}
}
