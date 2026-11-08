package sources

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// TestHTTPProxy_RefusesNameOnlyTheProxyKnows checks that a name we cannot look up is refused instead of left to the proxy.
func TestHTTPProxy_RefusesNameOnlyTheProxyKnows(t *testing.T) {
	var srcHits atomic.Int32
	src := torznab(t, duneFeed, &srcHits)
	addr, seen := connectProxy(t, strings.TrimPrefix(src.URL, "http://"))
	pu, _ := url.Parse("http://" + addr)
	c := New(Config{URL: "http://only-at-proxy.test/api", Proxy: pu, ProxyAllowLocal: true,
		AllowLocal: true, resolve: fakeDNS(nil)})
	_, err := c.Caps(context.Background())
	var se *Error
	if !errors.As(err, &se) || se.Kind != KindBlocked || srcHits.Load() != 0 {
		t.Fatalf("name only the proxy knows: %v", err)
	}
	if _, ok := seen.Load("connect"); ok {
		t.Fatal("the proxy was asked at all")
	}
}

// TestHTTPProxy_GetsTheCheckedAddress checks that an HTTP proxy gets the address that was checked, never the name.
func TestHTTPProxy_GetsTheCheckedAddress(t *testing.T) {
	var host atomic.Value
	src := serve(t, func(w http.ResponseWriter, r *http.Request) {
		host.Store(r.Host)
		_, _ = w.Write(file(t, "caps.xml"))
	})
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(src.URL, "http://"))
	addr, seen := connectProxy(t, strings.TrimPrefix(src.URL, "http://"))
	pu, _ := url.Parse("http://" + addr)
	c := New(Config{URL: "http://pinned.test:" + port + "/api", Proxy: pu, ProxyAllowLocal: true,
		AllowLocal: true, resolve: fakeDNS(map[string][]string{"pinned.test": {"127.0.0.1"}})})
	if _, err := c.Caps(context.Background()); err != nil {
		t.Fatal(err)
	}
	// The proxy would send a name somewhere else, so it only ever gets the checked address.
	if h, _ := seen.Load("connect"); h != "127.0.0.1:"+port {
		t.Fatalf("the proxy was asked for %v, not the checked address", h)
	}
	if h := host.Load(); h != "pinned.test:"+port {
		t.Fatalf("Host header %v", h)
	}
	seen.Delete("connect")
	// Rebinding: one local answer among the others refuses the whole name.
	c = New(Config{URL: "http://pinned.test:" + port + "/api", Proxy: pu, ProxyAllowLocal: true,
		AllowLocal: true, resolve: fakeDNS(map[string][]string{"pinned.test": {"127.0.0.1", "169.254.169.254"}})})
	_, err := c.Caps(context.Background())
	var se *Error
	if !errors.As(err, &se) || se.Kind != KindBlocked {
		t.Fatalf("mixed answers: %v", err)
	}
	if _, ok := seen.Load("connect"); ok {
		t.Fatal("the proxy was asked after a refused lookup")
	}
}

// TestSOCKS5Proxy_RefusesNameOnlyTheProxyKnows checks that a SOCKS5 proxy is not handed a name we cannot look up.
func TestSOCKS5Proxy_RefusesNameOnlyTheProxyKnows(t *testing.T) {
	var srcHits atomic.Int32
	src := torznab(t, duneFeed, &srcHits)
	addr, seen := socksSeen(t, strings.TrimPrefix(src.URL, "http://"))
	pu, _ := url.Parse("socks5://" + addr)
	c := New(Config{URL: "http://only-at-proxy.test/api", Proxy: pu, ProxyAllowLocal: true,
		AllowLocal: true, resolve: fakeDNS(nil)})
	_, err := c.Caps(context.Background())
	var se *Error
	if !errors.As(err, &se) || se.Kind != KindBlocked || srcHits.Load() != 0 {
		t.Fatalf("socks name only the proxy knows: %v", err)
	}
	if _, ok := seen.Load("host"); ok {
		t.Fatal("the proxy was asked at all")
	}
	c = New(Config{URL: "http://pinned.test/api", Proxy: pu, ProxyAllowLocal: true,
		AllowLocal: true, resolve: fakeDNS(map[string][]string{"pinned.test": {"127.0.0.1"}})})
	if _, err := c.Caps(context.Background()); err != nil {
		t.Fatal(err)
	}
	// A name we look up is sent as the checked address (ATYP 1), never as the name.
	if a, _ := seen.Load("atyp"); a != byte(1) {
		t.Fatalf("address type %v", a)
	}
	if h, _ := seen.Load("host"); h != "127.0.0.1" {
		t.Fatalf("host %v", h)
	}
	// Rebinding: one local answer among the others refuses the whole name.
	c = New(Config{URL: "http://pinned.test/api", Proxy: pu, ProxyAllowLocal: true,
		AllowLocal: true, resolve: fakeDNS(map[string][]string{"pinned.test": {"127.0.0.1", "169.254.169.254"}})})
	if _, err := c.Caps(context.Background()); !errors.As(err, &se) || se.Kind != KindBlocked {
		t.Fatalf("socks mixed answers: %v", err)
	}
}

// TestHTTPSThroughProxy_ConnectsToCheckedAddressKeepingName checks that an https request tunnels to the checked address while the certificate is still checked against the name.
func TestHTTPSThroughProxy_ConnectsToCheckedAddressKeepingName(t *testing.T) {
	var sni atomic.Value
	src := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sni.Store(r.TLS.ServerName)
		_, _ = w.Write(file(t, "caps.xml"))
	}))
	src.StartTLS()
	t.Cleanup(src.Close)
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(src.URL, "https://"))
	addr, seen := connectProxy(t, strings.TrimPrefix(src.URL, "https://"))
	pu, _ := url.Parse("http://" + addr)
	c := New(Config{URL: "https://example.com:" + port + "/api", Proxy: pu, ProxyAllowLocal: true,
		AllowAddr: onlyLoopback, resolve: fakeDNS(map[string][]string{"example.com": {"127.0.0.1"}})})
	c.http.Transport.(*http.Transport).TLSClientConfig = src.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	if _, err := c.Caps(context.Background()); err != nil {
		t.Fatal(err)
	}
	if h, _ := seen.Load("connect"); h != "127.0.0.1:"+port {
		t.Fatalf("CONNECT to %v", h)
	}
	if s := sni.Load(); s != "example.com" {
		t.Fatalf("SNI %v", s)
	}
}

// fakeDNS answers only the names it holds, and every other lookup fails.
func fakeDNS(m map[string][]string) func(context.Context, string) ([]net.IP, error) {
	return func(_ context.Context, host string) ([]net.IP, error) {
		var out []net.IP
		for _, s := range m[host] {
			out = append(out, net.ParseIP(s))
		}
		if len(out) == 0 {
			return nil, errors.New("no such host")
		}
		return out, nil
	}
}

// onlyLoopback stands in for the public check so tests can reach 127.0.0.1 while every other address counts as local.
func onlyLoopback(ip net.IP) bool { return ip.IsLoopback() }

// socksSeen starts a SOCKS5 proxy without sign-in that records the address type and host it was asked for, and connects names to target.
func socksSeen(t *testing.T, target string) (string, *sync.Map) {
	t.Helper()
	seen := &sync.Map{}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				head := make([]byte, 2)
				if _, err := io.ReadFull(c, head); err != nil {
					return
				}
				_, _ = io.ReadFull(c, make([]byte, head[1]))
				_, _ = c.Write([]byte{5, 0})
				req := make([]byte, 4)
				if _, err := io.ReadFull(c, req); err != nil {
					return
				}
				seen.Store("atyp", req[3])
				var host string
				switch req[3] {
				case 1:
					ip := make([]byte, 4)
					_, _ = io.ReadFull(c, ip)
					host = net.IP(ip).String()
				case 3:
					n := make([]byte, 1)
					_, _ = io.ReadFull(c, n)
					h := make([]byte, n[0])
					_, _ = io.ReadFull(c, h)
					host = string(h)
				}
				_, _ = io.ReadFull(c, make([]byte, 2))
				seen.Store("host", host)
				up, err := net.Dial("tcp", target)
				if err != nil {
					_, _ = c.Write([]byte{5, 5, 0, 1, 0, 0, 0, 0, 0, 0})
					return
				}
				defer up.Close()
				_, _ = c.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0})
				go func() { _, _ = io.Copy(up, c) }()
				_, _ = io.Copy(c, up)
			}(c)
		}
	}()
	return ln.Addr().String(), seen
}

// connectProxy starts an HTTP proxy that tunnels CONNECT requests and remembers the target it was asked for.
func connectProxy(t *testing.T, target string) (string, *sync.Map) {
	t.Helper()
	seen := &sync.Map{}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				br := bufio.NewReader(c)
				req, err := http.ReadRequest(br)
				if err != nil || req.Method != http.MethodConnect {
					return
				}
				seen.Store("connect", req.Host)
				up, err := net.Dial("tcp", target)
				if err != nil {
					_, _ = io.WriteString(c, "HTTP/1.1 502 Bad Gateway\r\n\r\n")
					return
				}
				defer up.Close()
				_, _ = io.WriteString(c, "HTTP/1.1 200 Connection established\r\n\r\n")
				go func() { _, _ = io.Copy(up, br) }()
				_, _ = io.Copy(c, up)
			}(c)
		}
	}()
	return ln.Addr().String(), seen
}
