package webserver

import "net/http"

// CacheHeaders marks an answer that depends on who asks: it varies by
// sign-in, and a private one (a Kid's) is never reused without asking
// again. Others get cache ("" for none).
func CacheHeaders(w http.ResponseWriter, cache string, private bool) {
	w.Header().Set("Vary", "Cookie, Authorization")
	if private {
		cache = "private, no-cache"
	}
	if cache != "" {
		w.Header().Set("Cache-Control", cache)
	}
}
