// Package urlbase lets HolyArrs run under a path such as /holyarrs behind
// a reverse proxy: requests under the base are served with the base taken
// off, everything else is not found, and the answers are fixed on the way
// out (redirects and cookie paths get the base back).
package urlbase

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

// Env and Flag name the setting.
const (
	Env = "HOLYARRS_URL_BASE"
)

// ErrBase refuses a base that is not a plain path.
var ErrBase = errors.New("the URL base is a path such as /holyarrs: letters, digits, '-', '_', '.' and '/' only, no '..' or '//'")

// Clean checks and normalises a base: "" or "/" is no base; otherwise it
// starts with / and does not end with one.
func Clean(base string) (string, error) {
	b := strings.TrimSpace(base)
	if b == "" || b == "/" {
		return "", nil
	}
	if !strings.HasPrefix(b, "/") {
		b = "/" + b
	}
	b = strings.TrimRight(b, "/")
	if len(b) > 100 || strings.Contains(b, "//") {
		return "", ErrBase
	}
	for _, seg := range strings.Split(b[1:], "/") {
		if seg == "" || seg == "." || seg == ".." {
			return "", ErrBase
		}
		for _, r := range seg {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.') {
				return "", ErrBase
			}
		}
	}
	return b, nil
}

type ctxKey struct{}

// From is the base the request came under ("" without one).
func From(ctx context.Context) string {
	b, _ := ctx.Value(ctxKey{}).(string)
	return b
}

// With puts base on ctx (tests and in-process callers).
func With(ctx context.Context, base string) context.Context {
	return context.WithValue(ctx, ctxKey{}, base)
}

// Handler serves h under base. Without a base it only records "" on the
// context. With one: base itself redirects to base/, a path under base is
// served with base taken off, any other path is 404.
func Handler(base string, h http.Handler) http.Handler {
	if base == "" {
		return h
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if p == base {
			http.Redirect(w, r, base+"/", http.StatusMovedPermanently)
			return
		}
		rest, ok := strings.CutPrefix(p, base+"/")
		// AI apps look for the sign-in server of base at the host's root
		// (RFC 8414: /.well-known/oauth-authorization-server<base>).
		if p == "/.well-known/oauth-authorization-server"+base {
			rest, ok = ".well-known/oauth-authorization-server", true
		}
		if !ok {
			http.NotFound(w, r)
			return
		}
		r2 := r.Clone(With(r.Context(), base))
		r2.URL.Path = "/" + rest
		if r.URL.RawPath != "" {
			if raw, ok := strings.CutPrefix(r.URL.RawPath, base+"/"); ok {
				r2.URL.RawPath = "/" + raw
			} else {
				r2.URL.RawPath = ""
			}
		}
		r2.RequestURI = r2.URL.RequestURI()
		h.ServeHTTP(&writer{ResponseWriter: w, base: base}, r2)
	})
}

// writer puts the base back on redirects to this app ("/x", never "//x"
// or another host) and on cookie paths.
type writer struct {
	http.ResponseWriter
	base  string
	wrote bool
}

func (w *writer) fix() {
	if w.wrote {
		return
	}
	w.wrote = true
	hd := w.Header()
	if loc := hd.Get("Location"); strings.HasPrefix(loc, "/") && !strings.HasPrefix(loc, "//") && !strings.HasPrefix(loc, "/\\") {
		hd.Set("Location", w.base+loc)
	}
	if cs := hd.Values("Set-Cookie"); len(cs) > 0 {
		out := make([]string, len(cs))
		for i, c := range cs {
			switch {
			case strings.HasSuffix(c, "; Path=/"):
				c = strings.TrimSuffix(c, "; Path=/") + "; Path=" + w.base + "/"
			case strings.Contains(c, "; Path=/;"):
				c = strings.Replace(c, "; Path=/;", "; Path="+w.base+"/;", 1)
			}
			out[i] = c
		}
		hd["Set-Cookie"] = out
	}
}

func (w *writer) WriteHeader(code int) {
	w.fix()
	w.ResponseWriter.WriteHeader(code)
}

func (w *writer) Write(b []byte) (int, error) {
	w.fix()
	return w.ResponseWriter.Write(b)
}

// Flush keeps the live stream (SSE) working through the wrapper.
func (w *writer) Flush() {
	w.fix()
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap lets http.ResponseController reach the real writer.
func (w *writer) Unwrap() http.ResponseWriter { return w.ResponseWriter }
