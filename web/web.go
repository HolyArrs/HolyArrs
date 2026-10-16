// Package web holds the built web app (web/app, built into web/dist) and
// serves it: real files as they are, every other page address as index.html
// so app links work on reload.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/CoderLuii/holyarrs/internal/urlbase"
)

//go:embed all:dist
var files embed.FS

// Files is the built app, rooted at its folder.
var Files, _ = fs.Sub(files, "dist")

// IsAsset reports whether p names a static file of the app that anyone may
// load before signing in (scripts, styles, fonts, the logo).
func IsAsset(p string) bool {
	return strings.HasPrefix(p, "/_app/") || strings.HasPrefix(p, "/brand/") || p == "/favicon.ico"
}

// Page writes the app's index.html, its links under the request's URL
// base (internal/urlbase), so the app runs at /holyarrs behind a proxy.
func Page(w http.ResponseWriter, r *http.Request) {
	b, err := fs.ReadFile(Files, "index.html")
	if err != nil {
		http.Error(w, "the web app is missing from this build\n", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(WithBase(b, urlbase.From(r.Context())))
}

// WithBase puts base in front of the built app's own links: scripts,
// styles, icons, fonts and the router's base. base is a checked path
// (urlbase.Clean), so it needs no escaping.
func WithBase(b []byte, base string) []byte {
	if base == "" {
		return b
	}
	s := string(b)
	for _, q := range []string{`"`, `url(`} {
		for _, p := range []string{"/_app/", "/brand/", "/favicon.ico"} {
			s = strings.ReplaceAll(s, q+p, q+base+p)
		}
	}
	s = strings.Replace(s, `base: ""`, `base: "`+base+`"`, 1)
	return []byte(s)
}

// Handler serves the app. Files under /_app/immutable/ never change name
// for new content, so browsers may keep them; everything else is checked
// again each time.
func Handler() http.Handler {
	fsrv := http.FileServerFS(Files)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := path.Clean("/" + r.URL.Path)
		if strings.HasPrefix(p, "/api/") || p == "/api" || p == "/live" {
			http.NotFound(w, r)
			return
		}
		if p == "/" || p == "/index.html" {
			Page(w, r)
			return
		}
		name := strings.TrimPrefix(p, "/")
		if st, err := fs.Stat(Files, name); err == nil && !st.IsDir() {
			if strings.HasPrefix(p, "/_app/immutable/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				w.Header().Set("Cache-Control", "no-cache")
			}
			// Styles name their fonts from the root: put the base in.
			if base := urlbase.From(r.Context()); base != "" && path.Ext(p) == ".css" {
				b, err := fs.ReadFile(Files, name)
				if err != nil {
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", "text/css; charset=utf-8")
				_, _ = w.Write(WithBase(b, base))
				return
			}
			fsrv.ServeHTTP(w, r)
			return
		}
		if IsAsset(p) || path.Ext(p) != "" {
			http.NotFound(w, r)
			return
		}
		Page(w, r)
	})
}
