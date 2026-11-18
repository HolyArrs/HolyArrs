package outbound

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ProxySettings is the app proxy as the owner typed it. An empty Type means
// no proxy.
type ProxySettings struct {
	Type     string // http, https or socks5
	Host     string
	Port     string
	Username string
	Password string
	// Bypass lists names (a.example.com, *.example.com), addresses and
	// ranges (10.0.0.0/8) that skip the proxy, split by commas or spaces.
	Bypass string
	// BypassLocal sends local-network addresses straight there.
	BypassLocal bool
}

// Proxy is a checked app proxy. Every call that goes out through a
// PolicyDialer uses it.
type Proxy struct {
	Type        string
	Host        string
	Port        string
	user, pass  string
	bypass      []bypassRule
	BypassLocal bool
}

type bypassRule struct {
	name   string // lower case; a leading "." matches any name below it
	ipNet  *net.IPNet
	exactI net.IP
}

var current atomic.Pointer[Proxy]

// lookup resolves names before the policy check; tests swap it.
var lookup = func(ctx context.Context, host string) ([]net.IP, error) {
	as, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	out := make([]net.IP, 0, len(as))
	for _, a := range as {
		out = append(out, a.IP)
	}
	return out, err
}

// ParseProxy checks the settings. It answers nil, nil when the proxy is off.
func ParseProxy(s ProxySettings) (*Proxy, error) {
	typ := strings.ToLower(strings.TrimSpace(s.Type))
	if typ == "" {
		return nil, nil
	}
	if typ != "http" && typ != "https" && typ != "socks5" {
		return nil, errors.New("proxy type must be http, https or socks5")
	}
	host := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(s.Host), "["), "]")
	if host == "" || len(host) > 253 || strings.ContainsAny(host, "/@?# \t") {
		return nil, errors.New("proxy host must be a name or an address")
	}
	port, err := strconv.Atoi(strings.TrimSpace(s.Port))
	if err != nil || port < 1 || port > 65535 {
		return nil, errors.New("proxy port must be between 1 and 65535")
	}
	if len(s.Username) > 255 || len(s.Password) > 255 {
		return nil, errors.New("proxy username and password must be 255 characters or fewer")
	}
	p := &Proxy{Type: typ, Host: host, Port: strconv.Itoa(port), user: s.Username, pass: s.Password, BypassLocal: s.BypassLocal}
	for _, e := range strings.FieldsFunc(s.Bypass, func(r rune) bool { return r == ',' || r == ';' || r == ' ' || r == '\n' || r == '\t' }) {
		r, err := parseBypass(e)
		if err != nil {
			return nil, err
		}
		p.bypass = append(p.bypass, r)
	}
	return p, nil
}

// SetProxy turns the app proxy on (or off with nil) for every later call.
// Open connections are closed, so kept-alive ones cannot carry on the old
// way; their clients connect again.
func SetProxy(p *Proxy) {
	current.Store(p)
	open.Range(func(k, _ any) bool {
		k.(*trackedConn).Close()
		return true
	})
}

// open holds every connection a Dialer made that is still open.
var open sync.Map

type trackedConn struct {
	net.Conn
	once sync.Once
}

func track(c net.Conn, err error) (net.Conn, error) {
	if err != nil {
		return nil, err
	}
	t := &trackedConn{Conn: c}
	open.Store(t, struct{}{})
	return t, nil
}

func (t *trackedConn) Close() error {
	t.once.Do(func() { open.Delete(t) })
	return t.Conn.Close()
}

// CurrentProxy is the proxy in use, nil when off.
func CurrentProxy() *Proxy { return current.Load() }

// UseForDefaultTransport makes Go's shared transport, which some outside
// calls use, go through the app proxy and refuse cloud metadata addresses.
// While the app proxy is off it keeps honouring HTTP_PROXY and friends.
func UseForDefaultTransport() {
	t, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return
	}
	env := t.Proxy
	t.Proxy = func(r *http.Request) (*url.URL, error) {
		if current.Load() != nil || env == nil {
			return nil, nil
		}
		return env(r)
	}
	t.DialContext = PolicyDialer(NotMetadata).DialContext
}

// String names the proxy without its password.
func (p *Proxy) String() string {
	if p == nil {
		return "off"
	}
	return p.Type + "://" + net.JoinHostPort(p.Host, p.Port)
}

// Dialer is a net.Dialer that asks the address policy about every address
// and carries the call through the app proxy when one is on. The policy is
// asked about the real destination before the proxy is reached, and the
// proxy only gets that checked address, so it cannot be used to reach a
// place the policy refuses.
type Dialer struct {
	net.Dialer
	allow func(net.IP) bool
}

// DialContext connects to addr, through the app proxy unless it is off or
// addr skips it.
func (d *Dialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return track(d.dial(ctx, network, addr))
}

func (d *Dialer) dial(ctx context.Context, network, addr string) (net.Conn, error) {
	p := current.Load()
	if p == nil || (network != "tcp" && network != "tcp4" && network != "tcp6") {
		return d.Dialer.DialContext(ctx, network, addr)
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ip, err := d.checked(ctx, host)
	if err != nil {
		return nil, &net.OpError{Op: "dial", Net: network, Err: err}
	}
	to := net.JoinHostPort(ip.String(), port)
	if p.skipsName(host) || p.skipsIP(ip) {
		return d.Dialer.DialContext(ctx, network, to)
	}
	return p.tunnel(ctx, to)
}

// Dial is DialContext without a context.
func (d *Dialer) Dial(network, addr string) (net.Conn, error) {
	return d.DialContext(context.Background(), network, addr)
}

// checked looks host up here and answers the first address the policy
// allows, like a direct dial that tries each address in turn.
func (d *Dialer) checked(ctx context.Context, host string) (net.IP, error) {
	ips := []net.IP{net.ParseIP(host)}
	if ips[0] == nil {
		got, err := lookup(ctx, host)
		if err != nil || len(got) == 0 {
			return nil, fmt.Errorf("looking up %s: %w", host, err)
		}
		ips = got
	}
	for _, ip := range ips {
		if ip != nil && d.allow(ip) {
			return ip, nil
		}
	}
	return nil, ErrAddressRefused
}

// tunnel opens a connection to the checked address to through the proxy.
// The proxy itself may live on the local network, but never on a cloud
// metadata or link-local address.
func (p *Proxy) tunnel(ctx context.Context, to string) (net.Conn, error) {
	conn, err := DirectDialer(NotMetadata).DialContext(ctx, "tcp", net.JoinHostPort(p.Host, p.Port))
	if err != nil {
		return nil, fmt.Errorf("proxy %s: %w", p, err)
	}
	if p.Type == "socks5" {
		if err := SOCKS5Connect(ctx, conn, to, p.user, p.pass); err != nil {
			conn.Close()
			return nil, fmt.Errorf("proxy %s: %w", p, err)
		}
		return conn, nil
	}
	if p.Type == "https" {
		tc := tls.Client(conn, &tls.Config{ServerName: p.Host, MinVersion: tls.VersionTLS12})
		if err := tc.HandshakeContext(ctx); err != nil {
			conn.Close()
			return nil, fmt.Errorf("proxy %s: %w", p, err)
		}
		conn = tc
	}
	if err := HTTPConnect(ctx, conn, to, p.user, p.pass); err != nil {
		conn.Close()
		return nil, fmt.Errorf("proxy %s: %w", p, err)
	}
	return conn, nil
}

func (p *Proxy) skipsName(host string) bool {
	h := strings.ToLower(strings.TrimSuffix(host, "."))
	if p.BypassLocal && (h == "localhost" || strings.HasSuffix(h, ".localhost") || strings.HasSuffix(h, ".local") || (net.ParseIP(h) == nil && !strings.Contains(h, "."))) {
		return true
	}
	for _, r := range p.bypass {
		if r.name == "" {
			continue
		}
		if h == r.name || (strings.HasPrefix(r.name, ".") && (strings.HasSuffix(h, r.name) || h == r.name[1:])) {
			return true
		}
	}
	return false
}

func (p *Proxy) skipsIP(ip net.IP) bool {
	if p.BypassLocal && !PublicOnly(ip) {
		return true
	}
	for _, r := range p.bypass {
		if (r.ipNet != nil && r.ipNet.Contains(ip)) || (r.exactI != nil && r.exactI.Equal(ip)) {
			return true
		}
	}
	return false
}

func parseBypass(e string) (bypassRule, error) {
	e = strings.ToLower(strings.TrimSpace(e))
	if strings.Contains(e, "/") {
		_, n, err := net.ParseCIDR(e)
		if err != nil {
			return bypassRule{}, fmt.Errorf("proxy bypass %q is not a valid range", e)
		}
		return bypassRule{ipNet: n}, nil
	}
	if ip := net.ParseIP(strings.Trim(e, "[]")); ip != nil {
		return bypassRule{exactI: ip}, nil
	}
	e = strings.TrimPrefix(e, "*")
	if e == "" || e == "." || len(e) > 253 || strings.ContainsAny(e, ":@?#*") {
		return bypassRule{}, fmt.Errorf("proxy bypass %q is not a name, address or range", e)
	}
	return bypassRule{name: strings.TrimSuffix(e, ".")}, nil
}

// HTTPConnect asks an http proxy on conn for a tunnel to addr.
func HTTPConnect(ctx context.Context, conn net.Conn, addr, user, pass string) error {
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	} else {
		_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	}
	req := &http.Request{Method: http.MethodConnect, URL: &url.URL{Opaque: addr}, Host: addr, Header: http.Header{}}
	if user != "" || pass != "" {
		req.Header.Set("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(user+":"+pass)))
	}
	if err := req.Write(conn); err != nil {
		return err
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New("the proxy could not connect: " + resp.Status)
	}
	if br.Buffered() > 0 {
		return errors.New("the proxy sent data before the tunnel was open")
	}
	_ = conn.SetDeadline(time.Time{})
	return nil
}

// SOCKS5Connect asks a SOCKS5 proxy to connect conn to addr (RFC 1928, with
// RFC 1929 for the name and password). An address is sent as an address and
// a name as it is, so the proxy looks it up.
func SOCKS5Connect(ctx context.Context, conn net.Conn, addr, user, pass string) error {
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	} else {
		_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	}
	defer conn.SetDeadline(time.Time{})
	host, portS, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	port, err := strconv.Atoi(portS)
	if err != nil || port < 1 || port > 65535 || host == "" || len(host) > 255 {
		return errors.New("socks5: bad address")
	}
	method := byte(0x00)
	if user != "" {
		method = 0x02
	}
	if _, err := conn.Write([]byte{5, 1, method}); err != nil {
		return err
	}
	var b [2]byte
	if _, err := io.ReadFull(conn, b[:]); err != nil {
		return err
	}
	if b[0] != 5 || b[1] != method {
		return errors.New("socks5: the proxy refused the sign-in method")
	}
	if method == 0x02 {
		if len(user) > 255 || len(pass) > 255 {
			return errors.New("socks5: name or password too long")
		}
		msg := append([]byte{1, byte(len(user))}, user...)
		msg = append(append(msg, byte(len(pass))), pass...)
		if _, err := conn.Write(msg); err != nil {
			return err
		}
		if _, err := io.ReadFull(conn, b[:]); err != nil {
			return err
		}
		if b[0] != 1 || b[1] != 0 {
			return errors.New("socks5: the proxy turned down the name or password")
		}
	}
	req := []byte{5, 1, 0}
	if ip := net.ParseIP(host); ip != nil && ip.To4() != nil {
		req = append(append(req, 1), ip.To4()...)
	} else if ip != nil {
		req = append(append(req, 4), ip.To16()...)
	} else {
		req = append(append(req, 3, byte(len(host))), host...)
	}
	req = binary.BigEndian.AppendUint16(req, uint16(port))
	if _, err := conn.Write(req); err != nil {
		return err
	}
	var head [4]byte
	if _, err := io.ReadFull(conn, head[:]); err != nil {
		return err
	}
	if head[0] != 5 {
		return errors.New("socks5: bad answer")
	}
	if head[1] != 0 {
		return errors.New("socks5: the proxy could not connect (code " + strconv.Itoa(int(head[1])) + ")")
	}
	skip := 0
	switch head[3] {
	case 1:
		skip = 4
	case 4:
		skip = 16
	case 3:
		var n [1]byte
		if _, err := io.ReadFull(conn, n[:]); err != nil {
			return err
		}
		skip = int(n[0])
	default:
		return errors.New("socks5: bad answer")
	}
	_, err = io.ReadFull(conn, make([]byte, skip+2))
	return err
}
