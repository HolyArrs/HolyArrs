package source

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/outside"
)

func TestOutsideBreakerSuppressesOneSourceAndLeavesAnotherRunning(t *testing.T) {
	var badHits atomic.Int32
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		badHits.Add(1)
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	t.Cleanup(bad.Close)
	badClient := New(Config{URL: bad.URL, HTTP: bad.Client(), Timeout: time.Second})
	for range 3 {
		if _, err := badClient.fetch(context.Background(), bad.URL); err == nil {
			t.Fatal("failing source returned no error")
		}
	}
	if _, err := badClient.fetch(context.Background(), bad.URL); !errors.Is(err, outside.ErrSuppressed) {
		t.Fatalf("fourth call error = %v", err)
	}
	if got := badHits.Load(); got != 3 {
		t.Fatalf("bad source network hits = %d, want 3", got)
	}

	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }))
	t.Cleanup(good.Close)
	goodClient := New(Config{URL: good.URL, HTTP: good.Client(), Timeout: time.Second})
	if body, err := goodClient.fetch(context.Background(), good.URL); err != nil || string(body) != "ok" {
		t.Fatalf("unrelated source body = %q, error = %v", body, err)
	}
}

func TestOutsideDeadlineCoversResponseBody(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		close(started)
		<-release
		_, _ = w.Write([]byte("late"))
	}))
	t.Cleanup(func() {
		close(release)
		srv.Close()
	})
	c := New(Config{URL: srv.URL, HTTP: srv.Client(), Timeout: 20 * time.Millisecond})
	_, err := c.fetch(context.Background(), srv.URL)
	<-started
	if err == nil {
		t.Fatal("body read outlived its deadline")
	}
}
