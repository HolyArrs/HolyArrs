package library

import (
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ServeArtwork sends a cached picture as it is stored (no re-compression).
// Cache names come from the picture's address. Where the address names
// the bytes (TMDB, a versioned server address) browsers keep it a year; a
// picture from a stable address (one that may change in place) is kept a
// day and then revalidated with an ETag that follows the file.
func ServeArtwork(w http.ResponseWriter, r *http.Request, name, p string) {
	if StableFile(p) {
		if st, err := os.Stat(p); err == nil {
			ServeArtworkCached(w, r, strings.TrimSuffix(name, filepath.Ext(name))+"-"+strconv.FormatInt(st.ModTime().Unix(), 36)+filepath.Ext(name), p, "private, max-age=86400")
			return
		}
	}
	ServeArtworkCached(w, r, name, p, "private, max-age=31536000, immutable")
}

// ServeArtworkCached sends a cached picture with the given Cache-Control.
func ServeArtworkCached(w http.ResponseWriter, r *http.Request, name, p, cacheControl string) {
	f, err := os.Open(p)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	h := w.Header()
	h.Set("Cache-Control", cacheControl)
	h.Set("ETag", `"`+strings.TrimSuffix(name, filepath.Ext(name))+`"`)
	if ct := mime.TypeByExtension(filepath.Ext(name)); ct != "" {
		h.Set("Content-Type", ct)
	}
	http.ServeContent(w, r, name, time.Time{}, f)
}
