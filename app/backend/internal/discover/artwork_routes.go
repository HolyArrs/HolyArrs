package discover

import (
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// LibraryArtwork serves a stored Library picture by its cache name (c nil:
// not found).
func LibraryArtwork(w http.ResponseWriter, r *http.Request, c *library.ArtworkCache) {
	if c == nil {
		http.NotFound(w, r)
		return
	}
	name := r.PathValue("name")
	p, ok := c.Path(name)
	if !ok {
		http.NotFound(w, r)
		return
	}
	// ?size=250: the small copy for thumbnails (made here once, like album covers).
	if r.URL.Query().Get("size") == "250" {
		if n, sp, err := c.Smaller(name, 250); err == nil && n != name {
			// A day, not a year: the address stays when the stored picture is replaced.
			library.ServeArtworkCached(w, r, n, sp, "private, max-age=86400")
			return
		}
	}
	library.ServeArtwork(w, r, name, p)
}

// TMDBArtwork serves a TMDB picture (search results, Explore rows, title
// pages) through the artwork cache: fetched once at TMDB's own size (with
// no Referer, through the metadata server's pass-through when TMDB cannot
// be reached), kept on disk at most six months, then sent from here, so
// the browser never calls TMDB itself.
func TMDBArtwork(w http.ResponseWriter, r *http.Request, c *library.ArtworkCache, misses *ArtworkBudget) {
	if c == nil {
		http.NotFound(w, r)
		return
	}
	src, ok := library.TMDBArtworkURL(metadata.ImageBase, r.PathValue("size"), r.PathValue("file"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	ServeRemoteArtwork(w, r, c, misses, src, library.ServeArtwork)
}

// ServeRemoteArtwork sends the picture at src from the artwork cache with
// send, fetching it once on a miss (within the person's miss budget).
func ServeRemoteArtwork(w http.ResponseWriter, r *http.Request, c *library.ArtworkCache, misses *ArtworkBudget, src string, send func(http.ResponseWriter, *http.Request, string, string)) {
	name := library.ArtworkName(src)
	p, ok := c.Path(name)
	if ok {
		c.Recheck(src, nil) // a copy past TMDB's six months is fetched again
	} else {
		if !misses.MissOK(w, r) {
			return
		}
		if _, err := c.Fetch(r.Context(), src); err != nil {
			http.Error(w, "picture unavailable", http.StatusBadGateway)
			return
		}
		if p, ok = c.Path(name); !ok {
			http.NotFound(w, r)
			return
		}
	}
	send(w, r, name, p)
}
