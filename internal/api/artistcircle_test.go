package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs/internal/library"
	"github.com/CoderLuii/holyarrs/internal/metadata"
)

// idArtists links each artist to a Deezer id, as MusicBrainz's links do.
type idArtists struct {
	metadata.ArtistProvider
	ids map[string]metadata.ArtistIDs
}

func (f idArtists) ArtistIDs(_ context.Context, mbid string) (metadata.ArtistIDs, error) {
	return f.ids[mbid], nil
}

// TestArtistCircleNameCollision: the artist picture is found by id. Two
// artists named "Drake" exist; the circle never shows the other one (an
// album cover reading "DRAKE / SIMBIOSIS"), and with no picture by id it
// shows the hero's wide photo, or nothing.
func TestArtistCircleNameCollision(t *testing.T) {
	_, s := discoverRig(t, newDiscoverTMDB(t).provider())
	const drake, linked, wide, none = "9fff2f8a-21e6-47de-a2b8-7f449929d43f", "11111111-1111-4111-8111-111111111111",
		"22222222-2222-4222-8222-222222222222", "33333333-3333-4333-8333-333333333333"
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search/artist":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"id":555,"name":"Drake","picture_xl":"` + srv.URL + `/pic/simbiosis.jpg"}]}`))
		case "/artist/246791":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":246791,"name":"Drake","picture_xl":"` + srv.URL + `/pic/drake.jpg"}`))
		default:
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("\xff\xd8\xff synthetic " + r.URL.Path))
		}
	}))
	defer srv.Close()
	extra := func(_ context.Context, kind, id string) (metadata.ServerArt, error) {
		if id == wide {
			return metadata.ServerArt{Backgrounds: []string{srv.URL + "/pic/wide.jpg"}}, nil
		}
		return metadata.ServerArt{}, metadata.ErrNotFound
	}
	art := &library.ArtworkCache{Dir: filepath.Join(t.TempDir(), "art"), Allowed: []string{srv.URL + "/pic/"}, HTTP: srv.Client()}
	lib := &library.Service{Store: s, Artwork: art,
		ArtistImages: &metadata.ArtistImages{BaseURL: srv.URL, Gap: time.Millisecond, Timeout: time.Second},
		Artists:      idArtists{ids: map[string]metadata.ArtistIDs{drake: {Deezer: 246791}}}}
	a := New(Server{Store: s, Metadata: newDiscoverTMDB(t).provider(), ExtraArt: extra, Library: lib, Version: "test"})
	a.rememberMusicNames([]nameKey{{kind: "artist", mbid: drake, name: "Drake"}, {kind: "artist", mbid: linked, name: "Drake"},
		{kind: "artist", mbid: wide, name: "Drake"}, {kind: "artist", mbid: none, name: "Drake"}})
	ctx := context.Background()
	for _, tc := range []struct{ mbid, want string }{
		{drake, srv.URL + "/pic/drake.jpg"},
		{wide, srv.URL + "/pic/wide.jpg"},
		{none, ""},
	} {
		name, _, ok := a.fetchMusicArt(ctx, "artist", tc.mbid, "250")
		if tc.want == "" {
			if ok {
				t.Fatalf("%s: a picture %s, want none over a wrong one", tc.mbid, name)
			}
			continue
		}
		if !ok || name != library.ArtworkName(tc.want) {
			t.Fatalf("%s: picture %q %v, want %s", tc.mbid, name, ok, tc.want)
		}
	}
	if _, ok := art.Path(library.ArtworkName(srv.URL + "/pic/simbiosis.jpg")); ok {
		t.Fatal("the same-named artist's cover was fetched")
	}
}
