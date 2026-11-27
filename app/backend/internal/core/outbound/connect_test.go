package outbound

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

// listen gives a listener on a free loopback port.
func listen(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	return ln
}

// A proxy may send the tunnel's first bytes in the same answer that opens it:
// RFC 9110 9.3.6 makes everything after the answer the tunnel. Those bytes
// belong to the caller, so they are kept rather than refused.
func TestHTTPConnectKeepsBytesSentWithTheAnswer(t *testing.T) {
	const early = "hello from the site"
	client, server := net.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = http.ReadRequest(bufio.NewReader(server))
		_, _ = io.WriteString(server, "HTTP/1.1 200 OK\r\n\r\n"+early)
		_, _ = io.Copy(io.Discard, server)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := HTTPConnect(ctx, client, "93.184.216.34:443", "", "")
	if err != nil {
		t.Fatalf("a proxy that sends the tunnel's first bytes with its answer: %v", err)
	}
	got := make([]byte, len(early))
	if _, err := io.ReadFull(conn, got); err != nil {
		t.Fatalf("reading the tunnel: %v", err)
	}
	if string(got) != early {
		t.Fatalf("the tunnel started with %q", got)
	}
	_ = conn.Close()
	<-done
}

// Gluetun's proxy answers CONNECT and only then opens the tunnel, and it
// throws away whatever it caught while opening it. A client that writes at
// once loses the start of its request, so a fresh tunnel is given a moment to
// show its first bytes before the caller writes.
func TestHTTPConnectWaitsForAProxyThatFinishesOpeningTheTunnel(t *testing.T) {
	const opening = 25 * time.Millisecond
	site := listen(t)
	heard := make(chan string, 1)
	go func() {
		c, err := site.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		_ = c.SetReadDeadline(time.Now().Add(4 * time.Second))
		b, _ := io.ReadAll(io.LimitReader(c, 5))
		heard <- string(b)
	}()
	proxy := listen(t)
	go func() {
		c, err := proxy.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		br := bufio.NewReader(c)
		if _, err := http.ReadRequest(br); err != nil {
			return
		}
		up, err := net.Dial("tcp", site.Addr().String())
		if err != nil {
			return
		}
		defer up.Close()
		_, _ = io.WriteString(c, "HTTP/1.1 200 OK\r\n\r\n")
		// The moment spent opening the tunnel, with its read taking whatever
		// the client sent in it.
		_ = c.SetReadDeadline(time.Now().Add(opening))
		_, _ = br.Peek(1)
		_ = c.SetReadDeadline(time.Time{})
		go func() { _, _ = io.Copy(up, c) }()
		_, _ = io.Copy(c, up)
	}()
	c, err := net.Dial("tcp", proxy.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	if _, err := HTTPConnect(ctx, c, site.Addr().String(), "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-heard:
		if got != "hello" {
			t.Fatalf("the site heard %q", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the start of the request never arrived: the proxy threw away what it read while opening the tunnel")
	}
}
