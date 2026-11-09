package music

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// route: GET /api/v1/albums/{id}/cover
func TestAlbumCoverServesTheCachedCover(t *testing.T) {
	t.Parallel()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	pics := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/img/album/"+apiAlbum+"/500.webp" {
			// Not stored on the server: it redirects to the archive, which
			// the app never follows.
			http.Redirect(w, r, "/release-group/x/front-250", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("\xff\xd8\xff synthetic"))
	}))
	defer pics.Close()
	art := &library.ArtworkCache{Dir: filepath.Join(t.TempDir(), "art"), Allowed: []string{pics.URL + "/"}, HTTP: pics.Client()}
	q := jobs.New(s, jobs.Options{})
	lib := &library.Service{Store: s, Queue: q, Artists: apiMusic(), Artwork: art}
	srv := httptest.NewServer(New(Server{Store: s, Queue: q, Library: lib, Metadata: &metadata.Static{}, ImageBase: pics.URL + "/v1/img"}).Handler())
	defer srv.Close()
	u := srv.URL + "/api/v1"
	do(t, "POST", u+"/library-folders", fmt.Sprintf(`{"path":%q,"kind":"music","label":"Music"}`, t.TempDir()), 201)
	var added struct {
		Title struct {
			ID int64 `json:"id"`
		} `json:"title"`
	}
	_ = json.Unmarshal(do(t, "POST", u+"/artists", `{"id":"mbid:`+apiArtist+`","albums":"all"}`, 201), &added)
	if _, err := lib.RefreshArtist(context.Background(), added.Title.ID, domain.System, nil); err != nil {
		t.Fatal(err)
	}
	var albums struct {
		Albums []library.AlbumSummary `json:"albums"`
	}
	_ = json.Unmarshal(do(t, "GET", fmt.Sprintf("%s/titles/%d/albums", u, added.Title.ID), "", 200), &albums)
	var with, without int64
	for _, a := range albums.Albums {
		if a.MBID == apiAlbum {
			with = a.ID
		} else {
			without = a.ID
		}
	}
	if with == 0 || without == 0 {
		t.Fatalf("albums %+v", albums.Albums)
	}
	resp, err := http.Get(fmt.Sprintf("%s/albums/%d/cover", u, with))
	if err != nil || resp.StatusCode != 200 || resp.Header.Get("Cache-Control") != "private, max-age=86400" {
		t.Fatalf("cover %v %d", err, resp.StatusCode)
	}
	resp.Body.Close()
	// Not stored on the server yet: nothing to show (204); an unknown id is 404.
	do(t, "GET", fmt.Sprintf("%s/albums/%d/cover", u, without), "", 204)
	do(t, "GET", u+"/albums/999999/cover", "", 404)
}
