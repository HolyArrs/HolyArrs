package upstream

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// EgressEnv lists the outgoing paths MusicBrainz calls are spread over
// (empty: every call leaves the usual way, one address). Entries are
// comma separated, each "[label=]ip", "[label=]:lo-hi" or
// "[label=]ip:lo-hi", or "[label=]default" for the usual way:
//
//   - ip binds the call's socket to that local address (host networking);
//   - :lo-hi binds it to a local port in that range, which the host turns
//     into its own source address (an iptables SNAT rule per range, for a
//     container on a bridge network);
//   - default is the primary address, shared with the Cover Art Archive
//     and ListenBrainz, so it keeps the shared MetaBrainz pace (Shared).
//
// The label (the public address, say) names the path in logs and stats.
const EgressEnv = "HOLYARRS_MB_EGRESS"

// EgressGap is each path's own pace: one MusicBrainz call a second.
var EgressGap = time.Second

// Egress backoff after a 503 from one path: doubles from the first, at
// most the last; one good answer clears it.
var (
	egressBackoffMin = 15 * time.Second
	egressBackoffMax = 5 * time.Minute
)

// Path is one outgoing route.
type Path struct {
	Label string
	IP    netip.Addr // invalid: any
	Lo    int        // 0: any port
	Hi    int
	Def   bool // the usual way (paced by Shared)

	pace  *Pace
	rt    http.RoundTripper
	mu    sync.Mutex
	until time.Time
	back  time.Duration
	n     int64
	n503  int64
	errs  int64
	held  int64 // 503 backoffs
}

// Egress spreads MusicBrainz calls over its paths, each at its own pace.
type Egress struct {
	Paths []*Path
	start time.Time
}

// Egresses is the configured pool (nil: one address).
var Egresses *Egress

// ParseEgress reads an EgressEnv value; an empty value answers nil.
func ParseEgress(s string) (*Egress, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	e := &Egress{start: time.Now()}
	defs := 0
	for _, raw := range strings.Split(s, ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		p := &Path{}
		if label, rest, ok := strings.Cut(raw, "="); ok {
			p.Label, raw = strings.TrimSpace(label), strings.TrimSpace(rest)
		}
		switch {
		case raw == "default":
			p.Def = true
			defs++
		default:
			host, ports := raw, ""
			if i := strings.LastIndexByte(raw, ':'); i >= 0 {
				host, ports = raw[:i], raw[i+1:]
			}
			if host != "" {
				ip, err := netip.ParseAddr(host)
				if err != nil {
					return nil, fmt.Errorf("egress %q: bad address", raw)
				}
				p.IP = ip
			}
			if ports != "" {
				lo, hi, ok := strings.Cut(ports, "-")
				a, err1 := strconv.Atoi(lo)
				b, err2 := strconv.Atoi(hi)
				if !ok || err1 != nil || err2 != nil || a < 1024 || b > 65535 || a > b {
					return nil, fmt.Errorf("egress %q: bad port range", raw)
				}
				p.Lo, p.Hi = a, b
			}
			if !p.IP.IsValid() && p.Lo == 0 {
				return nil, fmt.Errorf("egress %q: needs an address or a port range", raw)
			}
		}
		if p.Label == "" {
			p.Label = raw
		}
		e.Paths = append(e.Paths, p)
	}
	if defs > 1 {
		return nil, errors.New("egress: default listed twice")
	}
	if len(e.Paths) == 0 {
		return nil, nil
	}
	for _, p := range e.Paths {
		if p.Def {
			p.pace = Shared
			p.rt = egressTransport(nil)
		} else {
			p.pace = NewPace(EgressGap)
			p.rt = egressTransport(p)
		}
	}
	return e, nil
}

// N is how many paths share MusicBrainz (1 without a pool).
func (e *Egress) N() int {
	if e == nil || len(e.Paths) == 0 {
		return 1
	}
	return len(e.Paths)
}

// MBScale is the MusicBrainz rate multiplier for the configured pool.
func MBScale() int { return Egresses.N() }

func egressTransport(p *Path) http.RoundTripper {
	d := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second, Control: publicOnly}
	dial := d.DialContext
	if p != nil {
		dial = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return p.dial(ctx, network, addr)
		}
	}
	return &http.Transport{DialContext: dial, Proxy: nil, MaxIdleConnsPerHost: 2, MaxConnsPerHost: 4,
		IdleConnTimeout: 30 * time.Second, TLSHandshakeTimeout: 5 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second, ForceAttemptHTTP2: true}
}

// dial opens a connection from the path's address and port range (a free
// port picked at random; up to 20 tries).
func (p *Path) dial(ctx context.Context, network, addr string) (net.Conn, error) {
	var last error
	tries := 1
	if p.Lo > 0 {
		tries = 20
	}
	for range tries {
		d := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second, Control: publicOnly}
		la := &net.TCPAddr{}
		if p.IP.IsValid() {
			la.IP = p.IP.AsSlice()
		}
		if p.Lo > 0 {
			la.Port = p.Lo + rand.IntN(p.Hi-p.Lo+1)
			// The port must be reusable at once after a close (TIME_WAIT).
			d.Control = func(network, address string, c syscall.RawConn) error {
				if err := publicOnly(network, address, c); err != nil {
					return err
				}
				return reuseAddr(c)
			}
		}
		d.LocalAddr = la
		c, err := d.DialContext(ctx, network, addr)
		if err == nil {
			return c, nil
		}
		last = err
		if !errors.Is(err, syscall.EADDRINUSE) && !errors.Is(err, syscall.EADDRNOTAVAIL) {
			break
		}
	}
	return nil, last
}

// pick answers the path whose next slot comes first, skipping paths held
// back after a 503 (all held: the one free soonest).
func (e *Egress) pick(now time.Time) *Path {
	var best, held *Path
	var bestAt, heldAt time.Time
	for _, p := range e.Paths {
		p.mu.Lock()
		until := p.until
		p.mu.Unlock()
		if until.After(now) {
			if held == nil || until.Before(heldAt) {
				held, heldAt = p, until
			}
			continue
		}
		at := p.pace.nextAt()
		if best == nil || at.Before(bestAt) {
			best, bestAt = p, at
		}
	}
	if best == nil {
		return held
	}
	return best
}

func (p *Path) observe(resp *http.Response, err error, now time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.n++
	switch {
	case err != nil:
		p.errs++
	case resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusTooManyRequests:
		p.n503++
		p.held++
		if p.back == 0 {
			p.back = egressBackoffMin
		} else {
			p.back = min(2*p.back, egressBackoffMax)
		}
		p.until = now.Add(p.back)
	default:
		p.back = 0
	}
}

// roundTrip sends a MusicBrainz call through the next free path.
func (e *Egress) roundTrip(r *http.Request) (*http.Response, error) {
	p := e.pick(time.Now())
	p.mu.Lock()
	until := p.until
	p.mu.Unlock()
	if d := time.Until(until); d > 0 {
		t := time.NewTimer(d)
		select {
		case <-r.Context().Done():
			t.Stop()
			return nil, r.Context().Err()
		case <-t.C:
		}
	}
	if err := p.pace.Wait(r.Context()); err != nil {
		return nil, err
	}
	if v, ok := r.Context().Value(viaKey{}).(*atomic.Value); ok {
		v.Store(p.addr())
	}
	resp, err := p.rt.RoundTrip(r)
	if err != nil && r.Context().Err() != nil {
		return resp, err
	}
	p.observe(resp, err, time.Now())
	return resp, err
}

// EgressStat is one path's numbers.
type EgressStat struct {
	Label       string  `json:"label"`
	Calls       int64   `json:"calls"`
	Status503   int64   `json:"status_503_or_429"`
	Errors      int64   `json:"errors"`
	Backoffs    int64   `json:"backoffs"`
	HeldSeconds float64 `json:"held_for_seconds"`
}

// Stats answers every path's numbers and the overall rate since start.
func (e *Egress) Stats() map[string]any {
	if e == nil {
		return nil
	}
	now := time.Now()
	out := make([]EgressStat, 0, len(e.Paths))
	var total int64
	for _, p := range e.Paths {
		p.mu.Lock()
		st := EgressStat{Label: p.Label, Calls: p.n, Status503: p.n503, Errors: p.errs, Backoffs: p.held}
		if p.until.After(now) {
			st.HeldSeconds = p.until.Sub(now).Seconds()
		}
		p.mu.Unlock()
		total += st.Calls
		out = append(out, st)
	}
	secs := now.Sub(e.start).Seconds()
	return map[string]any{"paths": out, "calls": total, "per_second_since_start": float64(total) / max(secs, 1), "since": e.start.UTC().Format(time.RFC3339)}
}

// LogEvery writes one line of per-path call counts every interval, with
// the rate over that interval (docker logs, no token needed).
func (e *Egress) LogEvery(ctx context.Context, every time.Duration) {
	if e == nil {
		return
	}
	last := make([]int64, len(e.Paths))
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		var b strings.Builder
		var sum int64
		for i, p := range e.Paths {
			p.mu.Lock()
			n, n503 := p.n, p.n503
			p.mu.Unlock()
			fmt.Fprintf(&b, " %s=%d(503:%d)", p.Label, n-last[i], n503)
			sum += n - last[i]
			last[i] = n
		}
		log.Printf("mb egress: %d paths, %d calls in %s (%.2f/s):%s", len(e.Paths), sum, every, float64(sum)/every.Seconds(), b.String())
	}
}

// nextAt answers the pace's next free slot.
func (p *Pace) nextAt() time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.next
}

// musicBrainzHost is the one host spread over the pool.
func musicBrainzHost(host string) bool {
	h := strings.ToLower(host)
	return h == "musicbrainz.org" || h == "www.musicbrainz.org"
}

// Client answers an HTTP client spreading calls over the pool's paths,
// each path at its own pace (nil pool: nil client).
func (e *Egress) Client(timeout time.Duration) *http.Client {
	if e == nil || len(e.Paths) == 0 {
		return nil
	}
	return &http.Client{Timeout: timeout, Transport: egressRT{e}}
}

type egressRT struct{ e *Egress }

func (t egressRT) RoundTrip(r *http.Request) (*http.Response, error) { return t.e.roundTrip(r) }

type viaKey struct{}

// WithVia answers a context that records which pool path sent a
// MusicBrainz call made with it, and a func reading that path's address
// ("" when no pool path was used).
func WithVia(ctx context.Context) (context.Context, func() string) {
	v := &atomic.Value{}
	v.Store("")
	return context.WithValue(ctx, viaKey{}, v), func() string { return v.Load().(string) }
}

func (p *Path) addr() string {
	if p.IP.IsValid() {
		return p.IP.String()
	}
	return p.Label
}
