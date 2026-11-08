package webserver

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

// URLBaseEnv names the environment variable that sets the URL base.
const URLBaseEnv = "HOLYARRS_URL_BASE"

// ErrURLBase refuses a base that is not a plain path.
var ErrURLBase = errors.New("the URL base is a path such as /holyarrs: letters, digits, '-', '_', '.' and '/' only, no '..' or '//'")

type urlBaseKey struct{}

// baseWriter puts the base back on same-site redirects and on cookie paths.
type baseWriter struct {
	http.ResponseWriter
	base  string
	wrote bool
}

// CleanURLBase checks and normalises a base. "" and "/" mean no base;
// anything else starts with / and does not end with one.
func CleanURLBase(base string) (string, error) {
	b := strings.TrimSpace(base)
	if b == "" || b == "/" {
		return "", nil
	}
	if !strings.HasPrefix(b, "/") {
		b = "/" + b
	}
	b = strings.TrimRight(b, "/")
	if len(b) > 100 || strings.Contains(b, "//") {
		return "", ErrURLBase
	}
	for _, seg := range strings.Split(b[1:], "/") {
		if seg == "" || seg == "." || seg == ".." {
			return "", ErrURLBase
		}
		for _, r := range seg {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.') {
				return "", ErrURLBase
			}
		}
	}
	return b, nil
}

// URLBaseFrom is the base the request came in under, "" when there is none.
func URLBaseFrom(ctx context.Context) string {
	b, _ := ctx.Value(urlBaseKey{}).(string)
	return b
}

// WithURLBase returns ctx carrying base, for tests and in-process callers.
func WithURLBase(ctx context.Context, base string) context.Context {
	return context.WithValue(ctx, urlBaseKey{}, base)
}

// UnderURLBase serves h under base so HolyArrs can sit behind a reverse
// proxy at a path such as /holyarrs. The base itself redirects to base/, a
// path under it is served with the base taken off, and any other path is
// not found. Exact GET and HEAD of /healthz and /readyz also answer at the
// root, because the container health check cannot know a base that was set
// by flag or in Settings. Without a base, h is returned as it is.
func UnderURLBase(base string, h http.Handler) http.Handler {
	if base == "" {
		return h
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if p == base {
			http.Redirect(w, r, base+"/", http.StatusMovedPermanently)
			return
		}
		if (p == "/healthz" || p == "/readyz") && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
			h.ServeHTTP(w, r.WithContext(WithURLBase(r.Context(), base)))
			return
		}
		rest, ok := strings.CutPrefix(p, base+"/")
		// AI apps look for the sign-in server of a base at the host root
		// (RFC 8414: /.well-known/oauth-authorization-server<base>).
		if p == "/.well-known/oauth-authorization-server"+base {
			rest, ok = ".well-known/oauth-authorization-server", true
		}
		if !ok {
			http.NotFound(w, r)
			return
		}
		r2 := r.Clone(WithURLBase(r.Context(), base))
		r2.URL.Path = "/" + rest
		if r.URL.RawPath != "" {
			if raw, ok := strings.CutPrefix(r.URL.RawPath, base+"/"); ok {
				r2.URL.RawPath = "/" + raw
			} else {
				r2.URL.RawPath = ""
			}
		}
		r2.RequestURI = r2.URL.RequestURI()
		h.ServeHTTP(&baseWriter{ResponseWriter: w, base: base}, r2)
	})
}

// WriteHeader fixes the headers, then sends them.
func (w *baseWriter) WriteHeader(code int) {
	w.fix()
	w.ResponseWriter.WriteHeader(code)
}

// Write fixes the headers on the first write.
func (w *baseWriter) Write(b []byte) (int, error) {
	w.fix()
	return w.ResponseWriter.Write(b)
}

// Flush keeps the live stream working through the wrapper.
func (w *baseWriter) Flush() {
	w.fix()
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap lets http.ResponseController reach the real writer.
func (w *baseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// fix prefixes a redirect to this app ("/x", never "//x" or another host)
// and widens cookie paths from / to the base.
func (w *baseWriter) fix() {
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
