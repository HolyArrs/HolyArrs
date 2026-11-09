package main

import (
	"bufio"
	"net"
	"net/http"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/backups"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// maxBody is the largest API request body accepted.
const maxBody = 1 << 20

// limitBodies refuses API request bodies over maxBody with 413.
func limitBodies(h http.Handler) http.Handler {
	restoreZip := func(path string) bool { return path == backups.RestorePath } // has its own, larger limit
	return webserver.LimitBodies(maxBody, restoreZip, h)
}

// newServer has no WriteTimeout: the live stream stays open. Instead each
// write gets its own deadline (stallGuard), so a client that stops reading
// frees its handler instead of holding it forever.
func newServer(addr string, h http.Handler) *http.Server {
	return &http.Server{Addr: addr, Handler: stallGuard(h), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, IdleTimeout: 120 * time.Second}
}

// writeStall is how long one write may wait on a client that does not read.
var writeStall = 60 * time.Second

// stallGuard moves the connection's write deadline forward before every
// write, so long streams that keep writing stay open while a stalled one
// fails after writeStall.
func stallGuard(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g := &stallWriter{ResponseWriter: w, rc: http.NewResponseController(w)}
		h.ServeHTTP(g, r)
		if g.wrote && !g.hijacked {
			g.extend() // the final flush after the handler returns
		}
	})
}

type stallWriter struct {
	http.ResponseWriter
	rc       *http.ResponseController
	wrote    bool
	hijacked bool
}

func (w *stallWriter) extend() {
	w.wrote = true
	_ = w.rc.SetWriteDeadline(time.Now().Add(writeStall))
}

func (w *stallWriter) Write(b []byte) (int, error) {
	w.extend()
	return w.ResponseWriter.Write(b)
}

func (w *stallWriter) Flush() {
	w.extend()
	_ = w.rc.Flush()
}

// Hijack hands over the connection with no deadline left on it.
func (w *stallWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	c, rw, err := w.rc.Hijack()
	if err == nil {
		w.hijacked = true
		_ = c.SetDeadline(time.Time{})
	}
	return c, rw, err
}

// Unwrap gives http.ResponseController the underlying writer.
func (w *stallWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
