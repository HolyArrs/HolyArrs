package api

import (
	"net/http"
	"strings"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/library"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// albumCoverArt serves a Library album's cover for the title pages (the
// album hero and the artist's discography cards): the metadata server's
// stored copy by release group, nothing else. The picture is kept in the artwork cache, like every other
// picture, so the browser never fetches it from outside. A Kid gets 404
// (kidGuard), and an album with no cover anywhere is 404 too.
func (a *API) albumCoverArt(w http.ResponseWriter, r *http.Request) {
	p, ok := a.albumID(w, r)
	if !ok {
		return
	}
	lib := a.s.Library
	mbid := strings.ToLower(p.SourceIDs["musicbrainz"])
	if lib == nil || lib.Artwork == nil || mbid == "" {
		http.NotFound(w, r)
		return
	}
	var t domain.Title
	if err := store.Get(r.Context(), a.s.Store.Read(), "titles", p.TitleID, &t); err != nil {
		http.NotFound(w, r)
		return
	}
	// Only the metadata server's stored copy (Luis, 2026-10-24): an album it
	// has not stored yet is 404 (the page shows its placeholder) until it has.
	src := a.serverAlbumCoverURL(mbid, coverSize(r))
	if src == "" {
		http.NotFound(w, r)
		return
	}
	name := library.ArtworkName(src)
	path, ok := lib.Artwork.Path(name)
	if ok {
		lib.Artwork.Recheck(src, serverPlaceholder)
	} else {
		if _, err := lib.Artwork.FetchStored(r.Context(), src, serverPlaceholder); err != nil {
			http.Error(w, "picture unavailable", http.StatusNotFound)
			return
		}
		if path, ok = lib.Artwork.Path(name); !ok {
			http.NotFound(w, r)
			return
		}
	}
	serveArtwork(w, r, name, path)
}
