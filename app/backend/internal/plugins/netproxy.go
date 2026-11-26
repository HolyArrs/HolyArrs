package plugins

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Proxy limits, per plugin.
const (
	proxyMaxConns    = 64
	proxyDialTimeout = 10 * time.Second
	proxyIdle        = 10 * time.Minute
	proxyHeadTimeout = 15 * time.Second
	proxyRouteWait   = 2 * time.Second
	proxyRecent      = 20
)

// Resolver looks names up for the proxy; net.DefaultResolver fits.
type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// BlockedHit is one connection the proxy refused.
type BlockedHit struct {
	Host string    `json:"host"`
	Port int       `json:"port"`
	Why  string    `json:"why"`
	At   time.Time `json:"at"`
}

// Proxy is the only way out for a locked plugin. Every plugin gets its own
// loopback listener and token; the supervisor sends each of the plugin's
// connects there, and the proxy lets through only names on the plugin's
// list, resolved here and dialed by the exact address that was checked.
type Proxy struct {
	// Resolver and Dial default to the system's; tests replace them.
	Resolver Resolver
	Dial     func(ctx context.Context, network, addr string) (net.Conn, error)
	// Own lists this host's addresses, which are always refused; nil reads
	// the interfaces.
	Own func() []netip.Addr

	mu      sync.Mutex
	plugins map[string]*ProxyHandle
}

// ProxyHandle is one plugin's registration.
type ProxyHandle struct {
	p      *Proxy
	id     string
	hosts  []string
	owner  map[string]int // owner-added host -> its extra port (0: none)
	token  string
	ln     net.Listener
	closed atomic.Bool
	conns  atomic.Int32

	mu      sync.Mutex
	routes  map[int]int // plugin socket's local port -> port it asked for
	changed chan struct{}
	blocked int
	recent  []BlockedHit
	logged  map[string]time.Time
}

// Register opens id's listener. hosts are the manifest patterns, owner the
// owner-added "host" or "host:port" entries.
func (p *Proxy) Register(id string, hosts, owner []string) (*ProxyHandle, error) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("plugin proxy: %w", err)
	}
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		ln.Close()
		return nil, err
	}
	h := &ProxyHandle{p: p, id: id, token: hex.EncodeToString(b), ln: ln, owner: map[string]int{},
		routes: map[int]int{}, changed: make(chan struct{}), logged: map[string]time.Time{}}
	for _, x := range hosts {
		h.hosts = append(h.hosts, strings.ToLower(x))
	}
	for _, o := range owner {
		if n, port, ok := ParseOwnerHost(o); ok {
			h.owner[n] = port
		}
	}
	p.mu.Lock()
	if p.plugins == nil {
		p.plugins = map[string]*ProxyHandle{}
	}
	if old := p.plugins[id]; old != nil {
		old.Close()
	}
	p.plugins[id] = h
	p.mu.Unlock()
	go h.serve()
	return h, nil
}

// Blocked returns how many connections id had refused and the last few.
func (p *Proxy) Blocked(id string) (int, []BlockedHit) {
	p.mu.Lock()
	h := p.plugins[id]
	p.mu.Unlock()
	if h == nil {
		return 0, nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.blocked, append([]BlockedHit(nil), h.recent...)
}

// Addr is the listener's address.
func (h *ProxyHandle) Addr() netip.AddrPort {
	return h.ln.Addr().(*net.TCPAddr).AddrPort()
}

// Env is the proxy settings the plugin gets. Every connect is sent to the
// proxy anyway; these let ordinary HTTP clients ask for names.
func (h *ProxyHandle) Env(self string) []string {
	u := "http://p:" + h.token + "@" + h.Addr().String()
	no := "localhost,127.0.0.1"
	if self != "" {
		if host, _, err := net.SplitHostPort(self); err == nil && host != "127.0.0.1" {
			no += "," + host
		}
	}
	var out []string
	for _, k := range []string{"HTTPS_PROXY", "HTTP_PROXY", "ALL_PROXY", "https_proxy", "http_proxy", "all_proxy"} {
		out = append(out, k+"="+u)
	}
	return append(out, "NO_PROXY="+no, "no_proxy="+no)
}

// Expect records that the plugin socket bound to local port lport was sent
// here by the supervisor and asked for port want.
func (h *ProxyHandle) Expect(lport, want int) {
	h.mu.Lock()
	h.routes[lport] = want
	close(h.changed)
	h.changed = make(chan struct{})
	h.mu.Unlock()
}

// route waits a moment for the supervisor's note on a connection.
func (h *ProxyHandle) route(lport int) (int, bool) {
	deadline := time.NewTimer(proxyRouteWait)
	defer deadline.Stop()
	for {
		h.mu.Lock()
		want, ok := h.routes[lport]
		if ok {
			delete(h.routes, lport)
		}
		ch := h.changed
		h.mu.Unlock()
		if ok {
			return want, true
		}
		select {
		case <-ch:
		case <-deadline.C:
			return 0, false
		}
	}
}

// Close stops the listener; open connections end on their own.
func (h *ProxyHandle) Close() {
	if h.closed.Swap(true) {
		return
	}
	h.ln.Close()
	h.p.mu.Lock()
	if h.p.plugins[h.id] == h {
		delete(h.p.plugins, h.id)
	}
	h.p.mu.Unlock()
}

func (h *ProxyHandle) serve() {
	for {
		c, err := h.ln.Accept()
		if err != nil {
			if h.closed.Load() {
				return
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			time.Sleep(50 * time.Millisecond)
			continue
		}
		if h.conns.Add(1) > proxyMaxConns {
			h.conns.Add(-1)
			h.block("", 0, "too many connections")
			c.Close()
			continue
		}
		go func() {
			defer h.conns.Add(-1)
			h.handle(c)
		}()
	}
}

// block counts and logs one refusal; the log says it once a minute per host.
func (h *ProxyHandle) block(host string, port int, why string) {
	now := time.Now()
	h.mu.Lock()
	h.blocked++
	h.recent = append(h.recent, BlockedHit{Host: host, Port: port, Why: why, At: now})
	if len(h.recent) > proxyRecent {
		h.recent = h.recent[len(h.recent)-proxyRecent:]
	}
	key := host + ":" + strconv.Itoa(port)
	say := now.Sub(h.logged[key]) >= time.Minute
	if say {
		h.logged[key] = now
		if len(h.logged) > 200 {
			h.logged = map[string]time.Time{key: now}
		}
	}
	h.mu.Unlock()
	if say {
		target := host
		if target == "" {
			target = "?"
		}
		if port != 0 {
			target += ":" + strconv.Itoa(port)
		}
		log.Printf("plugins: blocked %s -> %s (%s)", h.id, target, why)
	}
}

func (h *ProxyHandle) authOK(r *http.Request) bool {
	v := r.Header.Get("Proxy-Authorization")
	b64, ok := strings.CutPrefix(v, "Basic ")
	if !ok {
		return false
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil {
		return false
	}
	_, pass, _ := strings.Cut(string(raw), ":")
	return subtle.ConstantTimeCompare([]byte(pass), []byte(h.token)) == 1
}

func (h *ProxyHandle) handle(c net.Conn) {
	defer c.Close()
	lport := 0
	if a, ok := c.RemoteAddr().(*net.TCPAddr); ok {
		lport = a.Port
	}
	defer func() {
		h.mu.Lock()
		delete(h.routes, lport)
		h.mu.Unlock()
	}()
	_ = c.SetDeadline(time.Now().Add(proxyHeadTimeout))
	br := bufio.NewReaderSize(c, 16<<10+5)
	first, err := br.Peek(1)
	if err != nil {
		return
	}
	routed := func() (int, bool) { return h.route(lport) }
	if first[0] == 0x16 { // TLS record: a client that ignored the proxy settings
		want, ok := routed()
		if !ok {
			h.block("", 0, "not from the plugin")
			return
		}
		sni, err := peekSNI(br)
		if err != nil || sni == "" {
			h.block("", want, "no server name")
			return
		}
		out, why := h.dial(sni, want)
		if out == nil {
			h.block(sni, want, why)
			return
		}
		h.pipe(c, br, out, nil)
		return
	}
	r, err := http.ReadRequest(br)
	if err != nil {
		return
	}
	proxyForm := r.Method == http.MethodConnect || r.URL.IsAbs()
	trusted := proxyForm && h.authOK(r)
	if !trusted {
		// Without the token it must have come through the supervisor; a plain
		// HTTP request straight from the plugin counts only on port 80.
		want, ok := routed()
		trusted = ok && (proxyForm || want == 80)
	}
	if !trusted {
		h.block(r.Host, 0, "bad proxy token")
		if r.Method == http.MethodConnect {
			io.WriteString(c, "HTTP/1.1 407 Proxy Authentication Required\r\nContent-Length: 0\r\n\r\n")
		}
		return
	}
	if r.Method == http.MethodConnect {
		host, port, ok := splitHostPortNum(r.Host)
		if !ok {
			h.block(r.Host, 0, "bad address")
			io.WriteString(c, "HTTP/1.1 400 Bad Request\r\nContent-Length: 0\r\n\r\n")
			return
		}
		out, why := h.dial(host, port)
		if out == nil {
			h.block(host, port, why)
			io.WriteString(c, "HTTP/1.1 403 Forbidden\r\nContent-Length: 0\r\n\r\n")
			return
		}
		if _, err := io.WriteString(c, "HTTP/1.1 200 Connection established\r\n\r\n"); err != nil {
			out.Close()
			return
		}
		h.pipe(c, br, out, nil)
		return
	}
	host, port := r.Host, 80
	if r.URL.IsAbs() {
		if r.URL.Scheme != "http" {
			h.block(r.URL.Host, 0, "only http here")
			return
		}
		host = r.URL.Host
	}
	if hh, pp, ok := splitHostPortNum(host); ok {
		host, port = hh, pp
	}
	out, why := h.dial(host, port)
	if out == nil {
		h.block(host, port, why)
		io.WriteString(c, "HTTP/1.1 403 Forbidden\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
		return
	}
	r.Header.Del("Proxy-Authorization")
	r.Header.Del("Proxy-Connection")
	// One request per connection, so a later request on it cannot ride to
	// this host under another name; an upgrade (a websocket, Tailscale's
	// control protocol) keeps the connection as it is.
	if !realUpgrade(r.Header) {
		r.Close = true
	}
	h.pipe(c, br, out, r)
}

// realUpgrade reports whether a request asks for a protocol switch we know
// (websocket, Tailscale control and DERP), with Connection: Upgrade. Any
// other Upgrade header gets one request per connection like the rest.
func realUpgrade(h http.Header) bool {
	switch strings.ToLower(strings.TrimSpace(h.Get("Upgrade"))) {
	case "websocket", "tailscale-control-protocol", "derp":
	default:
		return false
	}
	for _, v := range h.Values("Connection") {
		for _, t := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(t), "upgrade") {
				return true
			}
		}
	}
	return false
}

func splitHostPortNum(s string) (string, int, bool) {
	host, ps, err := net.SplitHostPort(s)
	if err != nil {
		return "", 0, false
	}
	port, err := strconv.Atoi(ps)
	if err != nil || port <= 0 || port > 65535 {
		return "", 0, false
	}
	return host, port, true
}

// dial checks the name and port, resolves the name, refuses unsafe answers
// and dials the checked address. It returns the reason when it refuses.
func (h *ProxyHandle) dial(host string, port int) (net.Conn, string) {
	if isIPLiteral(host) {
		return nil, "addresses are not allowed, only names"
	}
	name := normalizeHost(host)
	if name == "" {
		return nil, "bad name"
	}
	ownerPort, isOwner := h.owner[name]
	switch {
	case isOwner:
		if port != 443 && port != 80 && port != ownerPort {
			return nil, "port not allowed"
		}
	case matchHost(h.hosts, name):
		if port != 443 && port != 80 {
			return nil, "port not allowed"
		}
	default:
		return nil, "not on its list"
	}
	ctx, cancel := context.WithTimeout(context.Background(), proxyDialTimeout)
	defer cancel()
	var res Resolver = net.DefaultResolver
	if h.p.Resolver != nil {
		res = h.p.Resolver
	}
	ips, err := res.LookupNetIP(ctx, "ip", name)
	if err != nil || len(ips) == 0 {
		return nil, "name not found"
	}
	own := h.p.Own
	if own == nil {
		own = ownAddrs
	}
	mine := own()
	for _, ip := range ips {
		if !safeAddr(ip, isOwner, mine) {
			return nil, "name points to a private or local address"
		}
	}
	dial := h.p.Dial
	if dial == nil {
		d := &net.Dialer{Timeout: proxyDialTimeout}
		dial = d.DialContext
	}
	var last error
	for _, ip := range ips {
		c, err := dial(ctx, "tcp", netip.AddrPortFrom(ip.Unmap(), uint16(port)).String())
		if err == nil {
			return c, ""
		}
		last = err
	}
	_ = last
	return nil, "could not connect"
}

// pipe copies both ways until either side ends or stays idle too long.
// first, when set, is a plain HTTP request to send on before the rest.
func (h *ProxyHandle) pipe(c net.Conn, br *bufio.Reader, out net.Conn, first *http.Request) {
	defer out.Close()
	_ = c.SetDeadline(time.Time{})
	if first != nil {
		if err := first.Write(out); err != nil {
			return
		}
	}
	done := make(chan struct{}, 2)
	go func() {
		copyIdle(out, io.MultiReader(io.LimitReader(br, int64(br.Buffered())), c), c)
		closeWrite(out)
		done <- struct{}{}
	}()
	go func() {
		copyIdle(c, out, out)
		closeWrite(c)
		done <- struct{}{}
	}()
	<-done
	<-done
}

func closeWrite(c net.Conn) {
	if t, ok := c.(interface{ CloseWrite() error }); ok {
		_ = t.CloseWrite()
		return
	}
	_ = c.Close()
}

// copyIdle copies src to dst, ending when src stays silent for proxyIdle.
func copyIdle(dst io.Writer, src io.Reader, conn net.Conn) {
	buf := make([]byte, 16<<10)
	for {
		_ = conn.SetReadDeadline(time.Now().Add(proxyIdle))
		n, err := src.Read(buf)
		if n > 0 {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

// peekSNI reads the server name from a TLS ClientHello without consuming it.
func peekSNI(br *bufio.Reader) (string, error) {
	hdr, err := br.Peek(5)
	if err != nil {
		return "", err
	}
	if hdr[0] != 0x16 || hdr[1] != 3 {
		return "", errors.New("not TLS")
	}
	n := int(hdr[3])<<8 | int(hdr[4])
	if n < 4 || n > 16<<10 {
		return "", errors.New("bad record")
	}
	rec, err := br.Peek(5 + n)
	if err != nil {
		return "", err
	}
	return parseSNI(rec[5:])
}

// parseSNI finds the host_name entry of a ClientHello handshake message.
func parseSNI(b []byte) (string, error) {
	bad := errors.New("bad ClientHello")
	if len(b) < 4 || b[0] != 1 {
		return "", bad
	}
	l := int(b[1])<<16 | int(b[2])<<8 | int(b[3])
	b = b[4:]
	if l < len(b) {
		b = b[:l]
	}
	skip := func(n int) bool {
		if len(b) < n {
			return false
		}
		b = b[n:]
		return true
	}
	vec := func(lenBytes int) bool {
		if len(b) < lenBytes {
			return false
		}
		n := 0
		for i := 0; i < lenBytes; i++ {
			n = n<<8 | int(b[i])
		}
		return skip(lenBytes + n)
	}
	if !skip(2+32) || !vec(1) || !vec(2) || !vec(1) {
		return "", bad
	}
	if len(b) < 2 {
		return "", nil
	}
	ext := int(b[0])<<8 | int(b[1])
	b = b[2:]
	if ext < len(b) {
		b = b[:ext]
	}
	for len(b) >= 4 {
		typ, n := int(b[0])<<8|int(b[1]), int(b[2])<<8|int(b[3])
		b = b[4:]
		if n > len(b) {
			return "", bad
		}
		data := b[:n]
		b = b[n:]
		if typ != 0 {
			continue
		}
		if len(data) < 2 {
			return "", bad
		}
		data = data[2:]
		for len(data) >= 3 {
			kind, nl := data[0], int(data[1])<<8|int(data[2])
			data = data[3:]
			if nl > len(data) {
				return "", bad
			}
			if kind == 0 {
				return string(data[:nl]), nil
			}
			data = data[nl:]
		}
	}
	return "", nil
}
