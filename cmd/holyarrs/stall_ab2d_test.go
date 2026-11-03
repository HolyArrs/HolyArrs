package main

import (
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// A client that sends a request and never reads the answer frees the
// handler after the write stall time instead of holding it forever.
func TestStalledClientFreesHandler(t *testing.T) {
	old := writeStall
	writeStall = 300 * time.Millisecond
	defer func() { writeStall = old }()
	done := make(chan error, 1)
	chunk := []byte(strings.Repeat("x", 64<<10))
	s := newServer("", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for i := 0; i < 4096; i++ { // 256 MB, far more than socket buffers hold
			if _, err := w.Write(chunk); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}))
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = s.Serve(ln) }()
	defer s.Close()
	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if tc, ok := c.(*net.TCPConn); ok {
		_ = tc.SetReadBuffer(4096)
	}
	_, _ = c.Write([]byte("GET / HTTP/1.1\r\nHost: x\r\n\r\n"))
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("the whole answer was written to a client that never read")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("handler still blocked on a client that never reads")
	}
}
