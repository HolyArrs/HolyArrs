package downloads

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// a connection dropped before any answer fails one GET; the next try must win.
// A POST (torrents/add) must never be sent twice.
func dropFirst(t *testing.T) (*httptest.Server, *int32) {
	var n int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&n, 1) == 1 {
			c, _, _ := w.(http.Hijacker).Hijack()
			c.Close()
			return
		}
		w.Write([]byte("v5.0.0"))
	}))
	t.Cleanup(s.Close)
	return s, &n
}

func TestQbitRetriesGetOnceAfterDroppedConnection(t *testing.T) {
	s, n := dropFirst(t)
	if _, err := NewQbittorrentKey(s.URL, "k", nil).Health(context.Background()); err != nil {
		t.Fatalf("health after one dropped connection: %v", err)
	}
	if *n != 2 {
		t.Fatalf("calls %d, want 2", *n)
	}
}

func TestQbitNeverRetriesPost(t *testing.T) {
	s, n := dropFirst(t)
	q := NewQbittorrentKey(s.URL, "k", nil)
	if _, err := q.call(context.Background(), http.MethodPost, "torrents/add", nil); err == nil {
		t.Fatal("want error")
	}
	if *n != 1 {
		t.Fatalf("post sent %d times", *n)
	}
}

// A hung client that times out is not asked twice, so the wait is not doubled.
func TestQbitNeverRetriesTimeout(t *testing.T) {
	var n int32
	block := make(chan struct{})
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&n, 1)
		<-block
	}))
	t.Cleanup(s.Close)
	t.Cleanup(func() { close(block) })
	q := NewQbittorrentKey(s.URL, "k", &http.Client{Timeout: 200 * time.Millisecond})
	if _, err := q.Health(context.Background()); err == nil {
		t.Fatal("want a timeout error")
	}
	if got := atomic.LoadInt32(&n); got != 1 {
		t.Fatalf("calls = %d, want 1", got)
	}
}
