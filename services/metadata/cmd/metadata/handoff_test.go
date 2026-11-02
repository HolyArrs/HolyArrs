package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/store"
)

// The new server waits for the store while the old one owns it, and gets
// it as soon as the old one lets go.
func TestStoreHandedOver(t *testing.T) {
	dir := t.TempDir()
	o := store.Options{MaxBytes: 1 << 30, MaxEntries: 1000}
	old, err := store.Open(dir, o)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Open(dir, o); !errors.Is(err, store.ErrOwned) {
		t.Fatalf("a second owner is refused: %v", err)
	}
	go func() {
		time.Sleep(300 * time.Millisecond)
		old.Close()
	}()
	start := time.Now()
	s, err := openStoreWaiting(dir, o, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if d := time.Since(start); d < 250*time.Millisecond || d > 2*time.Second {
		t.Fatalf("took over after %v", d)
	}
	if _, err := openStoreWaiting(dir, o, 300*time.Millisecond); !errors.Is(err, store.ErrOwned) {
		t.Fatalf("gives up after the wait: %v", err)
	}
}

// Requests reaching the new server before it is ready are held and then
// served; the health check says not ready meanwhile.
func TestStartGateHoldsRequests(t *testing.T) {
	g := newStartGate()
	w := httptest.NewRecorder()
	g.ServeHTTP(w, httptest.NewRequest("GET", "/readyz", nil))
	if w.Code != 503 {
		t.Fatalf("readyz before ready: %d", w.Code)
	}
	done := make(chan int)
	go func() {
		w := httptest.NewRecorder()
		g.ServeHTTP(w, httptest.NewRequest("GET", "/v1/music/charts", nil))
		done <- w.Code
	}()
	select {
	case <-done:
		t.Fatal("answered before ready")
	case <-time.After(100 * time.Millisecond):
	}
	g.open(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	if code := <-done; code != 204 {
		t.Fatalf("held request served %d", code)
	}
	w = httptest.NewRecorder()
	g.ServeHTTP(w, httptest.NewRequest("GET", "/readyz", nil))
	if w.Code != 204 {
		t.Fatalf("after ready the real handler answers: %d", w.Code)
	}
}
