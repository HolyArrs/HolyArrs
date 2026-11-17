package metadata

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
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

// Slow answers do not count as the server being down: after three timeouts
// in a row the next call is still sent and answered. Refused or broken
// answers (5xx) still count, and the third one stops the next call.
func TestTMDBSlowIsNotDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/3/movie/1", "/3/movie/2", "/3/movie/3":
			select {
			case <-time.After(time.Second):
			case <-r.Context().Done():
				return
			}
		case "/3/movie/11", "/3/movie/12", "/3/movie/13":
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(`{"id":4,"title":"Fast"}`))
	}))
	t.Cleanup(srv.Close)
	p := newTMDB(srv, fakeCred)
	p.Timeout = 50 * time.Millisecond
	ctx := context.Background()
	for _, id := range []string{"1", "2", "3"} {
		if _, err := p.Movie(ctx, "tmdb:"+id); err == nil {
			t.Fatalf("slow %s answered", id)
		}
	}
	if m, err := p.Movie(ctx, "tmdb:4"); err != nil || m.Name != "Fast" {
		t.Fatalf("after three slow answers: %+v %v", m, err)
	}

	for _, id := range []string{"11", "12", "13"} {
		if _, err := p.Movie(ctx, "tmdb:"+id); err == nil {
			t.Fatalf("broken %s answered", id)
		}
	}
	if _, err := p.Movie(ctx, "tmdb:5"); !errors.Is(err, outbound.ErrSuppressed) {
		t.Fatalf("after three broken answers the next call should wait: %v", err)
	}
}

// A timeout says the server is slow; a refused connection still says down.
func TestTMDBSlowOrDownWords(t *testing.T) {
	p := newTMDB(slowTMDB(t, time.Second), fakeCred)
	p.Timeout = 50 * time.Millisecond
	_, err := p.Movie(context.Background(), "tmdb:5")
	if !errors.Is(err, ErrServiceSlow) || !errors.Is(err, ErrServiceUnavailable) || !strings.Contains(err.Error(), Slow) {
		t.Fatalf("timeout: %v", err)
	}
	gone := httptest.NewServer(http.NotFoundHandler())
	gone.Close()
	_, err = newTMDB(gone, fakeCred).Movie(context.Background(), "tmdb:5")
	if errors.Is(err, ErrServiceSlow) || !errors.Is(err, ErrServiceUnavailable) || !strings.Contains(err.Error(), Down) {
		t.Fatalf("refused: %v", err)
	}
	if unreached(context.DeadlineExceeded) != ErrServiceSlow || unreached(errors.New("connection refused")) != ErrServiceUnavailable {
		t.Fatal("unreached should tell slow from down")
	}
}
