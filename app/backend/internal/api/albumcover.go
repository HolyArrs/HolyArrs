package api

import (
	"net/http"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/store"
)

// albumCoverArt serves a Library album's cover for the title pages (the
// album hero and the artist's discography cards): the metadata server's
// stored copy by release group, nothing else. The picture is kept in the artwork cache, like every other
// picture, so the browser never fetches it from outside. A Kid gets 404
// (kidGuard). An album with no cover stored (yet) answers 204, like the
// other picture routes, so the page shows its placeholder without a failed
// request in the console.
func (a *API) albumCoverArt(w http.ResponseWriter, r *http.Request) {
	p, ok := a.albumID(w, r)
	if !ok {
		return
	}
	lib := a.s.Library
	mbid := strings.ToLower(p.SourceIDs["musicbrainz"])
	if lib == nil || lib.Artwork == nil {
		http.NotFound(w, r)
		return
	}
	if mbid == "" {
		noPicture(w)
		return
	}
	var t domain.Title
	if err := store.Get(r.Context(), a.s.Store.Read(), "titles", p.TitleID, &t); err != nil {
		http.NotFound(w, r)
		return
	}
	// Only the metadata server's stored copy (Luis, 2026-10-24): an album it
	// has not stored yet is 204 (the page shows its placeholder) until it has.
	size := coverSize(r)
	want := size
	if size == "250" {
		want = "500" // 250 is made here from the 500
	}
	src := a.musicArtSrc("album", mbid, want, a.pictureVersion("album", mbid))
	if src == "" {
		noPicture(w)
		return
	}
	name := library.ArtworkName(src)
	path, ok := lib.Artwork.Path(name)
	if ok {
		lib.Artwork.Recheck(src, serverStandIn)
	} else {
		if _, err := lib.Artwork.FetchStored(r.Context(), src, serverStandIn); err != nil {
			noPicture(w)
			return
		}
		if path, ok = lib.Artwork.Path(name); !ok {
			http.NotFound(w, r)
			return
		}
	}
	if size == "250" {
		if n, p, err := lib.Artwork.Smaller(name, 250); err == nil && n != name {
			serveArtworkCached(w, r, n, p, "private, max-age=86400")
			return
		}
	}
	serveArtwork(w, r, name, path)
}
