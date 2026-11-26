package outbound

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeProxy is an http CONNECT or SOCKS5 proxy on 127.0.0.1 that records
// each target it is asked for and answers success without going anywhere.
type fakeProxy struct {
	ln      net.Listener
	mu      sync.Mutex
	targets []string
	auth    []string
}

func newFakeProxy(t *testing.T, socks bool) *fakeProxy {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p := &fakeProxy{ln: ln}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				if socks {
					p.socks(c)
				} else {
					p.connect(c)
				}
				_, _ = io.Copy(io.Discard, c)
			}()
		}
	}()
	return p
}

func (p *fakeProxy) connect(c net.Conn) {
	req, err := http.ReadRequest(bufio.NewReader(c))
	if err != nil || req.Method != http.MethodConnect {
		return
	}
	p.mu.Lock()
	p.targets = append(p.targets, req.Host)
	p.auth = append(p.auth, req.Header.Get("Proxy-Authorization"))
	p.mu.Unlock()
	_, _ = io.WriteString(c, "HTTP/1.1 200 OK\r\n\r\n")
}

func (p *fakeProxy) socks(c net.Conn) {
	b := make([]byte, 3)
	if _, err := io.ReadFull(c, b); err != nil {
		return
	}
	_, _ = c.Write([]byte{5, b[2]})
	if b[2] == 2 {
		h := make([]byte, 2)
		_, _ = io.ReadFull(c, h)
		u := make([]byte, h[1])
		_, _ = io.ReadFull(c, u)
		n := make([]byte, 1)
		_, _ = io.ReadFull(c, n)
		pw := make([]byte, n[0])
		_, _ = io.ReadFull(c, pw)
		p.mu.Lock()
		p.auth = append(p.auth, string(u)+":"+string(pw))
		p.mu.Unlock()
		_, _ = c.Write([]byte{1, 0})
	}
	head := make([]byte, 4)
	if _, err := io.ReadFull(c, head); err != nil {
		return
	}
	var host string
	switch head[3] {
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
	pb := make([]byte, 2)
	_, _ = io.ReadFull(c, pb)
	p.mu.Lock()
	p.targets = append(p.targets, net.JoinHostPort(host, strconv.Itoa(int(pb[0])<<8|int(pb[1]))))
	p.mu.Unlock()
	_, _ = c.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0})
}

func (p *fakeProxy) seen() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.targets...)
}

func (p *fakeProxy) addr() string { return p.ln.Addr().String() }

// useProxy turns the app proxy on for one test.
func useProxy(t *testing.T, s ProxySettings) {
	t.Helper()
	p, err := ParseProxy(s)
	if err != nil {
		t.Fatal(err)
	}
	SetProxy(p)
	t.Cleanup(func() { SetProxy(nil) })
}

func fakeLookup(t *testing.T, answers map[string][]string) {
	t.Helper()
	was := lookup
	lookup = func(_ context.Context, host string) ([]net.IP, error) {
		var out []net.IP
		for _, a := range answers[host] {
			out = append(out, net.ParseIP(a))
		}
		if len(out) == 0 {
			return nil, errors.New("no such host")
		}
		return out, nil
	}
	t.Cleanup(func() { lookup = was })
}

func hostPort(addr string) (string, string) {
	h, p, _ := net.SplitHostPort(addr)
	return h, p
}

func dialThrough(t *testing.T, allow func(net.IP) bool, addr string) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c, err := PolicyDialer(allow).DialContext(ctx, "tcp", addr)
	if err == nil {
		c.Close()
	}
	return err
}

func TestProxyOffDialsDirect(t *testing.T) {
	SetProxy(nil)
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln.Close()
	if err := dialThrough(t, NotMetadata, ln.Addr().String()); err != nil {
		t.Fatalf("direct dial with no proxy: %v", err)
	}
	if err := dialThrough(t, PublicOnly, ln.Addr().String()); !errors.Is(err, ErrAddressRefused) {
		t.Fatalf("policy still refuses local without a proxy, got %v", err)
	}
}

func TestProxyCarriesPublicCallsByCheckedAddress(t *testing.T) {
	fp := newFakeProxy(t, false)
	h, port := hostPort(fp.addr())
	useProxy(t, ProxySettings{Type: "http", Host: h, Port: port, Username: "me", Password: "pw", BypassLocal: true})
	fakeLookup(t, map[string][]string{"api.example.com": {"93.184.216.34"}})
	if err := dialThrough(t, PublicOnly, "api.example.com:443"); err != nil {
		t.Fatalf("dial through proxy: %v", err)
	}
	got := fp.seen()
	if len(got) != 1 || got[0] != "93.184.216.34:443" {
		t.Fatalf("proxy asked for %v, want the checked address only", got)
	}
	if want := "Basic " + base64.StdEncoding.EncodeToString([]byte("me:pw")); fp.auth[0] != want {
		t.Fatalf("proxy sign-in header = %q", fp.auth[0])
	}
}

func TestProxyNeverReachesRefusedDestination(t *testing.T) {
	fp := newFakeProxy(t, false)
	h, port := hostPort(fp.addr())
	useProxy(t, ProxySettings{Type: "http", Host: h, Port: port})
	fakeLookup(t, map[string][]string{"sneaky.example.com": {"10.0.0.5"}, "meta.example.com": {"169.254.169.254"}})
	for _, addr := range []string{"sneaky.example.com:80", "10.0.0.5:80", "127.0.0.1:80"} {
		if err := dialThrough(t, PublicOnly, addr); !errors.Is(err, ErrAddressRefused) {
			t.Errorf("%s through proxy = %v, want refused", addr, err)
		}
	}
	if err := dialThrough(t, NotMetadata, "meta.example.com:80"); !errors.Is(err, ErrAddressRefused) {
		t.Errorf("metadata address through proxy = %v, want refused", err)
	}
	if got := fp.seen(); len(got) != 0 {
		t.Fatalf("the proxy was asked for refused places: %v", got)
	}
}

func TestProxyBypassLocalByDefault(t *testing.T) {
	fp := newFakeProxy(t, false)
	h, port := hostPort(fp.addr())
	useProxy(t, ProxySettings{Type: "http", Host: h, Port: port, BypassLocal: true})
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln.Close()
	if err := dialThrough(t, NotMetadata, ln.Addr().String()); err != nil {
		t.Fatalf("local dial: %v", err)
	}
	if got := fp.seen(); len(got) != 0 {
		t.Fatalf("local address went through the proxy: %v", got)
	}
}

func TestProxyLocalGoesThroughWhenBypassOff(t *testing.T) {
	fp := newFakeProxy(t, false)
	h, port := hostPort(fp.addr())
	useProxy(t, ProxySettings{Type: "http", Host: h, Port: port})
	if err := dialThrough(t, NotMetadata, "192.168.1.20:8989"); err != nil {
		t.Fatalf("dial: %v", err)
	}
	if got := fp.seen(); len(got) != 1 || got[0] != "192.168.1.20:8989" {
		t.Fatalf("proxy saw %v", got)
	}
}

func TestProxyBypassList(t *testing.T) {
	fp := newFakeProxy(t, false)
	h, port := hostPort(fp.addr())
	useProxy(t, ProxySettings{Type: "http", Host: h, Port: port, Bypass: "*.skip.test, exact.test ,127.0.0.0/8"})
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln.Close()
	_, lp := hostPort(ln.Addr().String())
	fakeLookup(t, map[string][]string{"a.skip.test": {"127.0.0.1"}, "exact.test": {"127.0.0.1"}, "other.test": {"93.184.216.34"}})
	for _, host := range []string{"a.skip.test", "exact.test", "127.0.0.1"} {
		if err := dialThrough(t, NotMetadata, net.JoinHostPort(host, lp)); err != nil {
			t.Errorf("%s: %v", host, err)
		}
	}
	if err := dialThrough(t, NotMetadata, "other.test:443"); err != nil {
		t.Fatal(err)
	}
	if got := fp.seen(); len(got) != 1 || got[0] != "93.184.216.34:443" {
		t.Fatalf("proxy saw %v, want only other.test", got)
	}
}

func TestProxyOnHomeNetworkAllowedOnlyAsTheProxy(t *testing.T) {
	fp := newFakeProxy(t, false)
	h, port := hostPort(fp.addr())
	useProxy(t, ProxySettings{Type: "http", Host: h, Port: port})
	// PublicOnly refuses 127.0.0.1 as a destination, but the proxy itself may live there.
	if err := dialThrough(t, PublicOnly, "93.184.216.34:443"); err != nil {
		t.Fatalf("proxy on loopback: %v", err)
	}
	if err := dialThrough(t, PublicOnly, fp.addr()); !errors.Is(err, ErrAddressRefused) {
		t.Fatalf("the proxy address as a destination must still be refused, got %v", err)
	}
}

func TestProxyOnMetadataAddressRefused(t *testing.T) {
	useProxy(t, ProxySettings{Type: "http", Host: "169.254.169.254", Port: "80"})
	if err := dialThrough(t, PublicOnly, "93.184.216.34:443"); !errors.Is(err, ErrAddressRefused) {
		t.Fatalf("got %v, want refused", err)
	}
}

func TestProxySocks5WithSignIn(t *testing.T) {
	fp := newFakeProxy(t, true)
	h, port := hostPort(fp.addr())
	useProxy(t, ProxySettings{Type: "socks5", Host: h, Port: port, Username: "u", Password: "secret"})
	fakeLookup(t, map[string][]string{"indexer.example.com": {"93.184.216.34"}})
	if err := dialThrough(t, PublicOnly, "indexer.example.com:443"); err != nil {
		t.Fatalf("socks5 dial: %v", err)
	}
	if got := fp.seen(); len(got) != 1 || got[0] != "93.184.216.34:443" {
		t.Fatalf("socks5 proxy saw %v", got)
	}
	if fp.auth[0] != "u:secret" {
		t.Fatalf("socks5 sign-in = %q", fp.auth[0])
	}
}

// testPass is a made-up proxy sign-in for these tests.
const testPass = "hunter2-secret"

func TestProxyRefusedAnswerHidesPassword(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err == nil {
			_, _ = http.ReadRequest(bufio.NewReader(c))
			_, _ = io.WriteString(c, "HTTP/1.1 407 Proxy Authentication Required\r\n\r\n")
			c.Close()
		}
	}()
	h, port := hostPort(ln.Addr().String())
	useProxy(t, ProxySettings{Type: "http", Host: h, Port: port, Username: "u", Password: testPass})
	err := dialThrough(t, PublicOnly, "93.184.216.34:443")
	if err == nil || strings.Contains(err.Error(), testPass) {
		t.Fatalf("err = %v", err)
	}
	if s := CurrentProxy().String(); strings.Contains(s, testPass) {
		t.Fatalf("String shows the password: %s", s)
	}
}

func TestParseProxy(t *testing.T) {
	if p, err := ParseProxy(ProxySettings{}); err != nil || p != nil {
		t.Fatalf("empty type is off: %v %v", p, err)
	}
	for _, s := range []ProxySettings{
		{Type: "ftp", Host: "h", Port: "1"},
		{Type: "http", Host: "", Port: "1"},
		{Type: "http", Host: "h", Port: "0"},
		{Type: "http", Host: "h", Port: "70000"},
		{Type: "http", Host: "h/x", Port: "80"},
		{Type: "http", Host: "h", Port: "80", Bypass: "10.0.0.0/99"},
		{Type: "http", Host: "h", Port: "80", Bypass: "a@b"},
	} {
		if _, err := ParseProxy(s); err == nil {
			t.Errorf("%+v accepted", s)
		}
	}
	p, err := ParseProxy(ProxySettings{Type: "HTTPS", Host: " proxy.lan ", Port: "3128", Bypass: "a.test,,10.0.0.0/8 ; b.test"})
	if err != nil || p.Type != "https" || p.Host != "proxy.lan" || len(p.bypass) != 3 {
		t.Fatalf("got %+v %v", p, err)
	}
}

func TestDefaultTransportUsesProxy(t *testing.T) {
	tr := http.DefaultTransport.(*http.Transport)
	wasProxy, wasDial := tr.Proxy, tr.DialContext
	t.Cleanup(func() { tr.Proxy, tr.DialContext = wasProxy, wasDial })
	UseForDefaultTransport()
	fp := newFakeProxy(t, false)
	h, port := hostPort(fp.addr())
	useProxy(t, ProxySettings{Type: "http", Host: h, Port: port})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", "http://93.184.216.34/", nil)
	_, _ = http.DefaultClient.Do(req)
	if got := fp.seen(); len(got) != 1 || got[0] != "93.184.216.34:80" {
		t.Fatalf("default transport skipped the proxy: %v", got)
	}
}

func TestSetProxyClosesOpenConnections(t *testing.T) {
	SetProxy(nil)
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err == nil {
			defer c.Close()
			_, _ = io.Copy(io.Discard, c)
		}
	}()
	c, err := PolicyDialer(NotMetadata).DialContext(context.Background(), "tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	fp := newFakeProxy(t, false)
	h, port := hostPort(fp.addr())
	useProxy(t, ProxySettings{Type: "http", Host: h, Port: port})
	_ = c.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := c.Read(make([]byte, 1)); err == nil || errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("a connection from before the change stayed open: %v", err)
	}
}
