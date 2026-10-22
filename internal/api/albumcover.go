package api

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/library"
	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// albumCoverArt serves a Library album's cover for the title pages (the
// album hero and the artist's discography cards): the metadata server's
// cover chain by release group, artist and name, else the Cover Art
// Archive. The picture is kept in the artwork cache, like every other
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
	src, found := a.albumCover(r.Context(), mbid, metadata.MusicTrendItem{MBID: mbid, Title: p.Name, Artist: t.Name})
	if !found {
		base := lib.CoverArtBase
		if base == "" {
			base = library.CoverArtBase
		}
		src = strings.TrimRight(base, "/") + "/release-group/" + url.PathEscape(mbid) + "/front-250"
	}
	src = sizedCover(src, coverSize(r))
	if src == "" {
		http.NotFound(w, r)
		return
	}
	name := library.ArtworkName(src)
	path, ok := lib.Artwork.Path(name)
	if !ok {
		if _, err := lib.Artwork.Fetch(r.Context(), src); err != nil {
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
