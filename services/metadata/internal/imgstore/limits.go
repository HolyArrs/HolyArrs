package imgstore

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
	_ "time/tzdata" // YouTube's quota day is Pacific time
)

// ErrSourcePaused: the source is paused (5 failures, a 429 back-off, or its
// daily quota is used up).
var ErrSourcePaused = errors.New("imgstore: source paused")

// Limits paces each picture source, pauses a source for an hour after 5
// failures in a row, backs off on 429 and keeps YouTube's daily quota.
type Limits struct {
	Now func() time.Time

	mu       sync.Mutex
	src      map[string]*srcState
	quotaDay string
	units    int
	persist  func(day string, units int)
}

type srcState struct {
	interval time.Duration
	next     time.Time
	fails    int
	until    time.Time
	backoff  time.Duration
	// win counts the last ten minutes' answers, one bucket a minute.
	win [10]bucket
}

type bucket struct {
	minute              int64
	ok, failed, limited int
}

// add counts one answer in the current minute's bucket.
func (s *srcState) add(now time.Time, failed, limited bool) {
	m := now.Unix() / 60
	b := &s.win[m%int64(len(s.win))]
	if b.minute != m {
		*b = bucket{minute: m}
	}
	switch {
	case limited:
		b.limited++
	case failed:
		b.failed++
	default:
		b.ok++
	}
}

// last10 sums the buckets of the last ten minutes.
func (s *srcState) last10(now time.Time) (total, failed, limited int) {
	m := now.Unix() / 60
	for _, b := range s.win {
		if m-b.minute < int64(len(s.win)) {
			total += b.ok + b.failed + b.limited
			failed += b.failed
			limited += b.limited
		}
	}
	return
}

// Alert thresholds: a source failing (errors and 429s) more than
// alertShare of at least alertMin answers in ten minutes.
const (
	alertShare = 0.2
	alertMin   = 20
)

// SourceStatus is one source in /v1/stats.
type SourceStatus struct {
	State    string    `json:"state"` // ok, paused, quota
	Failures int       `json:"failures"`
	Until    time.Time `json:"until,omitzero"`
	Used     int       `json:"used,omitempty"`
	Limit    int       `json:"limit,omitempty"`
	// The last ten minutes: answers, failures and 429s; Alert when more
	// than 20% of at least 20 answers failed.
	Answers10m int  `json:"answers_10m"`
	Failed10m  int  `json:"failed_10m,omitempty"`
	Limited10m int  `json:"limited_10m,omitempty"`
	Alert      bool `json:"alert,omitempty"`
}

// YouTubeDailyUnits is where YouTube calls stop for the day (of 10,000).
const YouTubeDailyUnits = 9000

const (
	pauseAfter = 5
	pauseFor   = time.Hour
)

var pacific = func() *time.Location {
	l, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		return time.UTC
	}
	return l
}()

// NewLimits makes the limits; persist (may be nil) keeps YouTube's count.
func NewLimits(persist func(day string, units int)) *Limits {
	return &Limits{Now: time.Now, src: map[string]*srcState{}, persist: persist}
}

func (l *Limits) state(n string) *srcState {
	s := l.src[n]
	if s == nil {
		s = &srcState{}
		l.src[n] = s
	}
	return s
}

// SetInterval sets the least time between two calls to a source.
func (l *Limits) SetInterval(name string, d time.Duration) {
	l.mu.Lock()
	l.state(name).interval = d
	l.mu.Unlock()
}

// Allow waits for the source's next slot; ErrSourcePaused when paused.
func (l *Limits) Allow(ctx context.Context, name string) error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	now := l.Now()
	s := l.state(name)
	if now.Before(s.until) {
		l.mu.Unlock()
		return ErrSourcePaused
	}
	if name == "youtube" && l.day(now) == l.quotaDay && l.units >= YouTubeDailyUnits {
		l.mu.Unlock()
		return ErrSourcePaused
	}
	wait := time.Duration(0)
	if s.interval > 0 {
		at := s.next
		if at.Before(now) {
			at = now
		}
		s.next = at.Add(s.interval)
		wait = at.Sub(now)
	}
	l.mu.Unlock()
	if wait <= 0 {
		return nil
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Busy reports the source is paused or its queue (the next free slot)
// waits longer than wait.
func (l *Limits) Busy(name string, wait time.Duration) bool {
	if l == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.Now()
	s := l.state(name)
	return now.Before(s.until) || s.next.Sub(now) > wait
}

// Done records a call's result: a success resets the failure count, 404
// and other "nothing here" answers count as neither, and a network error,
// 401, 403 or 5xx counts as a failure; the 5th in a row pauses the source
// for an hour. A 429 should go to Backoff instead.
func (l *Limits) Done(name string, status int, err error) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	s := l.state(name)
	failed := err != nil || status == 401 || status == 403 || status >= 500
	s.add(l.Now(), failed, false)
	switch {
	case failed:
		s.fails++
		if s.fails >= pauseAfter {
			s.until = l.Now().Add(pauseFor)
			s.fails = 0
		}
	case status >= 200 && status < 300:
		s.fails, s.backoff = 0, 0
	}
}

// Backoff pauses a source after a 429: retryAfter when the source said so,
// else 30 s doubling up to an hour.
func (l *Limits) Backoff(name string, retryAfter time.Duration) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	s := l.state(name)
	s.add(l.Now(), false, true)
	d := retryAfter
	if d <= 0 {
		if s.backoff == 0 {
			s.backoff = 30 * time.Second
		} else if s.backoff *= 2; s.backoff > pauseFor {
			s.backoff = pauseFor
		}
		d = s.backoff
	}
	s.until = l.Now().Add(d)
}

func (l *Limits) day(t time.Time) string { return t.In(pacific).Format("2006-01-02") }

// Spend takes units from a source's daily quota (YouTube only); false when
// they would pass YouTubeDailyUnits (the count starts again the next
// Pacific day, when YouTube resets it).
func (l *Limits) Spend(name string, units int) bool {
	if l == nil || name != "youtube" {
		return true
	}
	l.mu.Lock()
	d := l.day(l.Now())
	if d != l.quotaDay {
		l.quotaDay, l.units = d, 0
	}
	if l.units+units > YouTubeDailyUnits {
		l.mu.Unlock()
		return false
	}
	l.units += units
	day, n, p := l.quotaDay, l.units, l.persist
	l.mu.Unlock()
	if p != nil {
		p(day, n)
	}
	return true
}

// LoadQuota restores a stored YouTube count (after a restart).
func (l *Limits) LoadQuota(day string, units int) {
	l.mu.Lock()
	l.quotaDay, l.units = day, units
	l.mu.Unlock()
}

// Status reports every source that was used.
func (l *Limits) Status() map[string]SourceStatus {
	out := map[string]SourceStatus{}
	if l == nil {
		return out
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.Now()
	for n, s := range l.src {
		st := SourceStatus{State: "ok", Failures: s.fails}
		if now.Before(s.until) {
			st.State, st.Until = "paused", s.until
		}
		st.Answers10m, st.Failed10m, st.Limited10m = s.last10(now)
		st.Alert = st.Answers10m >= alertMin && float64(st.Failed10m+st.Limited10m) > alertShare*float64(st.Answers10m)
		out[n] = st
	}
	yt := out["youtube"]
	if yt.State == "" {
		yt.State = "ok"
	}
	if l.day(now) == l.quotaDay {
		yt.Used = l.units
	}
	yt.Limit = YouTubeDailyUnits
	if yt.Used >= YouTubeDailyUnits && yt.State == "ok" {
		yt.State = "quota"
	}
	out["youtube"] = yt
	return out
}

// Redact replaces every non-empty secret in s.
func Redact(s string, secrets ...string) string {
	for _, v := range secrets {
		if v != "" {
			s = strings.ReplaceAll(s, v, "[redacted]")
		}
	}
	return s
}
