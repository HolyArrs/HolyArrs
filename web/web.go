// Package web holds the built web app (web/app, built into web/dist) and
// serves it: real files as they are, every other page address as index.html
// so app links work on reload.
package web

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs/internal/compress"
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

// ImageHosts are where the app loads pictures from besides itself: TMDB,
// fanart.tv (logos and artist art from the metadata server),
// or a stand-in TMDB in the browser tests (set once at start).
var ImageHosts = "https://image.tmdb.org https://assets.fanart.tv"

// policy is the Content-Security-Policy of every answer; script-src gets
// the hashes of the page's own inline scripts (theme, app start).
const policy = "default-src 'self'; img-src 'self' %s data: blob:; " +
	"media-src 'self' https://*.googlevideo.com blob:; frame-src https://www.youtube-nocookie.com; " +
	"connect-src 'self'; style-src 'self' 'unsafe-inline'; font-src 'self'; worker-src 'self' blob:; " +
	"object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'"

// CSP is the policy, with script-src allowing the inline scripts of page.
func CSP(page []byte) string {
	src := "script-src 'self'"
	for _, m := range inlineScript.FindAllSubmatch(page, -1) {
		// Browsers hash the script after the HTML parser turns CR LF and lone
		// CR into LF, so a page built from a CRLF checkout still matches.
		sum := sha256.Sum256([]byte(newlines.Replace(string(m[1]))))
		src += " 'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
	}
	return strings.Replace(policy, "%s", ImageHosts, 1) + "; " + src
}

var inlineScript = regexp.MustCompile(`(?s)<script>(.*?)</script>`)

var newlines = strings.NewReplacer("\r\n", "\n", "\r", "\n")

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
	b = WithBase(b, urlbase.From(r.Context()))
	w.Header().Set("Content-Security-Policy", CSP(b))
	_, _ = w.Write(b)
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
			// Styles name their fonts from the root, and the split scripts list
			// the chunks they preload from the root: put the base in.
			if base := urlbase.From(r.Context()); base != "" && (path.Ext(p) == ".css" || path.Ext(p) == ".js") {
				b, err := fs.ReadFile(Files, name)
				if err != nil {
					http.NotFound(w, r)
					return
				}
				if path.Ext(p) == ".css" {
					w.Header().Set("Content-Type", "text/css; charset=utf-8")
				} else {
					w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
				}
				_, _ = w.Write(WithBase(b, base))
				return
			}
			if ServeBrotli(w, r, Files, name) {
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

// ServeBrotli sends name's build-time brotli copy (name.br, made by
// web/app/scripts/precompress.mjs) when the browser accepts brotli and the
// copy exists. It reports whether it answered.
func ServeBrotli(w http.ResponseWriter, r *http.Request, fsys fs.FS, name string) bool {
	if r.Header.Get("Range") != "" || !compress.Accepts(r, "br") {
		return false
	}
	b, err := fs.ReadFile(fsys, name+".br")
	if err != nil {
		return false
	}
	ct := mime.TypeByExtension(path.Ext(name))
	if ct == "" {
		ct = "application/octet-stream"
	}
	h := w.Header()
	h.Set("Content-Type", ct)
	h.Set("Content-Encoding", "br")
	if !strings.Contains(h.Get("Vary"), "Accept-Encoding") {
		h.Add("Vary", "Accept-Encoding")
	}
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(b))
	return true
}
