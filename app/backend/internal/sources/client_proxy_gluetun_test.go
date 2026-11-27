package sources

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// TestHTTPSThroughGluetunShapedProxy fetches an https Source through a proxy
// shaped like Gluetun's built-in one: the answer to CONNECT carries
// Transfer-Encoding chunked, the connection is hijacked to open the tunnel,
// and whatever the proxy caught while opening it is thrown away, so a client
// that writes at once loses the start of its request.
func TestHTTPSThroughGluetunShapedProxy(t *testing.T) {
	src := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(file(t, "caps.xml"))
	}))
	src.StartTLS()
	t.Cleanup(src.Close)
	target := strings.TrimPrefix(src.URL, "https://")
	_, port, _ := net.SplitHostPort(target)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	proxy := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		up, err := net.Dial("tcp", target)
		if err != nil {
			http.Error(w, "no tunnel", http.StatusBadGateway)
			return
		}
		defer up.Close()
		w.WriteHeader(http.StatusOK)
		c, brw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer c.Close()
		// The moment spent opening the tunnel, with its read keeping whatever
		// the client sent in it.
		_ = c.SetReadDeadline(time.Now().Add(25 * time.Millisecond))
		_, _ = brw.Reader.Peek(1)
		_ = c.SetReadDeadline(time.Time{})
		go func() { _, _ = io.Copy(up, c) }()
		_, _ = io.Copy(c, up)
	})}
	go func() { _ = proxy.Serve(ln) }()
	t.Cleanup(func() { _ = proxy.Close() })

	pu, _ := url.Parse("http://" + ln.Addr().String())
	c := New(Config{URL: "https://example.com:" + port + "/api", Proxy: pu, ProxyAllowLocal: true, Timeout: 5 * time.Second,
		AllowAddr: onlyLoopback, resolve: fakeDNS(map[string][]string{"example.com": {"127.0.0.1"}})})
	c.http.Transport.(*http.Transport).TLSClientConfig = src.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	if _, err := c.Caps(context.Background()); err != nil {
		t.Fatalf("a Source through a Gluetun-shaped proxy: %v", err)
	}
}
