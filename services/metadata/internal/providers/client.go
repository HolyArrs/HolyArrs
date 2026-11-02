package providers

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/model"
	"github.com/CoderLuii/holyarrs-metadata/internal/upstream"
)

type Config struct {
	BaseURL string
	KeyFile string
	// Key is an inline credential (for example read from the environment);
	// it is mutually exclusive with KeyFile.
	Key      string
	Interval time.Duration
	// Concurrency is how many calls may be in flight at once (default:
	// TMDB 6, others 1); starts still keep Interval apart.
	Concurrency int
}

type Options struct {
	Providers    map[string]Config
	AllowPrivate bool
	MaxBodyBytes int64
	MaxPending   int
	Timeout      time.Duration
	Attempts     int
	UserAgent    string
}

type flight struct {
	waiters int
	done    chan struct{}
	cancel  context.CancelFunc
	result  model.Result
	err     error
}

type provider struct {
	base     *url.URL
	token    string
	interval time.Duration
	gate     chan struct{}
	// mu guards next (several callers hold the gate when Concurrency > 1).
	mu     sync.Mutex
	next   time.Time
	paused atomic.Bool
	// prioWait counts priority callers waiting for the gate; lastPrio
	// (read and written under mu) says who had the
	// last slot. See acquire.
	prioWait atomic.Int32
	lastPrio bool
}

type Client struct {
	mu        sync.Mutex
	flights   map[string]*flight
	active    int
	opts      Options
	providers map[string]*provider
	http      *http.Client
}

var defaults = map[string]Config{
	// TMDB: up to 6 calls at once, starts 50 ms apart (20 a second; TMDB
	// allows about 50).
	"tmdb":        {BaseURL: "https://api.themoviedb.org/3", Interval: 50 * time.Millisecond, Concurrency: 6},
	"tvmaze":      {BaseURL: "https://api.tvmaze.com", Interval: time.Second},
	"musicbrainz": {BaseURL: "https://musicbrainz.org/ws/2", Interval: 1100 * time.Millisecond},
}

func New(opts Options) (*Client, error) {
	if opts.MaxBodyBytes == 0 {
		opts.MaxBodyBytes = 2 << 20
	}
	if opts.MaxPending == 0 {
		opts.MaxPending = 200
	}
	if opts.Timeout == 0 {
		opts.Timeout = 15 * time.Second
	}
	if opts.Attempts == 0 {
		opts.Attempts = 3
	}
	if opts.UserAgent == "" {
		opts.UserAgent = "HolyArrsMetadata/0.1 (+https://github.com/CoderLuii/HolyArrs)"
	}
	if opts.MaxBodyBytes < 1 || opts.MaxBodyBytes > 16<<20 || opts.MaxPending < 1 || opts.MaxPending > 10000 || opts.Timeout < time.Millisecond || opts.Timeout > 2*time.Minute || opts.Attempts < 1 || opts.Attempts > 5 || strings.ContainsAny(opts.UserAgent, "\r\n") || len(opts.UserAgent) > 512 {
		return nil, fmt.Errorf("invalid provider options")
	}
	configs := opts.Providers
	if configs == nil {
		configs = defaults
	}
	c := &Client{flights: make(map[string]*flight), opts: opts, providers: make(map[string]*provider)}
	for name, cfg := range configs {
		def, ok := defaults[name]
		if !ok {
			return nil, fmt.Errorf("unsupported provider configuration")
		}
		if cfg.BaseURL == "" {
			cfg.BaseURL = def.BaseURL
		}
		if cfg.Interval == 0 {
			cfg.Interval = def.Interval
		}
		if cfg.Interval < 0 || cfg.Interval > time.Hour {
			return nil, fmt.Errorf("invalid provider interval")
		}
		if cfg.Concurrency == 0 {
			cfg.Concurrency = max(def.Concurrency, 1)
		}
		if cfg.Concurrency < 1 || cfg.Concurrency > 16 || name == "musicbrainz" && cfg.Concurrency > 8 {
			return nil, fmt.Errorf("invalid provider concurrency")
		}
		u, err := url.Parse(cfg.BaseURL)
		if err != nil || u.Host == "" || (u.Scheme != "https" && !(opts.AllowPrivate && u.Scheme == "http")) || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || u.RawPath != "" || strings.Contains(u.Path, "..") || strings.ContainsAny(u.Path, "\\\r\n") {
			return nil, fmt.Errorf("invalid provider base URL")
		}
		u.Path = strings.TrimRight(u.Path, "/")
		if !opts.AllowPrivate && u.String() != def.BaseURL {
			return nil, fmt.Errorf("provider base URL is not allowlisted")
		}
		p := &provider{base: u, interval: cfg.Interval, gate: make(chan struct{}, cfg.Concurrency)}
		if cfg.Key != "" {
			if name != "tmdb" || cfg.KeyFile != "" {
				return nil, fmt.Errorf("credential unsupported for provider")
			}
			p.token = strings.TrimSpace(cfg.Key)
			if p.token == "" || len(p.token) > 16384 || strings.ContainsAny(p.token, " \t\r\n") {
				return nil, fmt.Errorf("invalid provider credential")
			}
		} else if cfg.KeyFile != "" {
			if name != "tmdb" {
				return nil, fmt.Errorf("credential file unsupported for provider")
			}
			f, err := os.Open(cfg.KeyFile)
			if err != nil {
				return nil, fmt.Errorf("cannot open provider credential")
			}
			b, readErr := io.ReadAll(io.LimitReader(f, 16385))
			f.Close()
			if readErr != nil || len(b) > 16384 {
				return nil, fmt.Errorf("invalid provider credential")
			}
			p.token = strings.TrimSpace(string(b))
			if p.token == "" || strings.ContainsAny(p.token, " \t\r\n") {
				return nil, fmt.Errorf("invalid provider credential")
			}
		}
		if name == "tmdb" && p.token == "" && !opts.AllowPrivate {
			p.paused.Store(true)
		}
		c.providers[name] = p
	}
	transport := &http.Transport{DialContext: safeDial(opts.AllowPrivate), MaxIdleConns: 6, MaxIdleConnsPerHost: 2, MaxConnsPerHost: 2, IdleConnTimeout: 30 * time.Second, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: opts.Timeout, MaxResponseHeaderBytes: 32 << 10, DisableCompression: true}
	c.http = &http.Client{Transport: upstream.Transport{Next: transport}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return c, nil
}

func safeDial(allowPrivate bool) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, errors.New("invalid upstream address")
		}
		addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			return nil, errors.New("upstream resolution failed")
		}
		if len(addresses) == 0 {
			return nil, errors.New("upstream resolution failed")
		}
		if !allowPrivate {
			for _, addr := range addresses {
				if !publicAddress(addr) {
					return nil, errors.New("upstream address blocked")
				}
			}
		}
		var last error
		for _, addr := range addresses {
			conn, err := (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext(ctx, network, net.JoinHostPort(addr.String(), port))
			if err == nil {
				return conn, nil
			}
			last = err
		}
		return nil, last
	}
}

func publicAddress(addr netip.Addr) bool {
	addr = addr.Unmap()
	if !addr.IsGlobalUnicast() || addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() || addr.Zone() != "" {
		return false
	}
	for _, prefix := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "64:ff9b::/96", "64:ff9b:1::/48", "100::/64", "2001::/23", "2001:db8::/32", "2002::/16", "fec0::/10"} {
		if netip.MustParsePrefix(prefix).Contains(addr) {
			return false
		}
	}
	return true
}

func (c *Client) Fetch(ctx context.Context, q model.Query) (model.Result, error) {
	if ctx.Err() != nil {
		return model.Result{}, &model.FetchError{Code: "cancelled"}
	}
	if err := q.Validate(); err != nil || !validQueryID(q) {
		return model.Result{}, &model.FetchError{Code: "invalid_query"}
	}
	p, ok := c.providers[q.Provider]
	if !ok {
		return model.Result{}, &model.FetchError{Code: "provider_unavailable"}
	}
	if p.paused.Load() {
		return model.Result{}, &model.FetchError{Code: "provider_credentials"}
	}
	key := q.Key()
	c.mu.Lock()
	f := c.flights[key]
	if f == nil {
		if c.active >= c.opts.MaxPending {
			c.mu.Unlock()
			return model.Result{}, &model.FetchError{Code: "queue_full", RetryAfter: time.Second}
		}
		jobCtx, cancel := context.WithTimeout(context.Background(), c.opts.Timeout)
		if Priority(ctx) {
			jobCtx = WithPriority(jobCtx)
		}
		f = &flight{done: make(chan struct{}), cancel: cancel}
		c.flights[key] = f
		c.active++
		go c.run(jobCtx, key, q, p, f)
	}
	f.waiters++
	c.mu.Unlock()
	select {
	case <-ctx.Done():
		c.mu.Lock()
		f.waiters--
		if f.waiters == 0 && c.flights[key] == f {
			delete(c.flights, key)
			f.cancel()
		}
		c.mu.Unlock()
		return model.Result{}, &model.FetchError{Code: "cancelled"}
	case <-f.done:
		r := f.result
		r.Data = append([]byte(nil), r.Data...)
		if fe, ok := f.err.(*model.FetchError); ok {
			copyErr := *fe
			return r, &copyErr
		}
		return r, f.err
	}
}

func (c *Client) run(ctx context.Context, key string, q model.Query, p *provider, f *flight) {
	r, err := c.lookup(ctx, q, p)
	c.mu.Lock()
	f.result = r
	f.err = err
	if c.flights[key] == f {
		delete(c.flights, key)
	}
	c.active--
	close(f.done)
	c.mu.Unlock()
	f.cancel()
}

func (c *Client) lookup(ctx context.Context, q model.Query, p *provider) (model.Result, error) {
	for attempt := 0; attempt < c.opts.Attempts; attempt++ {
		if p.acquire(ctx) != nil {
			return model.Result{}, &model.FetchError{Code: "upstream_timeout"}
		}
		if p.paused.Load() {
			<-p.gate
			return model.Result{}, &model.FetchError{Code: "provider_credentials"}
		}
		// Reserving the next start makes backoff global across distinct queries.
		if p.reserve(ctx) != nil {
			<-p.gate
			return model.Result{}, &model.FetchError{Code: "upstream_timeout"}
		}
		deadline, _ := ctx.Deadline()
		remaining := time.Until(deadline)
		attemptTime := remaining / time.Duration(c.opts.Attempts-attempt)
		attemptCtx, cancel := context.WithTimeout(ctx, attemptTime)
		r, fe, retry := c.request(attemptCtx, q, p)
		cancel()
		if fe != nil && (fe.Status == 401 || fe.Status == 403) {
			p.paused.Store(true)
			fe.Code = "provider_credentials"
			retry = false
		}
		if fe != nil && fe.RetryAfter > 0 {
			p.holdUntil(time.Now().Add(fe.RetryAfter))
		}
		if retry && attempt+1 < c.opts.Attempts && (fe == nil || fe.RetryAfter == 0) {
			p.holdUntil(time.Now().Add(time.Duration(1<<attempt) * 50 * time.Millisecond))
		}
		<-p.gate
		if fe == nil {
			return c.withMusicSecond(ctx, q, p, c.withSeasonTrailer(ctx, q, p, r))
		}
		if !retry || attempt+1 == c.opts.Attempts {
			return model.Result{}, fe
		}
		if ctx.Err() != nil {
			return model.Result{}, &model.FetchError{Code: "upstream_timeout"}
		}
	}
	return model.Result{}, &model.FetchError{Code: "upstream_timeout"}
}

func (c *Client) request(ctx context.Context, q model.Query, p *provider) (model.Result, *model.FetchError, bool) {
	u := requestURL(p.base, q)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return model.Result{}, &model.FetchError{Code: "invalid_query"}, false
	}
	req.Header.Set("User-Agent", c.opts.UserAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Encoding", "gzip")
	if p.token != "" {
		req.Header.Set("Authorization", "Bearer "+p.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if errors.Is(err, upstream.ErrBusy) {
			// Nothing was sent: the shared MetaBrainz pace is full.
			return model.Result{}, &model.FetchError{Code: "upstream_busy", Status: 503, RetryAfter: time.Second}, false
		}
		var ne net.Error
		if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &ne) && ne.Timeout() {
			return model.Result{}, &model.FetchError{Code: "upstream_timeout"}, true
		}
		return model.Result{}, &model.FetchError{Code: "upstream_unavailable"}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fe := &model.FetchError{Code: "upstream_status", Status: resp.StatusCode, RetryAfter: retryAfter(resp.Header.Get("Retry-After"), time.Now())}
		return model.Result{}, fe, resp.StatusCode == 429 || resp.StatusCode == 503
	}
	var body io.Reader = resp.Body
	switch strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Encoding"))) {
	case "", "identity":
	case "gzip":
		z, err := gzip.NewReader(resp.Body)
		if err != nil {
			return model.Result{}, &model.FetchError{Code: "upstream_invalid"}, false
		}
		defer z.Close()
		body = z
	default:
		return model.Result{}, &model.FetchError{Code: "upstream_invalid"}, false
	}
	b, err := io.ReadAll(io.LimitReader(body, c.opts.MaxBodyBytes+1))
	if err != nil {
		if ctx.Err() != nil {
			return model.Result{}, &model.FetchError{Code: "upstream_timeout"}, true
		}
		return model.Result{}, &model.FetchError{Code: "upstream_invalid"}, false
	}
	if int64(len(b)) > c.opts.MaxBodyBytes {
		return model.Result{}, &model.FetchError{Code: "upstream_too_large"}, false
	}
	r, err := normalize(q, b)
	if err != nil {
		return model.Result{}, &model.FetchError{Code: "upstream_invalid"}, false
	}
	return r, nil, false
}

func retryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil && seconds >= 0 {
		if seconds > int64((1<<63-1)/time.Second) {
			return time.Duration(1<<63 - 1)
		}
		return time.Duration(seconds) * time.Second
	}
	if date, err := http.ParseTime(value); err == nil && date.After(now) {
		return date.Sub(now)
	}
	return 0
}
