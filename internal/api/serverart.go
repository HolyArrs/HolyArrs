package api

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/CoderLuii/holyarrs/internal/library"
	"github.com/CoderLuii/holyarrs/internal/metadata"
)

// The pictures the metadata server stores for movies, shows and people
// (title logos, banners and wide backgrounds; person photos) reach the
// browser through the artwork cache, like every other picture: the page
// asks /api/v1/artwork/img/{kind}/{id}/{file}[?v=], never the server or a
// picture site itself.

const serverArtPrefix = "/api/v1/artwork/img/"

var (
	serverArtID   = map[string]*regexp.Regexp{"title-logo": titleArtID, "title-banner": titleArtID, "title-wide": titleArtID, "person": regexp.MustCompile(`^[1-9]\d{0,9}$`)}
	titleArtID    = regexp.MustCompile(`^(?:movie|tv)-[1-9]\d{0,9}$`)
	serverArtFile = regexp.MustCompile(`^\d{2,4}\.(?:png|webp|jpg)$`)
)

// serverArtURL is the server's address for a stored picture, or ok false
// for a kind, id, file or version that is not one.
func serverArtURL(base, kind, id, file, v string) (string, bool) {
	re := serverArtID[kind]
	if base == "" || re == nil || !re.MatchString(id) || !serverArtFile.MatchString(file) || v != "" && !metadata.ValidPictureVersion(v) {
		return "", false
	}
	u := strings.TrimRight(base, "/") + "/" + kind + "/" + id + "/" + file
	if v != "" {
		u += "?v=" + v
	}
	return u, true
}

// serverArtLink turns a server picture address into the artwork cache's
// link ("" when it is not one).
func (a *API) serverArtLink(u string) string {
	rest, ok := strings.CutPrefix(u, strings.TrimRight(a.s.ImageBase, "/")+"/")
	if !ok || a.s.ImageBase == "" {
		return ""
	}
	path, query, _ := strings.Cut(rest, "?")
	parts := strings.Split(path, "/")
	if len(parts) != 3 {
		return ""
	}
	q, err := url.ParseQuery(query)
	if err != nil || len(q) > 1 || len(q) == 1 && len(q["v"]) != 1 {
		return ""
	}
	v := q.Get("v")
	if _, ok := serverArtURL(a.s.ImageBase, parts[0], parts[1], parts[2], v); !ok {
		return ""
	}
	link := serverArtPrefix + path
	if v != "" {
		link += "?v=" + v
	}
	return link
}

// serverArtwork answers GET /api/v1/artwork/img/{kind}/{id}/{file}: the
// server's stored picture, fetched once into the artwork cache. A picture
// the server does not have (404) or only stands in for is 404 here too; the
// page then shows its placeholder.
func (a *API) serverArtwork(w http.ResponseWriter, r *http.Request) {
	if a.s.Library == nil || a.s.Library.Artwork == nil {
		http.NotFound(w, r)
		return
	}
	src, ok := serverArtURL(a.s.ImageBase, r.PathValue("kind"), r.PathValue("id"), r.PathValue("file"), r.URL.Query().Get("v"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	c := a.s.Library.Artwork
	name := library.ArtworkName(src)
	p, ok := c.Path(name)
	if ok {
		c.Recheck(src, serverPlaceholder)
	} else {
		if !a.artworkMissOK(w, r) {
			return
		}
		if _, err := c.FetchStored(r.Context(), src, serverPlaceholder); err != nil {
			http.Error(w, "picture unavailable", http.StatusNotFound)
			return
		}
		if p, ok = c.Path(name); !ok {
			http.NotFound(w, r)
			return
		}
	}
	serveArtwork(w, r, name, p)
}
