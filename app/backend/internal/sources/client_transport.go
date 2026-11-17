package sources

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
)

// Transport is the checked way out for fetching a Source's release files: it
// uses the Source's proxy and asks its network rule about every address.
func Transport(cfg Config) http.RoundTripper {
	if cfg.AllowAddr == nil {
		cfg.AllowAddr = outbound.PublicOnly
		if cfg.AllowLocal {
			cfg.AllowAddr = outbound.NotMetadata
		}
	}
	return agentTransport{transport(cfg)}
}

// UserAgent is sent on every Source request that has none; main sets the version.
var UserAgent = "HolyArrs/dev"

// agentTransport adds the HolyArrs User-Agent, since sites behind Cloudflare
// often block Go's default one.
type agentTransport struct{ next http.RoundTripper }

func (t agentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Header.Get("User-Agent") == "" {
		req = req.Clone(req.Context())
		req.Header.Set("User-Agent", UserAgent)
	}
	return t.next.RoundTrip(req)
}

// transport dials through the network policy. With a proxy, the policy checks
// the proxy's address and the Source's name is looked up and checked here. The
// proxy only gets the checked address, so it cannot resolve the name to
// something else.
func transport(cfg Config) *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = nil
	if cfg.Proxy == nil {
		t.DialContext = outbound.PolicyDialer(cfg.AllowAddr).DialContext
		return t
	}
	allow := outbound.PublicOnly
	if cfg.ProxyAllowLocal {
		allow = outbound.NotMetadata
	}
	d := outbound.PolicyDialer(allow)
	p := *cfg.Proxy
	target := func(ctx context.Context, addr string) (string, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return "", err
		}
		ip, err := checkName(ctx, cfg, host)
		if err != nil || ip == nil {
			return addr, err
		}
		return net.JoinHostPort(ip.String(), port), nil
	}
	switch {
	case p.Scheme == "socks5":
		t.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			to, err := target(ctx, addr)
			if err != nil {
				return nil, err
			}
			conn, err := d.DialContext(ctx, "tcp", p.Host)
			if err != nil {
				return nil, err
			}
			pass, _ := p.User.Password()
			if err := socks5Connect(ctx, conn, to, p.User.Username(), pass); err != nil {
				conn.Close()
				return nil, err
			}
			return conn, nil
		}
	default:
		t.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			to, err := target(ctx, addr)
			if err != nil {
				return nil, err
			}
			return httpConnect(ctx, d, &p, to)
		}
	}
	return t
}

// checkName looks host up here and asks the policy about every answer. A name
// we cannot look up is refused, because the proxy might know it but we could
// not check where it leads.
func checkName(ctx context.Context, cfg Config, host string) (net.IP, error) {
	var ips []net.IP
	if ip := net.ParseIP(host); ip != nil {
		ips = []net.IP{ip}
	} else {
		look := cfg.resolve
		if look == nil {
			look = func(ctx context.Context, host string) ([]net.IP, error) {
				as, err := net.DefaultResolver.LookupIPAddr(ctx, host)
				out := make([]net.IP, 0, len(as))
				for _, a := range as {
					out = append(out, a.IP)
				}
				return out, err
			}
		}
		got, err := look(ctx, host)
		if err != nil || len(got) == 0 {
			return nil, fmt.Errorf("%w: the Source name could not be looked up here, so its address cannot be checked", outbound.ErrAddressRefused)
		}
		ips = got
	}
	for _, ip := range ips {
		if !cfg.AllowAddr(ip) {
			return nil, fmt.Errorf("%w: the Source address is on the local network, which this Source may not reach", outbound.ErrAddressRefused)
		}
	}
	return ips[0], nil
}

// httpConnect opens a tunnel to addr through an http or https proxy.
func httpConnect(ctx context.Context, d *net.Dialer, p *url.URL, addr string) (net.Conn, error) {
	conn, err := d.DialContext(ctx, "tcp", p.Host)
	if err != nil {
		return nil, err
	}
	if p.Scheme == "https" {
		tc := tls.Client(conn, &tls.Config{ServerName: p.Hostname(), MinVersion: tls.VersionTLS12})
		if err := tc.HandshakeContext(ctx); err != nil {
			conn.Close()
			return nil, err
		}
		conn = tc
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	} else {
		_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	}
	req := &http.Request{Method: http.MethodConnect, URL: &url.URL{Opaque: addr}, Host: addr, Header: http.Header{}}
	if p.User != nil {
		pass, _ := p.User.Password()
		req.Header.Set("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(p.User.Username()+":"+pass)))
	}
	if err := req.Write(conn); err != nil {
		conn.Close()
		return nil, err
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, req)
	if err != nil {
		conn.Close()
		return nil, err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		conn.Close()
		return nil, errors.New("the proxy could not connect: " + resp.Status)
	}
	if br.Buffered() > 0 {
		conn.Close()
		return nil, errors.New("the proxy sent data before the tunnel was open")
	}
	_ = conn.SetDeadline(time.Time{})
	return conn, nil
}

// sameHostRedirect follows a redirect only to the same host, so the key in
// the query never goes to another server.
func sameHostRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 {
		return errors.New("too many redirects")
	}
	if req.URL.Host != via[0].URL.Host || req.URL.Scheme != via[0].URL.Scheme {
		return errors.New("the Source redirected to another host or scheme; refused")
	}
	return nil
}

// feedError reads a Newznab error answer, an error root with a code and a
// description.
func feedError(body []byte) error {
	var e struct {
		XMLName     xml.Name
		Code        int    `xml:"code,attr"`
		Description string `xml:"description,attr"`
	}
	if xml.Unmarshal(body, &e) != nil || e.XMLName.Local != "error" {
		return nil
	}
	kind := KindBroken
	switch {
	case e.Code >= 100 && e.Code < 200:
		kind = KindLoginRefused
	case e.Code == 429 || e.Code == 500 || e.Code == 501:
		kind = KindRateLimited
	case e.Code >= 900:
		kind = KindDown
	}
	return &Error{Kind: kind, Msg: fmt.Sprintf("code %d: %s", e.Code, e.Description), RetryAfter: Backoff(1)}
}
