package downloads

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// failOnce fails the first round trip with err, then uses the real transport.
type failOnce struct {
	trips int32
	err   error
}

func (f *failOnce) RoundTrip(r *http.Request) (*http.Response, error) {
	if atomic.AddInt32(&f.trips, 1) == 1 {
		return nil, f.err
	}
	return http.DefaultTransport.RoundTrip(r)
}

func okServer(t *testing.T) *httptest.Server {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("v5.0.0")) }))
	t.Cleanup(s.Close)
	return s
}

func healthWith(t *testing.T, err error) (error, int32) {
	s := okServer(t)
	f := &failOnce{err: err}
	q := NewQbittorrentKey(s.URL, "k", &http.Client{Transport: f})
	_, herr := q.Health(context.Background())
	return herr, atomic.LoadInt32(&f.trips)
}

func opErr(e error) error {
	return &net.OpError{Op: "read", Net: "tcp", Err: os.NewSyscallError("read", e)}
}

// The reset a Linux kernel reports when qBittorrent drops a kept-alive socket.
func TestEdgeLinuxResetIsRetried(t *testing.T) {
	if err, n := healthWith(t, opErr(syscall.ECONNRESET)); err != nil || n != 2 {
		t.Fatalf("err=%v trips=%d", err, n)
	}
}

// The same reset as Windows reports it (WSAECONNRESET 10054).
func TestEdgeWindowsResetIsRetried(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows errno")
	}
	if err, n := healthWith(t, opErr(syscall.Errno(10054))); err != nil || n != 2 {
		t.Fatalf("err=%v trips=%d", err, n)
	}
}

// Broken pipe: write on a socket the peer already closed.
func TestEdgeBrokenPipeIsRetried(t *testing.T) {
	if err, n := healthWith(t, opErr(syscall.EPIPE)); err != nil || n != 2 {
		t.Fatalf("err=%v trips=%d", err, n)
	}
}

// A plain wrapped EOF (fmt-style wrap, not url.Error) still counts.
func TestEdgeWrappedEOFIsRetried(t *testing.T) {
	if err, n := healthWith(t, errors.Join(errors.New("readLoop"), errors.New("x"), errEOFLike{})); err != nil || n != 2 {
		t.Fatalf("err=%v trips=%d", err, n)
	}
}

type errEOFLike struct{}

func (errEOFLike) Error() string { return "unexpected EOF" }
func (errEOFLike) Unwrap() error { return errUnexpectedEOF }

var errUnexpectedEOF = ioUnexpectedEOF

// A timeout is never retried.
func TestEdgeTimeoutNotRetried(t *testing.T) {
	err, n := healthWith(t, &net.OpError{Op: "dial", Net: "tcp", Err: timeoutErr{}})
	if err == nil || n != 1 {
		t.Fatalf("err=%v trips=%d", err, n)
	}
}

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

// Retry also dead: exactly two trips, error reported.
func TestEdgeTwoDeadConnectionsFailAfterTwoTrips(t *testing.T) {
	var n int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&n, 1)
		c, _, _ := w.(http.Hijacker).Hijack()
		c.Close()
	}))
	t.Cleanup(s.Close)
	if _, err := NewQbittorrentKey(s.URL, "k", nil).Health(context.Background()); err == nil {
		t.Fatal("want error")
	}
	if got := atomic.LoadInt32(&n); got != 2 {
		t.Fatalf("server saw %d, want 2", got)
	}
}

// The real bug: server closes the kept-alive connection while idle, next poll must work.
func TestEdgeIdleCloseBetweenPolls(t *testing.T) {
	s := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("v5.0.0")) }))
	s.Config.IdleTimeout = 50 * time.Millisecond
	s.Start()
	t.Cleanup(s.Close)
	q := NewQbittorrentKey(s.URL, "k", nil)
	for i := 0; i < 5; i++ {
		if _, err := q.Health(context.Background()); err != nil {
			t.Fatalf("poll %d: %v", i, err)
		}
		time.Sleep(80 * time.Millisecond)
	}
}

// Retry keeps the auth header (cookie login mode keeps Referer).
func TestEdgeRetryKeepsAuth(t *testing.T) {
	var n int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&n, 1) == 1 {
			c, _, _ := w.(http.Hijacker).Hijack()
			c.Close()
			return
		}
		if r.Header.Get("Authorization") != "Bearer k" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Write([]byte("v5.0.0"))
	}))
	t.Cleanup(s.Close)
	if _, err := NewQbittorrentKey(s.URL, "k", nil).Health(context.Background()); err != nil {
		t.Fatalf("retry lost auth: %v", err)
	}
}

// Refused (qBittorrent down): retried or not, the error must come back fast.
func TestEdgeRefusedFailsFast(t *testing.T) {
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := l.Addr().String()
	l.Close()
	start := time.Now()
	if _, err := NewQbittorrentKey("http://"+addr, "k", nil).Health(context.Background()); err == nil {
		t.Fatal("want error")
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("took %v", d)
	}
}

var ioUnexpectedEOF = io.ErrUnexpectedEOF
