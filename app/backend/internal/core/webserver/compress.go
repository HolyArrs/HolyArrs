package webserver

import (
	"bufio"
	"compress/gzip"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// minSize is the smallest answer worth compressing when its length is known.
const minSize = 1024

var pool = sync.Pool{New: func() any { w, _ := gzip.NewWriterLevel(nil, gzip.DefaultCompression); return w }}

// AcceptsEncoding reports whether the request's Accept-Encoding allows enc
// (q=0 refuses it).
func AcceptsEncoding(r *http.Request, enc string) bool {
	for _, part := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		name, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		if !strings.EqualFold(strings.TrimSpace(name), enc) {
			continue
		}
		if q, ok := strings.CutPrefix(strings.TrimSpace(params), "q="); ok {
			if v, err := strconv.ParseFloat(q, 64); err == nil && v == 0 {
				return false
			}
		}
		return true
	}
	return false
}

// Compressible says a Content-Type is text-like enough to gzip.
func Compressible(ct string) bool {
	ct = strings.ToLower(strings.TrimSpace(strings.Split(ct, ";")[0]))
	switch {
	case ct == "text/event-stream":
		return false
	case strings.HasPrefix(ct, "text/"):
		return true
	case ct == "application/json", ct == "application/javascript", ct == "application/xml",
		ct == "image/svg+xml", ct == "application/manifest+json", strings.HasSuffix(ct, "+json"):
		return true
	}
	return false
}

// Handler gzips h's compressible answers.
func Compress(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Accept-Encoding")
		if r.Method == http.MethodHead || r.Header.Get("Range") != "" || r.URL.Path == "/live" || !AcceptsEncoding(r, "gzip") {
			h.ServeHTTP(w, r)
			return
		}
		gw := &gzipWriter{ResponseWriter: w}
		defer gw.close()
		h.ServeHTTP(gw, r)
	})
}

type gzipWriter struct {
	http.ResponseWriter
	gz      *gzip.Writer
	decided bool
}

func (w *gzipWriter) decide(code int) {
	if w.decided {
		return
	}
	w.decided = true
	hd := w.Header()
	n, lenErr := strconv.Atoi(hd.Get("Content-Length"))
	ct := hd.Get("Content-Type")
	if code < 200 || code == http.StatusNoContent || code == http.StatusNotModified || code == http.StatusPartialContent ||
		hd.Get("Content-Encoding") != "" || !Compressible(ct) || (lenErr == nil && n < minSize) {
		return
	}
	hd.Del("Content-Length")
	hd.Del("Accept-Ranges")
	hd.Set("Content-Encoding", "gzip")
	if et := hd.Get("ETag"); et != "" && !strings.HasPrefix(et, "W/") {
		hd.Set("ETag", "W/"+et)
	}
	gz := pool.Get().(*gzip.Writer)
	gz.Reset(w.ResponseWriter)
	w.gz = gz
}

func (w *gzipWriter) WriteHeader(code int) {
	w.decide(code)
	w.ResponseWriter.WriteHeader(code)
}

func (w *gzipWriter) Write(b []byte) (int, error) {
	if !w.decided {
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", http.DetectContentType(b))
		}
		w.WriteHeader(http.StatusOK)
	}
	if w.gz != nil {
		return w.gz.Write(b)
	}
	return w.ResponseWriter.Write(b)
}

// Flush sends what is buffered so far.
func (w *gzipWriter) Flush() {
	if w.gz != nil {
		_ = w.gz.Flush()
	}
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack lets websocket-style handlers take the connection.
func (w *gzipWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := w.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, errors.New("hijack not supported")
}

// Unwrap gives http.ResponseController the underlying gzipWriter.
func (w *gzipWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *gzipWriter) close() {
	if w.gz != nil {
		_ = w.gz.Close()
		pool.Put(w.gz)
		w.gz = nil
	}
}
