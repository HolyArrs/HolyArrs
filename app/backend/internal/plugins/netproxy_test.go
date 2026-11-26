package plugins

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// proxyRig is a proxy whose names resolve to fixed answers and whose dials
// all land on one local web server; dialed records what it was asked to dial.
type proxyRig struct {
	p      *Proxy
	h      *ProxyHandle
	srv    *httptest.Server
	mu     sync.Mutex
	dialed []string
	answer map[string][]netip.Addr
	calls  map[string]int
}

func (r *proxyRig) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls[host]++
	if a, ok := r.answer[host]; ok {
		return a, nil
	}
	return nil, errors.New("no such host")
}

func newProxyRig(t *testing.T, hosts, owner []string) *proxyRig {
	t.Helper()
	r := &proxyRig{answer: map[string][]netip.Addr{}, calls: map[string]int{}}
	r.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { io.WriteString(w, "ok "+req.Host) }))
	t.Cleanup(r.srv.Close)
	r.p = &Proxy{Resolver: r, Own: func() []netip.Addr { return nil }, Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
		r.mu.Lock()
		r.dialed = append(r.dialed, addr)
		r.mu.Unlock()
		var d net.Dialer
		return d.DialContext(ctx, network, r.srv.Listener.Addr().String())
	}}
	h, err := r.p.Register("t", hosts, owner)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.Close)
	r.h = h
	return r
}

// connect sends a CONNECT through the proxy and returns the status code.
func (r *proxyRig) connect(t *testing.T, target, token string) int {
	t.Helper()
	c, err := net.Dial("tcp", r.h.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(10 * time.Second))
	auth := base64.StdEncoding.EncodeToString([]byte("p:" + token))
	fmt.Fprintf(c, "CONNECT %s HTTP/1.1\r\nHost: %s\r\nProxy-Authorization: Basic %s\r\n\r\n", target, target, auth)
	resp, err := http.ReadResponse(bufio.NewReader(c), nil)
	if err != nil {
		return 0
	}
	return resp.StatusCode
}

func TestProxyConnectRules(t *testing.T) {
	r := newProxyRig(t, []string{"listed.example.com", "*.wild.example.com"}, []string{"hs.home.arpa:8080"})
	pub, lan := []netip.Addr{netip.MustParseAddr("93.184.216.34")}, []netip.Addr{netip.MustParseAddr("10.0.0.1")}
	r.answer["listed.example.com"] = pub
	r.answer["a.wild.example.com"] = pub
	r.answer["unlisted.example.com"] = pub
	r.answer["hs.home.arpa"] = lan
	tok := r.h.token
	for _, c := range []struct {
		target, token string
		want          int
	}{
		{"listed.example.com:443", tok, 200},
		{"listed.example.com:80", tok, 200},
		{"a.wild.example.com:443", tok, 200},
		{"wild.example.com:443", tok, 403},
		{"unlisted.example.com:443", tok, 403},
		{"listed.example.com:22", tok, 403},
		{"93.184.216.34:443", tok, 403},
		{"[2606:2800:220:1::1]:443", tok, 403},
		{"listed.example.com:443", "wrong", 407},
		{"hs.home.arpa:8080", tok, 200},
		{"hs.home.arpa:9090", tok, 403},
	} {
		if got := r.connect(t, c.target, c.token); got != c.want {
			t.Errorf("CONNECT %s (token ok %v) = %d, want %d", c.target, c.token == tok, got, c.want)
		}
	}
	if n, recent := r.p.Blocked("t"); n < 6 || len(recent) == 0 {
		t.Errorf("blocked count %d, recent %v", n, recent)
	}
}

func TestProxyRefusesPrivateAnswersAndRebinding(t *testing.T) {
	r := newProxyRig(t, []string{"lan.example.com", "mixed.example.com"}, nil)
	r.answer["lan.example.com"] = []netip.Addr{netip.MustParseAddr("10.0.0.1")}
	r.answer["mixed.example.com"] = []netip.Addr{netip.MustParseAddr("93.184.216.34"), netip.MustParseAddr("127.0.0.1")}
	if got := r.connect(t, "lan.example.com:443", r.h.token); got != 403 {
		t.Errorf("listed name with a LAN answer: %d, want 403", got)
	}
	if got := r.connect(t, "mixed.example.com:443", r.h.token); got != 403 {
		t.Errorf("listed name with one loopback answer: %d, want 403", got)
	}
	// Each connect resolves once and dials only the address it checked.
	r.answer["mixed.example.com"] = []netip.Addr{netip.MustParseAddr("93.184.216.34")}
	if got := r.connect(t, "mixed.example.com:443", r.h.token); got != 200 {
		t.Fatalf("public answer: %d", got)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, d := range r.dialed {
		if d != "93.184.216.34:443" {
			t.Errorf("dialed %s", d)
		}
	}
}

func TestProxyPlainHTTPAbsoluteURI(t *testing.T) {
	r := newProxyRig(t, []string{"listed.example.com"}, nil)
	r.answer["listed.example.com"] = []netip.Addr{netip.MustParseAddr("93.184.216.34")}
	pu := "http://p:" + r.h.token + "@" + r.h.Addr().String()
	c := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{Proxy: func(*http.Request) (*url.URL, error) { return url.Parse(pu) }}}
	resp, err := c.Get("http://listed.example.com/x")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(b), "listed.example.com") {
		t.Errorf("plain http through the proxy: %d %q", resp.StatusCode, b)
	}
	if resp, err := c.Get("http://other.example.com/"); err == nil && resp.StatusCode == 200 {
		t.Error("unlisted plain http went through")
	}
}

func TestProxyRawNeedsSupervisorNote(t *testing.T) {
	r := newProxyRig(t, []string{"listed.example.com"}, nil)
	r.answer["listed.example.com"] = []netip.Addr{netip.MustParseAddr("93.184.216.34")}
	// A TLS hello straight to the proxy, without the supervisor's note, is refused.
	c, err := net.Dial("tcp", r.h.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	tc := tls.Client(c, &tls.Config{ServerName: "listed.example.com", InsecureSkipVerify: true})
	tc.SetDeadline(time.Now().Add(5 * time.Second))
	if err := tc.Handshake(); err == nil {
		t.Error("TLS without a supervisor note was passed")
	}
	tc.Close()
}

func TestProxyConnectionCap(t *testing.T) {
	r := newProxyRig(t, []string{"listed.example.com"}, nil)
	var conns []net.Conn
	defer func() {
		for _, c := range conns {
			c.Close()
		}
	}()
	for i := 0; i < proxyMaxConns; i++ {
		c, err := net.Dial("tcp", r.h.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		conns = append(conns, c)
	}
	time.Sleep(200 * time.Millisecond)
	c, err := net.Dial("tcp", r.h.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err := c.Read(make([]byte, 1)); err == nil || isTimeout(err) {
		t.Errorf("connection %d was not closed at once: %v", proxyMaxConns+1, err)
	}
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

func TestParseSNI(t *testing.T) {
	// Capture a real ClientHello.
	cli, srv := net.Pipe()
	go func() {
		tls.Client(cli, &tls.Config{ServerName: "derp7.tailscale.com", InsecureSkipVerify: true}).Handshake()
		cli.Close()
	}()
	br := bufio.NewReaderSize(srv, 16<<10+5)
	name, err := peekSNI(br)
	srv.Close()
	if err != nil || name != "derp7.tailscale.com" {
		t.Fatalf("peekSNI = %q, %v", name, err)
	}
	if _, err := parseSNI([]byte{1, 0, 0, 2, 3}); err == nil {
		t.Error("short hello parsed")
	}
}

// An Upgrade header alone must not keep the connection open: a second
// request piped after it would reach the same IP under another name.
func TestProxyUnknownUpgradeIsOneRequest(t *testing.T) {
	r := newProxyRig(t, []string{"listed.example.com"}, nil)
	r.answer["listed.example.com"] = []netip.Addr{netip.MustParseAddr("93.184.216.34")}
	auth := base64.StdEncoding.EncodeToString([]byte("p:" + r.h.token))
	for _, up := range []string{"Upgrade: x\r\n", "Upgrade: websocket\r\n"} {
		c, err := net.Dial("tcp", r.h.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		c.SetDeadline(time.Now().Add(3 * time.Second))
		fmt.Fprintf(c, "GET http://listed.example.com/ HTTP/1.1\r\nHost: listed.example.com\r\n%sProxy-Authorization: Basic %s\r\n\r\nGET / HTTP/1.1\r\nHost: evil2.example.net\r\nConnection: close\r\n\r\n", up, auth)
		b, _ := io.ReadAll(bufio.NewReader(c))
		c.Close()
		if strings.Contains(string(b), "ok evil2.example.net") {
			t.Errorf("%q: a second request rode the connection", up)
		}
	}
}

func TestRealUpgrade(t *testing.T) {
	for _, c := range []struct {
		up, conn string
		want     bool
	}{
		{"websocket", "Upgrade", true},
		{"tailscale-control-protocol", "keep-alive, upgrade", true},
		{"DERP", "Upgrade", true},
		{"x", "Upgrade", false},
		{"websocket", "", false},
		{"", "Upgrade", false},
	} {
		h := http.Header{}
		if c.up != "" {
			h.Set("Upgrade", c.up)
		}
		if c.conn != "" {
			h.Set("Connection", c.conn)
		}
		if got := realUpgrade(h); got != c.want {
			t.Errorf("%q/%q = %v", c.up, c.conn, got)
		}
	}
}

// #836: every owner host name and port pair is allowed, not only the last.
func TestOwnerHostsKeepEveryPort(t *testing.T) {
	r := newProxyRig(t, nil, []string{"whisper.example.lan:9000", "whisper.example.lan:9001", "Other.lan", "bad host:x"})
	cases := []struct {
		name  string
		port  int
		owner bool
		ok    bool
	}{
		{"whisper.example.lan", 9000, true, true},
		{"whisper.example.lan", 9001, true, true},
		{"whisper.example.lan", 443, true, true},
		{"whisper.example.lan", 80, true, true},
		{"whisper.example.lan", 9002, true, false},
		{"other.lan", 443, true, true},
		{"other.lan", 9000, true, false},
		{"nothere.lan", 9000, false, false},
	}
	for _, c := range cases {
		owner, ok := r.h.ownerPort(c.name, c.port)
		if owner != c.owner || ok != c.ok {
			t.Errorf("%s:%d = owner %v ok %v, want %v %v", c.name, c.port, owner, ok, c.owner, c.ok)
		}
	}
}
