// Package upstream holds what every outside call shares, whichever client
// makes it: one pace for all MetaBrainz hosts (musicbrainz.org, the Cover
// Art Archive and ListenBrainz sit behind one gateway and count against one
// address) and a health watch that notices a run of connection failures to
// one host, logs it and reports it in the stats.
package upstream

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/model"
)

// MetaBrainz reports a host behind the MetaBrainz gateway.
func MetaBrainz(host string) bool {
	h := strings.ToLower(host)
	if i := strings.LastIndexByte(h, ':'); i > 0 && !strings.Contains(h[i:], "]") {
		h = h[:i]
	}
	for _, s := range []string{"musicbrainz.org", "coverartarchive.org", "listenbrainz.org", "metabrainz.org"} {
		if h == s || strings.HasSuffix(h, "."+s) {
			return true
		}
	}
	return false
}

// DefaultGap is the least time between two MetaBrainz requests from this
// server, all hosts together (1.25 a second; musicbrainz.org itself keeps
// its own 1.1 s on top).
const DefaultGap = 800 * time.Millisecond

// MaxQueued is how many slots ahead a pace hands out: a caller that would
// wait longer than gap*MaxQueued is refused at once with ErrBusy instead
// of pushing the next slot further out.
const MaxQueued = 64

// ErrBusy: the pace is full (MaxQueued callers already wait). Nothing was
// sent; a queue treats it as "try again later", never as a failure.
var ErrBusy = errors.New("upstream busy: metabrainz pace full")

// Pace hands out request slots one Gap apart.
type Pace struct {
	mu   sync.Mutex
	gap  time.Duration
	next time.Time
	wait time.Duration // total time callers waited (stats)
	n    int64
	busy int64 // callers refused with ErrBusy
	// max is how many slots ahead next may run (0: MaxQueued).
	max int
}

// NewPace makes a pace with the given gap.
func NewPace(gap time.Duration) *Pace { return &Pace{gap: gap} }

// SetGap changes the gap (config).
func (p *Pace) SetGap(gap time.Duration) {
	if gap <= 0 {
		return
	}
	p.mu.Lock()
	p.gap = gap
	p.mu.Unlock()
}

// Wait blocks until the next slot (or ctx ends). Past the cap it answers
// ErrBusy at once. A caller whose ctx ends while it waits gives its slot
// back when it holds the last one handed out; a slot in the middle is left
// unused (moving the later ones would put two calls closer than gap), and
// the cap bounds how far ahead such gaps can push.
func (p *Pace) Wait(ctx context.Context) error {
	p.mu.Lock()
	now := time.Now()
	slot := p.next
	if slot.Before(now) {
		slot = now
	}
	limit := p.max
	if limit <= 0 {
		limit = MaxQueued
	}
	if slot.Sub(now) >= time.Duration(limit)*p.gap {
		p.busy++
		p.mu.Unlock()
		return ErrBusy
	}
	end := slot.Add(p.gap)
	p.next = end
	d := slot.Sub(now)
	p.wait += d
	p.n++
	p.mu.Unlock()
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		p.mu.Lock()
		if p.next.Equal(end) {
			p.next = slot
		}
		p.mu.Unlock()
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// PaceStats are the shared pace's numbers.
type PaceStats struct {
	PerSecond   float64 `json:"per_second"`
	Requests    int64   `json:"requests"`
	AvgWaitSecs float64 `json:"avg_wait_seconds"`
	Busy        int64   `json:"busy_refused"`
}

// Stats reads the numbers.
func (p *Pace) Stats() PaceStats {
	p.mu.Lock()
	defer p.mu.Unlock()
	st := PaceStats{PerSecond: float64(time.Second) / float64(p.gap), Requests: p.n, Busy: p.busy}
	if p.n > 0 {
		st.AvgWaitSecs = p.wait.Seconds() / float64(p.n)
	}
	return st
}

// Shared is the process's MetaBrainz pace; every transport below waits on it.
var Shared = NewPace(DefaultGap)

// Watch is the process's connection health watch.
var Watch = NewHealth(log.Printf)

// ConnError reports a failure to reach a host at all (refused, reset,
// a dropped TLS handshake, DNS, an EOF before any answer, a dial or
// handshake timeout), as opposed to an answer saying no. Such a failure
// says nothing about the item asked for, so a queue retries it later
// without counting it.
func ConnError(err error) bool {
	if err == nil || errors.Is(err, ErrBusy) {
		return false
	}
	var fe *model.FetchError
	if errors.As(err, &fe) {
		return fe.Code == "upstream_unavailable" || fe.Code == "upstream_timeout"
	}
	var ne net.Error
	var op *net.OpError
	var dns *net.DNSError
	var rh tls.RecordHeaderError
	switch {
	case errors.As(err, &op), errors.As(err, &dns), errors.As(err, &rh):
		return true
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF),
		errors.Is(err, syscall.ECONNRESET), errors.Is(err, syscall.ECONNREFUSED),
		errors.Is(err, syscall.ECONNABORTED), errors.Is(err, syscall.EPIPE), errors.Is(err, syscall.ETIMEDOUT):
		return true
	case errors.As(err, &ne) && ne.Timeout():
		return true
	}
	s := strings.ToLower(err.Error())
	for _, k := range []string{"connection reset", "connection refused", "handshake", "no such host", "i/o timeout", "eof", "broken pipe", "network is unreachable", "upstream_unavailable"} {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

// AlarmAfter is how many connection failures in a row to one host raise
// the alarm (a log line, and the host shows as failing in the stats).
const AlarmAfter = 3

// Health watches connection failures per host.
type Health struct {
	mu    sync.Mutex
	logf  func(string, ...any)
	hosts map[string]*hostHealth
	now   func() time.Time
}

type hostHealth struct {
	Host      string    `json:"host"`
	Run       int       `json:"failures_in_a_row"`
	Since     time.Time `json:"failing_since,omitzero"`
	LastError string    `json:"last_error,omitempty"`
	LastFail  time.Time `json:"last_failure,omitzero"`
	LastOK    time.Time `json:"last_ok,omitzero"`
	Alarms    int       `json:"alarms"`
	Failures  int64     `json:"failures"`
	// LongestRun and LastRecovery describe the last alarm that ended.
	LongestRun   int       `json:"longest_run"`
	LastRecovery time.Time `json:"last_recovery,omitzero"`
}

// NewHealth makes a watch logging through logf.
func NewHealth(logf func(string, ...any)) *Health {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	return &Health{logf: logf, hosts: map[string]*hostHealth{}, now: time.Now}
}

// Observe records one request's outcome (err nil: an answer came back).
func (h *Health) Observe(host string, err error) {
	host = strings.ToLower(host)
	if host == "" {
		return
	}
	conn := ConnError(err)
	if err != nil && !conn {
		return // the host answered; a cancelled call or a bad body says nothing
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	now := h.now()
	e := h.hosts[host]
	if e == nil {
		if !conn {
			return // only hosts that failed once are kept
		}
		e = &hostHealth{Host: host}
		h.hosts[host] = e
	}
	if !conn {
		if e.Run >= AlarmAfter {
			h.logf("upstream: %s answers again after %d connection failures in a row (%s)", host, e.Run, now.Sub(e.Since).Round(time.Second))
			e.LastRecovery = now
		}
		e.LongestRun = max(e.LongestRun, e.Run)
		e.Run, e.Since, e.LastOK = 0, time.Time{}, now
		return
	}
	if e.Run == 0 {
		e.Since = now
	}
	e.Run++
	e.Failures++
	e.LastFail = now
	e.LastError = trim(err.Error())
	if e.Run == AlarmAfter || e.Run > AlarmAfter && e.Run%25 == 0 {
		if e.Run == AlarmAfter {
			e.Alarms++
		}
		h.logf("upstream ALARM: %s failed to connect %d times in a row since %s (last: %s)", host, e.Run, e.Since.UTC().Format(time.RFC3339), e.LastError)
	}
}

func trim(s string) string {
	if len(s) > 160 {
		return s[:160]
	}
	return s
}

// HealthReport is the watch's stats: hosts failing now and every host
// with a failure in the last day.
type HealthReport struct {
	Failing []string     `json:"failing"`
	Hosts   []hostHealth `json:"hosts"`
}

// Report reads the watch.
func (h *Health) Report() HealthReport {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := h.now()
	out := HealthReport{Failing: []string{}, Hosts: []hostHealth{}}
	for host, e := range h.hosts {
		if e.Run >= AlarmAfter {
			out.Failing = append(out.Failing, host)
		}
		if now.Sub(e.LastFail) < 24*time.Hour {
			out.Hosts = append(out.Hosts, *e)
		}
	}
	sort.Strings(out.Failing)
	sort.Slice(out.Hosts, func(a, b int) bool { return out.Hosts[a].Host < out.Hosts[b].Host })
	return out
}

// Failing reports whether host is in an alarm now.
func (h *Health) Failing(host string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	e := h.hosts[strings.ToLower(host)]
	return e != nil && e.Run >= AlarmAfter
}

// Transport waits on the shared MetaBrainz pace for MetaBrainz hosts and
// reports every request's outcome to the health watch.
type Transport struct {
	Next   http.RoundTripper
	Pace   *Pace   // nil: Shared
	Health *Health // nil: Watch
}

// RoundTrip implements http.RoundTripper.
func (t Transport) RoundTrip(r *http.Request) (*http.Response, error) {
	pace, health := t.Pace, t.Health
	if pace == nil {
		pace = Shared
	}
	if health == nil {
		health = Watch
	}
	host := r.URL.Hostname()
	if e := Egresses; e != nil && musicBrainzHost(host) {
		resp, err := e.roundTrip(r)
		if err != nil && r.Context().Err() != nil {
			return resp, err
		}
		if !errors.Is(err, ErrBusy) {
			health.Observe(host, err)
		}
		return resp, err
	}
	if MetaBrainz(host) {
		if err := pace.Wait(r.Context()); err != nil {
			return nil, err
		}
	}
	next := t.Next
	if next == nil {
		next = http.DefaultTransport
	}
	resp, err := next.RoundTrip(r)
	if err != nil && r.Context().Err() != nil {
		return resp, err // the caller gave up; not the host's fault
	}
	health.Observe(host, err)
	return resp, err
}

// Wrap answers rt wrapped in Transport (rt nil: the default transport).
func Wrap(rt http.RoundTripper) http.RoundTripper {
	if _, ok := rt.(Transport); ok {
		return rt
	}
	return Transport{Next: rt}
}
