package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/music"
)

// idArtists links each artist to a Deezer id, as MusicBrainz's links do.
type idArtists struct {
	metadata.ArtistProvider
	ids map[string]metadata.ArtistIDs
}

func (f idArtists) ArtistIDs(_ context.Context, mbid string) (metadata.ArtistIDs, error) {
	return f.ids[mbid], nil
}

// TestArtistCircleNameCollision: the artist picture is found by id, from
// the server only. Two artists named "Drake" exist; the circle never shows
// the other one, and with no stored picture it shows none (placeholder).
func TestArtistCircleNameCollision(t *testing.T) {
	t.Parallel()
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
		case "/v1/img/artist/" + drake + "/500.webp":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("\xff\xd8\xff server " + r.URL.Path))
		default:
			w.Header().Set("Content-Type", "image/jpeg")
			if strings.HasPrefix(r.URL.Path, "/v1/img/") {
				w.Header().Set("X-Metadata-Placeholder", "true")
			}
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
	art := &library.ArtworkCache{Dir: filepath.Join(t.TempDir(), "art"), Allowed: []string{srv.URL + "/"}, HTTP: srv.Client()}
	lib := &library.Service{Store: s, Artwork: art,
		ArtistImages: &metadata.ArtistImages{BaseURL: srv.URL, Gap: time.Millisecond, Timeout: time.Second},
		Artists:      idArtists{ids: map[string]metadata.ArtistIDs{drake: {Deezer: 246791}}}}
	a := buildHandlers(services{Store: s, Metadata: newDiscoverTMDB(t).provider(), ExtraArt: extra, Library: lib, ImageBase: srv.URL + "/v1/img", Version: "test"})
	a.mh().RememberNames([]music.NameKey{music.NewNameKey("artist", drake, "Drake", ""), music.NewNameKey("artist", linked, "Drake", ""),
		music.NewNameKey("artist", wide, "Drake", ""), music.NewNameKey("artist", none, "Drake", "")})
	ctx := context.Background()
	for _, tc := range []struct{ mbid, want string }{
		{drake, srv.URL + "/v1/img/artist/" + drake + "/500.webp"},
		{wide, ""}, // only the server's pictures: no outside wide photo
		{none, ""},
	} {
		name, _, ok := a.musicTrendArtFile(ctx, "artist", tc.mbid, "500")
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
