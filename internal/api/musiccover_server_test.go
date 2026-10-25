package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/CoderLuii/holyarrs/internal/library"
	"github.com/CoderLuii/holyarrs/internal/music/rowtpl"
)

// An album cover comes first from the metadata server's stored copy; an
// album it has not stored (a redirect) takes the other sources, and any
// catalog album answers at once, listed by a page or not.
func TestAlbumCoverServerCopyFirstAndCatalogAlbums(t *testing.T) {
	_, s := discoverRig(t, newDiscoverTMDB(t).provider())
	const stored, cold, stranger = "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222", "33333333-3333-4333-8333-333333333333"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/img/album/" + stored + "/250.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("\xff\xd8\xff stored"))
		case "/v1/img/album/" + cold + "/250.jpg", "/v1/img/album/" + stranger + "/250.jpg":
			http.Redirect(w, r, "/release-group/x/front-250", http.StatusFound)
		default:
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("\xff\xd8\xff archive " + r.URL.Path))
		}
	}))
	defer srv.Close()
	art := &library.ArtworkCache{Dir: filepath.Join(t.TempDir(), "art"), Allowed: []string{srv.URL + "/"}, HTTP: srv.Client()}
	lib := &library.Service{Store: s, Artwork: art, CoverArtBase: srv.URL}
	a := New(Server{Store: s, Metadata: newDiscoverTMDB(t).provider(), Library: lib, ImageBase: srv.URL + "/v1/img", Version: "test"})
	b, err := rowtpl.ParseBundle([]byte(`{"version":"t","items":[{"id":"` + stored + `","k":"a","t":"Kept","ar":"A"},{"id":"` + cold + `","k":"a","t":"Cold","ar":"B"}],"keys":{},"names":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	a.musicPre.catalog.Store(b)
	ctx := context.Background()
	if name, _, ok := a.fetchMusicArt(ctx, "album", stored, "250"); !ok || name != library.ArtworkName(srv.URL+"/v1/img/album/"+stored+"/250.jpg") {
		t.Fatalf("stored album %q %v, want the server copy", name, ok)
	}
	if name, _, ok := a.fetchMusicArt(ctx, "album", cold, "250"); !ok || name != library.ArtworkName(srv.URL+"/release-group/"+cold+"/front-250") {
		t.Fatalf("cold album %q %v, want the archive cover", name, ok)
	}
	if _, ok := art.Path(library.ArtworkName(srv.URL + "/v1/img/album/" + cold + "/250.jpg")); ok {
		t.Fatal("the redirect was kept as the server copy")
	}
	if _, _, ok := a.fetchMusicArt(ctx, "album", stranger, "250"); ok {
		t.Fatal("an album neither listed nor in the catalog was fetched")
	}
}
