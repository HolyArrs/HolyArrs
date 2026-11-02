package main

import (
	"errors"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/store"
)

// A deploy starts the new server next to the old one (zero-downtime swap,
// releases/metadata-deploy-steps.md). The new process listens at once and
// holds the requests that reach it while it waits for the store, which the
// old process owns until it has drained its own requests and exited; then
// it opens everything and serves the held requests. Nothing is refused
// while the two hand over.

// startGate holds requests until the real handler is ready.
type startGate struct {
	ready chan struct{}
	h     atomic.Pointer[http.Handler]
}

func newStartGate() *startGate { return &startGate{ready: make(chan struct{})} }

// gateWait is the longest a request is held (a start takes about 15 s
// after the old process lets go); a released request gets a fresh write
// deadline, so being held never cuts its answer short.
const (
	gateWait   = 30 * time.Second
	gateWriter = 20 * time.Second
)

func (g *startGate) open(h http.Handler) {
	g.h.Store(&h)
	close(g.ready)
}

func (g *startGate) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	select {
	case <-g.ready:
		(*g.h.Load()).ServeHTTP(w, r)
		return
	default:
	}
	if r.URL.Path == "/readyz" {
		// Not ready until the store is ours (the health check waits).
		w.Header().Set("Retry-After", "1")
		http.Error(w, `{"ok":false,"starting":true}`, http.StatusServiceUnavailable)
		return
	}
	t := time.NewTimer(gateWait)
	defer t.Stop()
	select {
	case <-g.ready:
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(gateWriter))
		(*g.h.Load()).ServeHTTP(w, r)
	case <-r.Context().Done():
	case <-t.C:
		w.Header().Set("Retry-After", "2")
		w.Header().Set("Cache-Control", "no-store")
		http.Error(w, `{"error":"service_starting"}`, http.StatusServiceUnavailable)
	}
}

// ownerWait is how long serve waits for another process to give up the
// store.
const ownerWait = 2 * time.Minute

// openStoreWaiting opens the store, waiting while another process (the
// server being replaced) still owns it.
func openStoreWaiting(dir string, o store.Options, wait time.Duration) (*store.Store, error) {
	deadline := time.Now().Add(wait)
	for {
		s, err := store.Open(dir, o)
		if err == nil || !errors.Is(err, store.ErrOwned) || time.Now().After(deadline) {
			return s, err
		}
		time.Sleep(200 * time.Millisecond)
	}
}
