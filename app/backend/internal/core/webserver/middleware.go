package webserver

import (
	"net/http"
	"strings"
)

// SecureHeaders sets the browser-hardening headers on every answer. Pages an
// outside site opens lose their handle on the app, while popups the app opens
// itself (the Plex sign-in) keep theirs.
func SecureHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hd := w.Header()
		hd.Set("X-Content-Type-Options", "nosniff")
		hd.Set("Referrer-Policy", "no-referrer")
		hd.Set("X-Frame-Options", "DENY")
		hd.Set("Cross-Origin-Opener-Policy", "same-origin-allow-popups")
		hd.Set("Content-Security-Policy", CSP(nil))
		h.ServeHTTP(w, r)
	})
}

// LimitBodies refuses API request bodies over max with 413. A path for which
// exempt returns true keeps whatever limit its own handler sets.
func LimitBodies(max int64, exempt func(path string) bool, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") && r.Body != nil {
			if exempt != nil && exempt(r.URL.Path) {
				h.ServeHTTP(w, r)
				return
			}
			if r.ContentLength > max {
				http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, max)
		}
		h.ServeHTTP(w, r)
	})
}
