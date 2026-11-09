package music

import (
	"net/http"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// albumCoverArt serves a Library album's cover for the title pages (the
// album hero and the artist's discography cards): the metadata server's
// stored copy by release group, nothing else. The picture is kept in the artwork cache, like every other
// picture, so the browser never fetches it from outside. A Kid gets 404
// (kidGuard). An album with no cover stored (yet) answers 204, like the
// other picture routes, so the page shows its placeholder without a failed
// request in the console.
func (h *Handlers) albumCoverArt(w http.ResponseWriter, r *http.Request) {
	p, ok := h.albumID(w, r)
	if !ok {
		return
	}
	lib := h.lib()
	mbid := strings.ToLower(p.SourceIDs["musicbrainz"])
	if lib == nil || lib.Artwork == nil {
		http.NotFound(w, r)
		return
	}
	if mbid == "" {
		NoPicture(w)
		return
	}
	var t mediafiles.Title
	if err := database.Get(r.Context(), h.Store.Read(), "titles", p.TitleID, &t); err != nil {
		http.NotFound(w, r)
		return
	}
	// Only the metadata server's stored copy: an album it
	// has not stored yet is 204 (the page shows its placeholder) until it has.
	size := CoverSize(r)
	want := size
	if size == "250" {
		want = "500" // 250 is made here from the 500
	}
	src := h.musicArtSrc("album", mbid, want, h.PictureVersion("album", mbid))
	if src == "" {
		NoPicture(w)
		return
	}
	name := library.ArtworkName(src)
	path, ok := lib.Artwork.Path(name)
	if ok {
		lib.Artwork.Recheck(src, ServerStandIn)
	} else {
		if _, err := lib.Artwork.FetchStored(r.Context(), src, ServerStandIn); err != nil {
			NoPicture(w)
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
