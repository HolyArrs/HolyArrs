package music

import "net/http"

// NoPicture answers a picture route that has nothing to show: 204, kept briefly.
func NoPicture(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.WriteHeader(http.StatusNoContent)
}

// ArtSize is the slot size asked for a kind: covers and photos 250, 500
// (the default, every card) or 1200 (heroes); wide backgrounds 960 (rows) or
// 1920 (heroes, the default); logos as stored.
func ArtSize(kind, size string) string {
	switch kind {
	case "wide":
		if size == "500" || size == "960" {
			return "960"
		}
		return "1920"
	case "logo":
		return "1200"
	}
	if size == "250" || size == "1200" {
		return size
	}
	return "500"
}

// StandIn is the smaller stored size that stands in for a big one not made
// yet.
var StandIn = map[string]string{"1200": "500", "1920": "960"}

// ServerPlaceholder says a metadata server picture answer is only its
// stand-in for a photo it does not have yet.
func ServerPlaceholder(h http.Header) bool {
	return h.Get("X-Metadata-Placeholder") != ""
}

// ServerStandIn says a picture answer is not the picture asked for: the
// server's placeholder, or a smaller size sent until the big one is made
// (X-Metadata-Partial); neither is ever kept.
func ServerStandIn(h http.Header) bool {
	return ServerPlaceholder(h) || h.Get("X-Metadata-Partial") != ""
}

// CoverSize reads the ?size= a cover is asked at: 500 (the default, every
// card), 250 for small thumbnails, 1200 for heroes.
func CoverSize(r *http.Request) string { return ArtSize("album", r.URL.Query().Get("size")) }
