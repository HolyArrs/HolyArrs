// Package source talks to Sources (Torznab and Newznab feeds): it reads their
// caps, searches them and turns their XML into plain results.
package source

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode"

	"github.com/CoderLuii/holyarrs/internal/outside"
)

// Error kinds match the Source pause reasons in model.md section 2.9.
const (
	KindDown         = "down"
	KindRateLimited  = "rate_limited"
	KindLoginRefused = "login_refused"
	KindBroken       = "broken_definition"
	KindBlocked      = "blocked"
)

// Error is a failed request to a Source. It never holds the URL, so the
// Source's key cannot leak into logs.
type Error struct {
	Kind string
	// Detail is the kind of block (BlockBotCheck, BlockSiteBan) when Kind is blocked by the site.
	Detail     string
	RetryAfter time.Duration
	Msg        string
	cause      error
}

func (e *Error) Error() string { return "source " + e.Kind + ": " + e.Msg }

func (e *Error) Unwrap() error { return e.cause }

// Code lets the jobs queue record the kind.
func (e *Error) Code() string { return "source_" + e.Kind }

// Config says how to reach one Source and how hard it may be used.
type Config struct {
	URL       string // base URL, with or without /api
	APIKey    string // sent as a parameter; never logged
	Protocol  string // torrent, usenet
	GapMS     int    // least time between two requests
	PerWindow int    // most requests per Window (0 means no cap)
	Window    time.Duration
	Timeout   time.Duration // default 30s
	HTTP      *http.Client
	// AllowAddr, when set, is asked about every address a request would
	// connect to; false refuses the connection. nil means PublicOnly, unless
	// AllowLocal is set.
	AllowAddr func(net.IP) bool
	// AllowLocal lets the Source reach the local network (the person's
	// per-Source choice).
	AllowLocal bool
	// Format is torznab, newznab or rss. An rss Source is read as it is: its
	// address plus Key (the address's query, kept secret) is the whole feed.
	Format string
	// Proxy, when set, carries every request (http, https or socks5). The
	// proxy address must pass the same network policy as the Source, unless
	// ProxyAllowLocal is set.
	Proxy           *url.URL
	ProxyAllowLocal bool
	// Observe, when set, is told about every call that reached the network:
	// what it was, how long it took and how it ended.
	Observe func(op string, took time.Duration, err error)
	// resolve looks names up; nil means the system resolver. Tests set it.
	resolve func(ctx context.Context, host string) ([]net.IP, error)
	// PathKeys are the passkeys URL holds in its path; never logged.
	PathKeys []string
}

// Client is a Torznab/Newznab client for one Source. It is safe for
// concurrent use; requests are spaced by the Source's limits.
type Client struct {
	cfg  Config
	http *http.Client

	mu   sync.Mutex
	last time.Time
	hits []time.Time
	caps *Caps
}

// New makes a Client.
func New(cfg Config) *Client {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.Window <= 0 {
		cfg.Window = time.Hour
	}
	if cfg.AllowAddr == nil {
		cfg.AllowAddr = PublicOnly
		if cfg.AllowLocal {
			cfg.AllowAddr = NotMetadata
		}
	}
	var h http.Client
	if cfg.HTTP != nil {
		h = *cfg.HTTP
	} else {
		h = http.Client{Timeout: cfg.Timeout, Transport: transport(cfg)}
	}
	if h.CheckRedirect == nil {
		h.CheckRedirect = sameHostRedirect
	}
	return &Client{cfg: cfg, http: &h}
}

// transport dials the Source through the network policy. With a proxy, the
// policy checks the proxy's address, and the Source's name is looked up and
// checked here. The proxy is only ever handed the checked address, so it
// cannot look the name up differently (a name only it knows, or one that
// changes between the two lookups).
func transport(cfg Config) *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = nil // the environment's proxy would hide the real address
	if cfg.Proxy == nil {
		t.DialContext = PolicyDialer(cfg.AllowAddr).DialContext
		return t
	}
	allow := PublicOnly
	if cfg.ProxyAllowLocal {
		allow = NotMetadata
	}
	d := PolicyDialer(allow)
	p := *cfg.Proxy
	// target is the checked address handed to the proxy.
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
		// A tunnel (CONNECT) to the checked address, for http and https
		// Sources alike; the transport speaks to the Source over it with
		// the Source's name (Host header and TLS SNI).
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

// checkName looks host up here and asks the policy about every answer. It
// returns the address to hand the proxy. A name that cannot be looked up here
// is refused: the proxy may know it, but we could not check where it leads.
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
			return nil, fmt.Errorf("%w: the Source name could not be looked up here, so its address cannot be checked", ErrAddressRefused)
		}
		ips = got
	}
	for _, ip := range ips {
		if !cfg.AllowAddr(ip) {
			return nil, fmt.Errorf("%w: the Source address is on the local network, which this Source may not reach", ErrAddressRefused)
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

// maxRetryAfter caps a Source's Retry-After, in seconds (one day).
const maxRetryAfter = 86400

// scrub takes the key and the proxy password out of a message, in every
// spelling a URL may give them.
func (c *Client) scrub(msg string) string {
	hide := func(v string) {
		if v != "" {
			msg = strings.ReplaceAll(msg, v, "[hidden]")
			msg = strings.ReplaceAll(msg, url.QueryEscape(v), "[hidden]")
			msg = strings.ReplaceAll(msg, url.PathEscape(v), "[hidden]")
			_, user, _ := strings.Cut(url.UserPassword("", v).String(), ":")
			msg = strings.ReplaceAll(msg, user, "[hidden]")
		}
	}
	hide(c.cfg.APIKey)
	for _, k := range c.cfg.PathKeys {
		hide(k)
	}
	if c.cfg.Proxy != nil {
		pass, _ := c.cfg.Proxy.User.Password()
		hide(pass)
	}
	return msg
}

// ErrAddressRefused is the dial error for an address the policy refuses.
var ErrAddressRefused = errors.New("address refused by the network policy")

// PolicyDialer is a dialer that asks allow about every address it connects
// to, after DNS, and refuses the ones it says no to. Alerts use it too.
func PolicyDialer(allow func(net.IP) bool) *net.Dialer {
	return &net.Dialer{Timeout: 10 * time.Second, Control: func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		if ip := net.ParseIP(host); ip == nil || !allow(ip) {
			return ErrAddressRefused
		}
		return nil
	}}
}

// metadataAddrs are cloud metadata services outside 169.254.0.0/16: the
// AWS IPv6 one and Alibaba's.
var metadataAddrs = []net.IP{net.ParseIP("fd00:ec2::254"), net.ParseIP("100.100.100.200")}

// NotMetadata allows the local network but never link-local addresses
// (169.254.0.0/16, where cloud metadata services answer, and fe80::/10) or
// the other metadata addresses. It is the policy for anything that may
// reach the local network.
func NotMetadata(ip net.IP) bool {
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	for _, m := range metadataAddrs {
		if m.Equal(ip) {
			return false
		}
	}
	return true
}

// blocked are ranges refused besides the ones net.IP names: CGNAT (also
// Tailscale), "this network", benchmarking, reserved, and the IPv6 ranges
// that carry an IPv4 address inside (NAT64 and 6to4), which could reach a
// local IPv4 host.
var blocked = func() []*net.IPNet {
	var out []*net.IPNet
	for _, c := range []string{"100.64.0.0/10", "0.0.0.0/8", "198.18.0.0/15", "240.0.0.0/4", "64:ff9b::/96", "64:ff9b:1::/48", "2002::/16"} {
		_, n, _ := net.ParseCIDR(c)
		out = append(out, n)
	}
	return out
}()

// PublicOnly refuses addresses on this machine or a local network: loopback,
// private, link-local, CGNAT, unspecified, multicast, broadcast, reserved
// and NAT64/6to4 ranges. The
// check runs on each address as it is dialled, after DNS and on every
// redirect, so a name that later resolves to a local address is refused too.
func PublicOnly(ip net.IP) bool {
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	return !(ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() || ip.Equal(net.IPv4bcast) || inBlocked(ip))
}

func inBlocked(ip net.IP) bool {
	for _, n := range blocked {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// sameHostRedirect follows a redirect only to the same host, so the key
// in the query never goes to another server.
func sameHostRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 {
		return errors.New("too many redirects")
	}
	if req.URL.Host != via[0].URL.Host || req.URL.Scheme != via[0].URL.Scheme {
		return errors.New("the Source redirected to another host or scheme; refused")
	}
	return nil
}

// Backoff is how long a Source stays paused after n failures in a row:
// one minute, doubling, at most six hours.
func Backoff(n int) time.Duration {
	d := time.Minute
	for i := 1; i < n && d < 6*time.Hour; i++ {
		d *= 2
	}
	return min(d, 6*time.Hour)
}

// wait spaces requests by the gap and refuses once the window is full.
func (c *Client) wait(ctx context.Context) error {
	c.mu.Lock()
	now := time.Now()
	if c.cfg.PerWindow > 0 {
		keep := c.hits[:0]
		for _, h := range c.hits {
			if now.Sub(h) < c.cfg.Window {
				keep = append(keep, h)
			}
		}
		c.hits = keep
		if len(c.hits) >= c.cfg.PerWindow {
			retry := c.cfg.Window - now.Sub(c.hits[0])
			c.mu.Unlock()
			return &Error{Kind: KindRateLimited, RetryAfter: retry, Msg: fmt.Sprintf("%d requests in the window already", c.cfg.PerWindow)}
		}
	}
	at := now
	if next := c.last.Add(time.Duration(c.cfg.GapMS) * time.Millisecond); next.After(now) {
		at = next
	}
	c.last = at
	if c.cfg.PerWindow > 0 {
		c.hits = append(c.hits, at)
	}
	c.mu.Unlock()
	if d := time.Until(at); d > 0 {
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-t.C:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (c *Client) get(ctx context.Context, op string, q url.Values) ([]byte, error) {
	return c.getWithObserver(ctx, op, q, c.cfg.Observe)
}

func (c *Client) getWithObserver(ctx context.Context, op string, q url.Values, observer func(string, time.Duration, error)) ([]byte, error) {
	if err := c.wait(ctx); err != nil {
		return nil, err
	}
	raw := strings.TrimRight(c.cfg.URL, "/")
	if c.cfg.Format == "rss" {
		raw = c.cfg.URL
		if c.cfg.APIKey != "" {
			raw += "?" + c.cfg.APIKey
		}
	} else {
		if c.cfg.APIKey != "" {
			q.Set("apikey", c.cfg.APIKey)
		}
		raw += "?" + q.Encode()
	}
	start := time.Now()
	body, err := c.fetch(ctx, raw)
	if observer != nil && ctx.Err() == nil {
		observer(op, time.Since(start), err)
	}
	return body, err
}

func (c *Client) fetch(ctx context.Context, raw string) ([]byte, error) {
	identity := []string{c.cfg.URL, c.cfg.APIKey}
	if c.cfg.Proxy != nil {
		identity = append(identity, c.cfg.Proxy.String())
	}
	op, err := outside.Begin(ctx, outside.Target{Kind: "source", Identity: identity})
	if err != nil {
		if errors.Is(err, outside.ErrSuppressed) {
			return nil, &Error{Kind: KindDown, Msg: err.Error(), cause: err}
		}
		return nil, err
	}
	outcome := outside.Canceled
	defer func() { op.Finish(outcome) }()
	rctx, cancel := op.AttemptContext(ctx, c.cfg.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(rctx, http.MethodGet, raw, nil)
	if err != nil {
		outcome = outside.TerminalFailure
		return nil, &Error{Kind: KindBroken, Msg: "bad Source address"}
	}
	resp, err := c.http.Do(req)
	if err != nil {
		outcome = outside.HTTPOutcome(0, err, ctx)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err // drop the URL, it holds the key
		}
		if errors.Is(err, ErrAddressRefused) {
			msg := err.Error()
			if msg == ErrAddressRefused.Error() {
				msg += ": the address is on the local network, which this Source may not reach"
			}
			return nil, &Error{Kind: KindBlocked, Msg: c.scrub(msg)}
		}
		return nil, &Error{Kind: KindDown, Msg: c.scrub(err.Error())}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if closeErr := resp.Body.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		outcome = outside.HTTPOutcome(resp.StatusCode, err, ctx)
		return nil, &Error{Kind: KindDown, Msg: "reading the answer failed"}
	}
	outcome = outside.HTTPOutcome(resp.StatusCode, nil, ctx)
	if b := botCheck(resp, body); b != "" {
		return nil, &Error{Kind: KindBlocked, Detail: b, Msg: resp.Status + ": " + blockText[b]}
	}
	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		e := &Error{Kind: KindRateLimited, Msg: "the Source asked us to slow down", RetryAfter: Backoff(1)}
		if s, err := strconv.ParseInt(strings.TrimSpace(resp.Header.Get("Retry-After")), 10, 64); err == nil && s > 0 {
			e.RetryAfter = time.Duration(min(s, maxRetryAfter)) * time.Second
		}
		return nil, e
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, &Error{Kind: KindLoginRefused, Msg: resp.Status}
	case resp.StatusCode >= 500:
		return nil, &Error{Kind: KindDown, Msg: resp.Status}
	case resp.StatusCode != http.StatusOK:
		return nil, &Error{Kind: KindBroken, Msg: resp.Status}
	}
	if err := feedError(body); err != nil {
		var se *Error
		if errors.As(err, &se) {
			clean := *se
			clean.Msg = c.scrub(clean.Msg)
			return nil, &clean
		}
		return nil, err
	}
	return body, nil
}

// feedError reads a Newznab error answer (an <error> root with code and description).
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

// Caps is what a Source says it can do.
type Caps struct {
	Search      bool
	MovieSearch bool
	TVSearch    bool
	MusicSearch bool
	Params      map[string][]string // by search type: search, movie, tv, music
	Categories  []int
	Max         int
}

// Supports says whether a search type takes a parameter.
func (c Caps) Supports(kind, param string) bool {
	for _, p := range c.Params[kind] {
		if p == param {
			return true
		}
	}
	return false
}

type capsSearch struct {
	Available string `xml:"available,attr"`
	Params    string `xml:"supportedParams,attr"`
}

func (s capsSearch) on() bool { return s.Available == "yes" }

func (s capsSearch) params() []string {
	var out []string
	for _, p := range strings.Split(s.Params, ",") {
		if p = strings.TrimSpace(strings.ToLower(p)); p != "" {
			out = append(out, p)
		}
	}
	return out
}

type capsXML struct {
	Limits struct {
		Max int `xml:"max,attr"`
	} `xml:"limits"`
	Searching struct {
		Search capsSearch `xml:"search"`
		Movie  capsSearch `xml:"movie-search"`
		TV     capsSearch `xml:"tv-search"`
		Music  capsSearch `xml:"music-search"`
		Audio  capsSearch `xml:"audio-search"`
	} `xml:"searching"`
	Categories []struct {
		ID   int `xml:"id,attr"`
		Subs []struct {
			ID int `xml:"id,attr"`
		} `xml:"subcat"`
	} `xml:"categories>category"`
}

// Caps reads (and remembers) what the Source can do.
func (c *Client) Caps(ctx context.Context) (Caps, error) {
	body, err := c.get(ctx, "caps", url.Values{"t": {"caps"}})
	if err != nil {
		return Caps{}, err
	}
	var x capsXML
	if err := xml.Unmarshal(body, &x); err != nil {
		return Caps{}, &Error{Kind: KindBroken, Msg: "caps are not valid XML: " + err.Error()}
	}
	caps := Caps{
		Search: x.Searching.Search.on(), MovieSearch: x.Searching.Movie.on(), TVSearch: x.Searching.TV.on(),
		MusicSearch: x.Searching.Music.on() || x.Searching.Audio.on(),
		Params: map[string][]string{"search": x.Searching.Search.params(), "movie": x.Searching.Movie.params(), "tv": x.Searching.TV.params(),
			"music": append(x.Searching.Music.params(), x.Searching.Audio.params()...)},
		Max: x.Limits.Max,
	}
	for _, cat := range x.Categories {
		caps.Categories = append(caps.Categories, cat.ID)
		for _, s := range cat.Subs {
			caps.Categories = append(caps.Categories, s.ID)
		}
	}
	c.mu.Lock()
	c.caps = &caps
	c.mu.Unlock()
	return caps, nil
}

// Query is one movie search, or with TV set one show search.
type Query struct {
	Title      string
	Year       int
	IMDb       string // "tt" and digits
	TMDb       int
	Categories []int
	// Shows: TV makes it a tv search. Season 0 with no Episode asks for
	// every season; Episode is a number, or "MM/DD" for a daily show (its
	// Season is then the year). Words replaces the numbers for a Source
	// with only a text search ("S01E02", "S01", "2026 03 14", "112").
	TV      bool
	Season  int
	Episode string
	Words   string
	TVDB    int
	TVMaze  int
	// Music makes it an album search: by artist and album where the Source
	// takes them, else a text search of both (stage 3.3.3).
	Music  bool
	Artist string
	Album  string
}

// Item is one search result, before parsing and judging.
type Item struct {
	Title     string
	GUID      string
	Link      string
	Size      int64
	Protocol  string
	Seeders   *int
	Peers     *int
	Published time.Time
	InfoHash  string
	Files     int // how many files the Source says it holds (0: not said)
}

type rssXML struct {
	Items []struct {
		Title     string `xml:"title"`
		GUID      string `xml:"guid"`
		Link      string `xml:"link"`
		Size      int64  `xml:"size"`
		PubDate   string `xml:"pubDate"`
		Enclosure struct {
			URL    string `xml:"url,attr"`
			Length int64  `xml:"length,attr"`
			Type   string `xml:"type,attr"`
		} `xml:"enclosure"`
		Attrs []struct {
			Name  string `xml:"name,attr"`
			Value string `xml:"value,attr"`
		} `xml:"attr"`
	} `xml:"channel>item"`
}

func textQuery(q Query) string {
	words := strings.FieldsFunc(q.Title, func(r rune) bool {
		return r != '\'' && !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	if q.Year > 0 {
		words = append(words, strconv.Itoa(q.Year))
	}
	return strings.Join(words, " ")
}

// Search runs a movie search. It uses the ids the Source supports and falls
// back to a text search by title and year.
func (c *Client) Search(ctx context.Context, q Query) ([]Item, error) {
	c.mu.Lock()
	caps := c.caps
	c.mu.Unlock()
	if caps == nil {
		got, err := c.Caps(ctx)
		if err != nil {
			return nil, err
		}
		caps = &got
	}
	v := url.Values{}
	switch {
	case q.Music && caps.MusicSearch:
		v.Set("t", "music")
		named := false
		if caps.Supports("music", "artist") {
			v.Set("artist", q.Artist)
			named = true
		}
		if q.Album != "" && caps.Supports("music", "album") {
			v.Set("album", q.Album)
		} else if named && q.Album != "" {
			v.Set("q", textQuery(Query{Title: q.Album}))
		}
		if !named {
			v.Set("q", textQuery(Query{Title: q.Artist + " " + q.Album}))
		}
	case q.Music && caps.Search:
		v.Set("t", "search")
		v.Set("q", textQuery(Query{Title: q.Artist + " " + q.Album}))
	case q.Music:
		return nil, &Error{Kind: KindBroken, Msg: "the Source offers no music search"}
	case q.TV && caps.TVSearch:
		v.Set("t", "tvsearch")
		ids := false
		if q.TVDB > 0 && caps.Supports("tv", "tvdbid") {
			v.Set("tvdbid", strconv.Itoa(q.TVDB))
			ids = true
		}
		if q.TVMaze > 0 && caps.Supports("tv", "tvmazeid") {
			v.Set("tvmazeid", strconv.Itoa(q.TVMaze))
			ids = true
		}
		if !ids {
			v.Set("q", textQuery(Query{Title: q.Title}))
		}
		if q.Season > 0 || q.Episode != "" {
			v.Set("season", strconv.Itoa(q.Season))
		}
		if q.Episode != "" {
			v.Set("ep", q.Episode)
		}
	case q.TV && caps.Search:
		v.Set("t", "search")
		v.Set("q", strings.TrimSpace(textQuery(Query{Title: q.Title})+" "+q.Words))
	case q.TV:
		return nil, &Error{Kind: KindBroken, Msg: "the Source offers no show search"}
	case caps.MovieSearch:
		v.Set("t", "movie")
		ids := false
		if q.IMDb != "" && caps.Supports("movie", "imdbid") {
			v.Set("imdbid", strings.TrimPrefix(q.IMDb, "tt"))
			ids = true
		}
		if q.TMDb > 0 && caps.Supports("movie", "tmdbid") {
			v.Set("tmdbid", strconv.Itoa(q.TMDb))
			ids = true
		}
		if !ids {
			v.Set("q", textQuery(q))
		}
	case caps.Search:
		v.Set("t", "search")
		v.Set("q", textQuery(q))
	default:
		return nil, &Error{Kind: KindBroken, Msg: "the Source offers no search"}
	}
	if len(q.Categories) > 0 {
		cs := make([]string, len(q.Categories))
		for i, n := range q.Categories {
			cs[i] = strconv.Itoa(n)
		}
		v.Set("cat", strings.Join(cs, ","))
	}
	if caps.Max > 0 {
		v.Set("limit", strconv.Itoa(caps.Max))
	}
	body, err := c.get(ctx, "search", v)
	if err != nil {
		return nil, err
	}
	return parseItems(body, c.cfg.Protocol)
}

// Feed reads the newest Releases: an rss Source's feed as it is, or a
// Torznab/Newznab search with no words (their own newest list).
func (c *Client) Feed(ctx context.Context, categories []int) ([]Item, error) {
	return c.feed(ctx, categories, c.cfg.Observe)
}

// feedProbe reads the feed through the same client's request limits without
// recording call health. A schedule probe must not write Source counters;
// the durable feed Job records the call when work or recovery is needed.
func (c *Client) feedProbe(ctx context.Context, categories []int, observer func(string, time.Duration, error)) ([]Item, error) {
	return c.feed(ctx, categories, observer)
}

func (c *Client) feed(ctx context.Context, categories []int, observer func(string, time.Duration, error)) ([]Item, error) {
	v := url.Values{}
	if c.cfg.Format != "rss" {
		v.Set("t", "search")
		if len(categories) > 0 {
			cs := make([]string, len(categories))
			for i, n := range categories {
				cs[i] = strconv.Itoa(n)
			}
			v.Set("cat", strings.Join(cs, ","))
		}
	}
	body, err := c.getWithObserver(ctx, "feed", v, observer)
	if err != nil {
		return nil, err
	}
	return parseItems(body, c.cfg.Protocol)
}

// Test checks the Source answers: its caps for Torznab and Newznab, its
// feed for rss. It says how many Releases the feed holds (0 for caps).
func (c *Client) Test(ctx context.Context) (int, error) {
	if c.cfg.Format == "rss" {
		items, err := c.Feed(ctx, nil)
		return len(items), err
	}
	_, err := c.Caps(ctx)
	return 0, err
}

var infoHashRe = regexp.MustCompile(`^([0-9a-f]{40}|[0-9a-f]{64})$`)

// ValidInfoHash says whether s is a lower-case v1 (40 hex) or v2 (64 hex)
// torrent info hash.
func ValidInfoHash(s string) bool { return infoHashRe.MatchString(s) }

func parseItems(body []byte, protocol string) ([]Item, error) {
	var x rssXML
	if err := xml.Unmarshal(body, &x); err != nil {
		return nil, &Error{Kind: KindBroken, Msg: "results are not valid XML: " + err.Error()}
	}
	out := make([]Item, 0, len(x.Items))
	for _, it := range x.Items {
		i := Item{Title: strings.TrimSpace(it.Title), GUID: it.GUID, Link: it.Enclosure.URL, Size: it.Enclosure.Length, Protocol: protocol}
		if i.Link == "" {
			i.Link = strings.TrimSpace(it.Link)
		}
		if i.Size == 0 {
			i.Size = it.Size
		}
		switch it.Enclosure.Type {
		case "application/x-nzb":
			i.Protocol = "usenet"
		case "application/x-bittorrent":
			i.Protocol = "torrent"
		}
		pd := strings.TrimSpace(it.PubDate)
		if t, err := time.Parse(time.RFC1123Z, pd); err == nil {
			i.Published = t.UTC()
		} else if t, err := time.Parse(time.RFC1123, pd); err == nil {
			i.Published = t.UTC()
		}
		for _, a := range it.Attrs {
			switch strings.ToLower(a.Name) {
			case "seeders":
				if n, err := strconv.Atoi(a.Value); err == nil {
					i.Seeders = &n
				}
			case "peers":
				if n, err := strconv.Atoi(a.Value); err == nil {
					i.Peers = &n
				}
			case "infohash":
				// Kept only when it is a real hash: it later selects
				// torrents in a Downloader by id.
				if h := strings.ToLower(strings.TrimSpace(a.Value)); ValidInfoHash(h) {
					i.InfoHash = h
				}
			case "files":
				if n, err := strconv.Atoi(a.Value); err == nil && n > 0 {
					i.Files = n
				}
			case "size":
				if i.Size == 0 {
					i.Size, _ = strconv.ParseInt(a.Value, 10, 64)
				}
			}
		}
		if i.Title == "" || i.Link == "" {
			continue
		}
		out = append(out, i)
	}
	return out, nil
}
