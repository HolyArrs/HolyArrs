package webserver

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/HolyArrs/holyarrs/app/frontend"
)

// LoginSeedMark is where index.html takes the sign-in page's first-frame data.
const LoginSeedMark = "<!--holyarrs-signin-seed-->"

// policy is the Content-Security-Policy of every answer. script-src gets the
// hashes of the page's own inline scripts (theme, app start).
const policy = "default-src 'self'; img-src 'self' %s data: blob:; " +
	"media-src 'self' https://*.googlevideo.com blob:; frame-src https://www.youtube-nocookie.com; " +
	"connect-src 'self'; style-src 'self' 'unsafe-inline'; font-src 'self'; worker-src 'self' blob:; " +
	"object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'"

// Files is the built app, rooted at its folder.
var Files = frontend.Files

// ImageHosts are the hosts the app loads pictures from besides itself: TMDB,
// YouTube thumbnails, Wikimedia Commons and TVmaze person photos. The
// browser tests replace it once at start with a stand-in TMDB.
var ImageHosts = "https://image.tmdb.org https://i.ytimg.com https://upload.wikimedia.org https://static.tvmaze.com"

var (
	inlineScript = regexp.MustCompile(`(?s)<script>(.*?)</script>`)
	newlines     = strings.NewReplacer("\r\n", "\n", "\r", "\n")
	loginSeed    atomic.Pointer[func(*http.Request) []byte]
)

// Handler serves the built app. Files under /_app/immutable/ change name
// whenever their content changes, so browsers may keep them; everything else
// is checked again each time. A page address that is not a file gets
// index.html so links work on reload.
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
			// Styles and scripts name fonts and chunks from the root, so they
			// need the base put in.
			if base := URLBaseFrom(r.Context()); base != "" && (path.Ext(p) == ".css" || path.Ext(p) == ".js") {
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

// Page writes the app's index.html with its links under the request's URL
// base, so the app runs at /holyarrs behind a proxy.
func Page(w http.ResponseWriter, r *http.Request) {
	b, err := fs.ReadFile(Files, "index.html")
	if err != nil {
		http.Error(w, "the web app is missing from this build\n", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	b = WithBase(b, URLBaseFrom(r.Context()))
	w.Header().Set("Content-Security-Policy", CSP(b))
	b = withLoginSeed(b, r)
	_, _ = w.Write(b)
}

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

// WithBase puts base in front of the built app's own links: scripts, styles,
// icons, fonts and the router's base. base is a checked path (CleanURLBase),
// so it needs no escaping.
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

// IsAsset reports whether p names a static file of the app that anyone may
// load before signing in: scripts, styles, fonts and the logo.
func IsAsset(p string) bool {
	return strings.HasPrefix(p, "/_app/") || strings.HasPrefix(p, "/brand/") || p == "/favicon.ico"
}

// SetLoginSeed sets what fills LoginSeedMark on /login: JSON that is already
// safe inside a script element, or nil for nothing.
func SetLoginSeed(f func(*http.Request) []byte) { loginSeed.Store(&f) }

// ServeBrotli sends the build-time brotli copy of name (name.br, written by
// the frontend build) when the browser accepts brotli and the copy exists.
// It reports whether it answered.
func ServeBrotli(w http.ResponseWriter, r *http.Request, fsys fs.FS, name string) bool {
	if r.Header.Get("Range") != "" || !AcceptsEncoding(r, "br") {
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

// withLoginSeed puts the sign-in seed into the page on /login only, as a data
// block. It never runs, so the script policy stays the same.
func withLoginSeed(b []byte, r *http.Request) []byte {
	f := loginSeed.Load()
	if f == nil || !strings.HasSuffix(strings.TrimSuffix(r.URL.Path, "/"), "/login") || !bytes.Contains(b, []byte(LoginSeedMark)) {
		return b
	}
	j := (*f)(r)
	if len(j) == 0 {
		return b
	}
	tag := append(append([]byte(`<script type="application/json" id="holyarrs-signin-inline">`), j...), "</script>"...)
	return bytes.Replace(b, []byte(LoginSeedMark), tag, 1)
}
