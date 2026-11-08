package discover

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/music"
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

// ServerArtLink turns a server picture address (under imageBase) into the
// artwork cache's link ("" when it is not one).
func ServerArtLink(imageBase, u string) string {
	rest, ok := strings.CutPrefix(u, strings.TrimRight(imageBase, "/")+"/")
	if !ok || imageBase == "" {
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
	if _, ok := serverArtURL(imageBase, parts[0], parts[1], parts[2], v); !ok {
		return ""
	}
	link := serverArtPrefix + path
	if v != "" {
		link += "?v=" + v
	}
	return link
}

// ServerArtwork answers GET /api/v1/artwork/img/{kind}/{id}/{file}: the
// server's stored picture, fetched once into the artwork cache. A picture
// the server does not have (404) or only stands in for is 404 here too; the
// page then shows its placeholder.
// c is the artwork cache (nil: 404) and misses each person's miss budget.
func ServerArtwork(w http.ResponseWriter, r *http.Request, imageBase string, c *library.ArtworkCache, misses *ArtworkBudget) {
	if c == nil {
		http.NotFound(w, r)
		return
	}
	src, ok := serverArtURL(imageBase, r.PathValue("kind"), r.PathValue("id"), r.PathValue("file"), r.URL.Query().Get("v"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	name := library.ArtworkName(src)
	p, ok := c.Path(name)
	if ok {
		c.Recheck(src, music.ServerPlaceholder)
	} else {
		if !misses.MissOK(w, r) {
			return
		}
		if _, err := c.FetchStored(r.Context(), src, music.ServerPlaceholder); err != nil {
			http.Error(w, "picture unavailable", http.StatusNotFound)
			return
		}
		if p, ok = c.Path(name); !ok {
			http.NotFound(w, r)
			return
		}
	}
	library.ServeArtwork(w, r, name, p)
}
