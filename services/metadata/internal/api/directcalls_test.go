package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/discog"
	"github.com/CoderLuii/holyarrs-metadata/internal/fetchq"
	"github.com/CoderLuii/holyarrs-metadata/internal/musicdata"
)

type fakeDeezerData struct{ fakeMusicData }

func (fakeDeezerData) DeezerArtist(_ context.Context, mbid string) (musicdata.DeezerArtist, error) {
	switch mbid {
	case tA2:
		return musicdata.DeezerArtist{}, musicdata.ErrNone
	case discArtist:
		return musicdata.DeezerArtist{}, musicdata.ErrLater
	}
	return musicdata.DeezerArtist{ID: 42, Top: []musicdata.Song{}, Albums: []musicdata.DeezerAlbum{{ID: 7, Title: "Blonde", Explicit: true}}}, nil
}

func (fakeDeezerData) DeezerAlbum(_ context.Context, id int64) (musicdata.DeezerAlbumInfo, error) {
	if id != 7 {
		return musicdata.DeezerAlbumInfo{}, musicdata.ErrNone
	}
	return musicdata.DeezerAlbumInfo{ID: 7, Label: "Boys Don't Cry", Explicit: true, Tracks: []musicdata.DeezerTrack{{Title: "Nikes", Explicit: true}}}, nil
}

func TestMusicDeezerRoutes(t *testing.T) {
	s, _, _ := setup(Options{Burst: 100, MusicData: fakeDeezerData{}})
	tok := "synthetic-test-token"
	for _, c := range []struct {
		path string
		code int
	}{
		{"/v1/music/deezer/artist/nope", 400},
		{"/v1/music/deezer/album/07", 400},
		{"/v1/music/deezer/album/-1", 400},
		{"/v1/music/deezer/song/7", 400},
		{"/v1/music/deezer/artist/" + tA1 + "?x=1", 400},
		{"/v1/music/deezer/artist/" + tA2, 404},
		{"/v1/music/deezer/artist/" + discArtist, 503},
		{"/v1/music/deezer/album/8", 404},
	} {
		if w := req(s, c.path, tok); w.Code != c.code || (w.Code != 200 && w.Header().Get("Cache-Control") != "no-store") {
			t.Errorf("%s: %d %v, want %d", c.path, w.Code, w.Header(), c.code)
		}
	}
	// A miss needs a credential; a hit is kept a day at the edge.
	if w := req(s, "/v1/music/deezer/artist/"+tA1, ""); w.Code != 401 {
		t.Fatalf("anonymous deezer %d", w.Code)
	}
	w := req(s, "/v1/music/deezer/artist/"+tA1, tok)
	var a struct {
		Artist musicdata.DeezerArtist `json:"artist"`
	}
	json.Unmarshal(w.Body.Bytes(), &a)
	if w.Code != 200 || a.Artist.ID != 42 || !a.Artist.Albums[0].Explicit || !strings.Contains(w.Header().Get("Cache-Control"), "s-maxage=86400") || w.Header().Get("Vary") != "" {
		t.Fatalf("artist %d %s %v", w.Code, w.Body, w.Header())
	}
	w = req(s, "/v1/music/deezer/album/7", tok)
	var al struct {
		Album musicdata.DeezerAlbumInfo `json:"album"`
	}
	json.Unmarshal(w.Body.Bytes(), &al)
	if w.Code != 200 || al.Album.Label != "Boys Don't Cry" || !al.Album.Tracks[0].Explicit || !strings.Contains(w.Header().Get("Cache-Control"), "public") {
		t.Fatalf("album %d %s %v", w.Code, w.Body, w.Header())
	}
	// No music data at all: 404, never a call.
	bare, _, _ := setup(Options{Burst: 100})
	if w := req(bare, "/v1/music/deezer/album/7", tok); w.Code != 404 {
		t.Fatalf("no music data %d", w.Code)
	}
}

func TestReleasesFromStoredDiscographies(t *testing.T) {
	other := "c10bbbfc-cf9e-42e0-be17-e2c3e1d2600d"
	d := &fakeDiscog{artists: map[string]discog.Artist{discArtist: {MBID: discArtist, Complete: true, Walked: time.Now(), Groups: []discog.Group{
		{ID: "33333333-0000-0000-0000-000000000001", Title: "New Album", PrimaryType: "Album", SecondaryTypes: []string{"Live"}, FirstReleaseDate: "2026-10-20"},
		{ID: "33333333-0000-0000-0000-000000000002", Title: "New Single", PrimaryType: "Single", FirstReleaseDate: "2026-10-21"},
		{ID: "33333333-0000-0000-0000-000000000003", Title: "Old Album", PrimaryType: "Album", FirstReleaseDate: "2019-01-01"},
		{ID: "33333333-0000-0000-0000-000000000004", Title: "Year Only", PrimaryType: "Album", FirstReleaseDate: "2026"},
	}}}}
	q := &keyQueue{}
	s, _, _ := setup(Options{Burst: 100, Discog: d})
	s.SetQueue(q)
	tok := "synthetic-test-token"
	if w := req(s, "/v1/music/releases?types=album,lp&artists="+discArtist, tok); w.Code != 400 {
		t.Fatalf("bad type %d", w.Code)
	}
	w := req(s, "/v1/music/releases?from=2026-10-01&to=2026-10-31&types=album,ep&artists="+discArtist+","+other, tok)
	var body struct {
		Partial bool `json:"partial"`
		Items   []struct {
			MBID      string   `json:"mbid"`
			Type      string   `json:"type"`
			Artist    string   `json:"artist_mbid"`
			Secondary []string `json:"secondary_types"`
		} `json:"items"`
	}
	json.Unmarshal(w.Body.Bytes(), &body)
	if w.Code != 200 || len(body.Items) != 1 || body.Items[0].Type != "album" || body.Items[0].Artist != discArtist || body.Items[0].Secondary[0] != "live" {
		t.Fatalf("releases %d %s", w.Code, w.Body)
	}
	// The artist with no stored list: its walk is queued in the background,
	// never waited on, and the answer says it is partial.
	if !body.Partial || w.Header().Get(partialHeader) == "" || len(q.keys) != 1 || q.keys[0] != discog.ArtistKey(other) || q.p[0] != fetchq.P3Refresh {
		t.Fatalf("missing list not queued %v %v %v", body.Partial, q.keys, q.p)
	}
	w = req(s, "/v1/music/releases?from=2026-10-01&to=2026-10-31&artists="+discArtist, tok)
	json.Unmarshal(w.Body.Bytes(), &body)
	if w.Code != 200 || len(body.Items) != 2 || body.Items[0].Type != "single" || body.Partial {
		t.Fatalf("all types %d %s", w.Code, w.Body)
	}
}

func TestDirectCallRoutesAtTheEdge(t *testing.T) {
	for _, p := range []string{"/v1/music/deezer/artist/" + tA1, "/v1/music/deezer/album/7", "/v1/wikidata/artist/" + tA1, "/v1/shows/tvmaze/82"} {
		if edgePolicy(p) != "" || !strings.Contains(edgeAuthPolicy(p), "s-maxage=86400") {
			t.Errorf("%s: edge %q auth %q", p, edgePolicy(p), edgeAuthPolicy(p))
		}
	}
	// Other wikidata and show answers stay as they were.
	for _, p := range []string{"/v1/wikidata/person/5", "/v1/shows/tmdb/82"} {
		if edgeAuthPolicy(p) != "" {
			t.Errorf("%s kept at the edge", p)
		}
	}
}
