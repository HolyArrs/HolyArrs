package music

import (
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Small helpers the Music files share (the program keeps its own copies).

const aboveLimit = metadata.AboveLimit

// decode reads a JSON body into v; false means an answer was already sent.
func decode(w http.ResponseWriter, r *http.Request, v any, help string) bool {
	return webserver.Decode(w, r, v, help)
}

// actor is the signed-in person; without sign-in (tests) it is the owner.
func actor(r *http.Request) domain.Actor {
	return webserver.ActorOr(r, domain.Actor{Kind: "user", ID: 1})
}

// mayApprove says whether the signed-in person decides asks.
func mayApprove(r *http.Request) bool {
	who, ok := webserver.From(r.Context())
	return !ok || who.Has(webserver.PermApprove)
}

func isCountry(r string) bool {
	return len(r) == 2 && r[0] >= 'A' && r[0] <= 'Z' && r[1] >= 'A' && r[1] <= 'Z'
}

// artworkLinks turns stored artwork ("poster:<cache name>") into links the
// web app can load (/api/v1/artwork/<cache name>); anything else is kept.
func artworkLinks(in []string) []string {
	out := make([]string, 0, len(in))
	for _, a := range in {
		if name, ok := strings.CutPrefix(a, "poster:"); ok && name != "" {
			a = "/api/v1/artwork/" + url.PathEscape(name)
		}
		out = append(out, a)
	}
	return out
}

// serveArtwork sends a stored picture: a stable address is kept a day and
// revalidated, a named one a year.
func serveArtwork(w http.ResponseWriter, r *http.Request, name, p string) {
	if library.StableFile(p) {
		if st, err := os.Stat(p); err == nil {
			serveArtworkCached(w, r, strings.TrimSuffix(name, filepath.Ext(name))+"-"+strconv.FormatInt(st.ModTime().Unix(), 36)+filepath.Ext(name), p, "private, max-age=86400")
			return
		}
	}
	serveArtworkCached(w, r, name, p, "private, max-age=31536000, immutable")
}

func serveArtworkCached(w http.ResponseWriter, r *http.Request, name, p, cacheControl string) {
	f, err := os.Open(p)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	hd := w.Header()
	hd.Set("Cache-Control", cacheControl)
	hd.Set("ETag", `"`+strings.TrimSuffix(name, filepath.Ext(name))+`"`)
	if ct := mime.TypeByExtension(filepath.Ext(name)); ct != "" {
		hd.Set("Content-Type", ct)
	}
	http.ServeContent(w, r, name, time.Time{}, f)
}
