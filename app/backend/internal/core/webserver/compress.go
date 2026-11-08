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

// minCompressSize is the smallest answer worth gzipping when its length is
// known up front.
const minCompressSize = 1024

var gzipPool = sync.Pool{New: func() any { w, _ := gzip.NewWriterLevel(nil, gzip.DefaultCompression); return w }}

// gzipWriter decides on the first write whether the answer is gzipped.
type gzipWriter struct {
	http.ResponseWriter
	gz      *gzip.Writer
	decided bool
}

// Compress gzips the text-like answers of h for browsers that accept gzip.
// Ranged and HEAD requests and the live stream pass through untouched.
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

// AcceptsEncoding reports whether the request's Accept-Encoding allows enc.
// A q value of 0 refuses it.
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

// Compressible reports whether a Content-Type is text-like enough to gzip.
// The event stream is excluded: it must reach the browser as it is written.
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

// WriteHeader settles whether to gzip, then sends the header.
func (w *gzipWriter) WriteHeader(code int) {
	w.decide(code)
	w.ResponseWriter.WriteHeader(code)
}

// Write sends b, gzipped when the answer qualified.
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

// Hijack hands the connection over for handlers that take it.
func (w *gzipWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := w.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, errors.New("webserver: hijack not supported")
}

// Unwrap lets http.ResponseController reach the real writer.
func (w *gzipWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// decide turns gzip on only for answers that are small enough to be worth
// skipping, already encoded, bodiless or partial. The ETag becomes weak
// because the bytes on the wire differ from the stored ones.
func (w *gzipWriter) decide(code int) {
	if w.decided {
		return
	}
	w.decided = true
	hd := w.Header()
	n, lenErr := strconv.Atoi(hd.Get("Content-Length"))
	ct := hd.Get("Content-Type")
	if code < 200 || code == http.StatusNoContent || code == http.StatusNotModified || code == http.StatusPartialContent ||
		hd.Get("Content-Encoding") != "" || !Compressible(ct) || (lenErr == nil && n < minCompressSize) {
		return
	}
	hd.Del("Content-Length")
	hd.Del("Accept-Ranges")
	hd.Set("Content-Encoding", "gzip")
	if et := hd.Get("ETag"); et != "" && !strings.HasPrefix(et, "W/") {
		hd.Set("ETag", "W/"+et)
	}
	gz := gzipPool.Get().(*gzip.Writer)
	gz.Reset(w.ResponseWriter)
	w.gz = gz
}

func (w *gzipWriter) close() {
	if w.gz != nil {
		_ = w.gz.Close()
		gzipPool.Put(w.gz)
		w.gz = nil
	}
}
