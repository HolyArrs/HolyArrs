package metadata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// slowTMDB answers every request after delay, as the metadata server does
// for an answer it has not kept yet.
func slowTMDB(t *testing.T, delay time.Duration) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(delay):
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":5,"title":"Standalone"}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// The test scale is 10 ms for a second: the server answers after "12 s".
// The old "10 s" attempt gave up; the new "25 s" one gets the answer.
func TestTMDBSlowAnswerFitsTheAttempt(t *testing.T) {
	srv := slowTMDB(t, 120*time.Millisecond)
	old := newTMDB(srv, fakeCred)
	old.Timeout = 100 * time.Millisecond
	if _, err := old.Movie(context.Background(), "tmdb:5"); err == nil {
		t.Fatal("a 10 s attempt should not get a 12 s answer")
	}
	p := newTMDB(srv, fakeCred)
	p.Timeout = 250 * time.Millisecond
	if m, err := p.Movie(context.Background(), "tmdb:5"); err != nil || m.Name != "Standalone" {
		t.Fatalf("slow answer: %+v %v", m, err)
	}
}

// Waiting at our own Gate does not use up the attempt's time: a "10 s"
// hold at the Gate, then a "15 s" answer, fits a "25 s" attempt.
func TestTMDBGateWaitIsNotTheServer(t *testing.T) {
	srv := slowTMDB(t, 150*time.Millisecond)
	g := &Gate{Next: http.DefaultTransport}
	_, _ = g.take(context.Background(), false) // starts the bucket
	g.mu.Lock()
	g.personHold = time.Now().Add(100 * time.Millisecond)
	g.mu.Unlock()
	p := newTMDB(srv, fakeCred)
	p.HTTP = &http.Client{Transport: g}
	p.Timeout = 250 * time.Millisecond
	start := time.Now()
	if m, err := p.Movie(context.Background(), "tmdb:5"); err != nil || m.Name != "Standalone" {
		t.Fatalf("after a Gate hold: %+v %v", m, err)
	}
	if d := time.Since(start); d < 250*time.Millisecond {
		t.Fatalf("took %v: the hold and the answer together pass the attempt time", d)
	}
}

// A server that never answers still fails in bounded time, and a Gate wait
// longer than the attempt fails too (it is held to the same time).
func TestTMDBDeadServerFailsInTime(t *testing.T) {
	srv := slowTMDB(t, time.Hour)
	p := newTMDB(srv, fakeCred)
	p.Timeout = 100 * time.Millisecond
	start := time.Now()
	if _, err := p.Movie(context.Background(), "tmdb:5"); err == nil {
		t.Fatal("a server that never answers should fail")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("took %v", d)
	}

	g := &Gate{Next: http.DefaultTransport}
	_, _ = g.take(context.Background(), false)
	g.mu.Lock()
	g.personHold = time.Now().Add(time.Hour)
	g.mu.Unlock()
	p = newTMDB(slowTMDB(t, 0), fakeCred)
	p.HTTP = &http.Client{Transport: g}
	p.Timeout = 100 * time.Millisecond
	start = time.Now()
	if _, err := p.Movie(context.Background(), "tmdb:5"); err == nil {
		t.Fatal("an hour at the Gate should fail")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("Gate wait took %v", d)
	}
}
