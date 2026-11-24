package plugins

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"regexp"
)

// Plugin logos and developer pictures are copied once by the plugins robot
// to the plugin site (images/<id>/icon-<size>.webp and
// images/avatars/<login>-64.webp). This server fetches them and hands them
// to the page, so a browser never asks github.com or any other host.
var imagePathRe = regexp.MustCompile(`^images/(?:[a-z0-9][a-z0-9-]{0,63}/icon-(?:128|256)|avatars/[A-Za-z0-9][A-Za-z0-9-]{0,38}-64)\.webp$`)

// iconURLRe is the only icon_url a manifest may name: its own 256 px logo.
var iconURLRe = regexp.MustCompile(`^images/([a-z0-9][a-z0-9-]{0,63})/icon-256\.webp$`)

const maxImage = 256 << 10

var errNotImage = errors.New("not a WebP picture")

// image serves one robot-made picture from the plugin site.
func (h *Handlers) image(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	rel := r.PathValue("path")
	if !imagePathRe.MatchString(rel) {
		http.NotFound(w, r)
		return
	}
	data, err := h.Engine.fetchImage(r, rel)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/webp")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	_, _ = w.Write(data)
}

func (e *Engine) fetchImage(r *http.Request, rel string) ([]byte, error) {
	body, err := e.fetchBody(r.Context(), rel, maxImage)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	data, err := io.ReadAll(io.LimitReader(body, maxImage+1))
	// Only a real WebP file passes (RIFF....WEBP), whatever the site says it is.
	if err != nil || len(data) > maxImage || len(data) < 12 || !bytes.Equal(data[:4], []byte("RIFF")) || !bytes.Equal(data[8:12], []byte("WEBP")) {
		return nil, errNotImage
	}
	return data, nil
}
