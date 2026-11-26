package mediaservers

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// playbackPoster serves only a recorded Plex poster, using the saved owner
// credential server-side. The browser never supplies an upstream path.
func (h *Handlers) playbackPoster(w http.ResponseWriter, r *http.Request) {
	m, ok := h.playbackServer(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("play"), 10, 64)
	if err != nil || id < 1 || h.Linker == nil {
		http.NotFound(w, r)
		return
	}
	path, err := playbackThumb(r.Context(), h.Store.Read(), m.ID, id)
	if err != nil || !plexThumbPath.MatchString(path) {
		http.NotFound(w, r)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer cancel()
	// A Plex original can be several megabytes. Ask Plex for a card-sized
	// image before applying the response limit.
	size := url.Values{"url": {path}, "width": {"360"}, "height": {"540"}}
	var art []byte
	err = h.Linker.doStream(ctx, m, http.MethodGet, "/photo/:/transcode?"+size.Encode(), nil, 2<<20, func(body io.Reader) error {
		var err error
		art, err = io.ReadAll(body)
		return err
	})
	if err != nil {
		http.NotFound(w, r)
		return
	}
	kind := http.DetectContentType(art)
	switch kind {
	case "image/jpeg", "image/png", "image/webp", "image/gif":
	default:
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", kind)
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(art)
}
