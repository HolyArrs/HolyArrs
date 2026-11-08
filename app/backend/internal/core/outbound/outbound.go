// Package outbound guards calls to outside services. A circuit breaker per
// endpoint stops calls to a service that keeps failing and lets one probe
// through after a cooldown. It never retries: callers keep their own retry
// and pacing rules and report one result per completed operation.
package outbound

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"net/http"
	"sort"
	"sync"
	"time"
)

const (
	defaultThreshold = 3
	defaultCooldown  = 30 * time.Second
	defaultMax       = 1024
)

// ErrSuppressed means a recently failing endpoint is waiting before one
// recovery probe is admitted.
var ErrSuppressed = errors.New("outside service is retrying")

var shared = New(nil, Options{})

// Outcome is the availability result of one completed operation.
type Outcome uint8

// The outcomes a caller reports through Operation.Finish.
const (
	Success Outcome = iota
	TransientFailure
	TerminalFailure
	Canceled
)

// Options changes breaker limits. Zero values use the production defaults.
type Options struct {
	Threshold int
	Cooldown  time.Duration
	Max       int
	Now       func() time.Time
}

// Target identifies an endpoint without keeping its address or credentials.
// Identity values may hold sensitive settings, so only their keyed digest is
// kept.
type Target struct {
	Kind     string
	Identity []string
}

// State is the safe health view of an endpoint that is suppressed or being
// probed. ID is an opaque digest that means nothing outside this process.
type State struct {
	ID     string
	Kind   string
	Status string
	Reason string
	// OpenUntil is when the next call is allowed. It is zero when the
	// endpoint is not suppressed and a probe may run now.
	OpenUntil time.Time
}

// Registry holds the breaker state, bounded in size, for one process.
type Registry struct {
	mu        sync.Mutex
	key       []byte
	entries   map[string]*entry
	threshold int
	cooldown  time.Duration
	max       int
	now       func() time.Time
}

// Operation is one admitted call. Finish may run more than once; only the
// first result changes the breaker.
type Operation struct {
	r    *Registry
	id   string
	once sync.Once
}

type entry struct {
	kind      string
	failures  int
	openUntil time.Time
	probe     bool
	active    int
	touched   time.Time
}

// New makes an independent registry. key should be unpredictable and
// private to the process; an empty key is replaced with a random one.
func New(key []byte, opt Options) *Registry {
	if len(key) == 0 {
		key = make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			panic("outbound: random identity key unavailable")
		}
	} else {
		key = append([]byte(nil), key...)
	}
	if opt.Threshold <= 0 {
		opt.Threshold = defaultThreshold
	}
	if opt.Cooldown <= 0 {
		opt.Cooldown = defaultCooldown
	}
	if opt.Max <= 0 {
		opt.Max = defaultMax
	}
	if opt.Now == nil {
		opt.Now = time.Now
	}
	return &Registry{key: key, entries: map[string]*entry{}, threshold: opt.Threshold, cooldown: opt.Cooldown, max: opt.Max, now: opt.Now}
}

// Begin admits one operation through the process-wide registry.
func Begin(ctx context.Context, target Target) (*Operation, error) {
	return shared.Begin(ctx, target)
}

// Reset forgets every breaker in the process-wide registry. Tests call it
// because each test opens a fresh database whose ids start at 1 again, and a
// breaker left open by an earlier test would refuse the new test's calls.
func Reset() {
	shared.mu.Lock()
	defer shared.mu.Unlock()
	shared.entries = map[string]*entry{}
}

// States returns the health states of the process-wide registry.
func States() []State { return shared.States() }

// HTTPOutcome classifies a transport error, a body read error or a status.
// caller is the context of the whole operation, not of one attempt.
func HTTPOutcome(status int, err error, caller context.Context) Outcome {
	if caller != nil && caller.Err() != nil {
		return Canceled
	}
	if err != nil {
		return TransientFailure
	}
	if status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= 500 {
		return TransientFailure
	}
	if status >= 200 && status < 300 {
		return Success
	}
	return TerminalFailure
}

// Begin admits a call to a closed endpoint, or the one probe of a half-open
// endpoint.
func (r *Registry) Begin(ctx context.Context, target Target) (*Operation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	id := r.identity(target)
	now := r.now()
	r.mu.Lock()
	defer r.mu.Unlock()
	e := r.entries[id]
	if e == nil {
		if !r.evict() {
			return &Operation{}, nil
		}
		e = &entry{kind: target.Kind}
		r.entries[id] = e
	}
	e.touched = now
	if !e.openUntil.IsZero() {
		if now.Before(e.openUntil) || e.probe || e.active != 0 {
			return nil, ErrSuppressed
		}
		e.probe = true
	}
	e.active++
	return &Operation{r: r, id: id}, nil
}

// States returns only the endpoints whose breaker is open or probing.
func (r *Registry) States() []State {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]State, 0)
	for id, e := range r.entries {
		if e.openUntil.IsZero() && !e.probe {
			continue
		}
		out = append(out, State{ID: id, Kind: e.kind, Status: "retrying", Reason: "It does not answer; HolyArrs tries again on its own.", OpenUntil: e.openUntil})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// AttemptContext limits one network attempt and the read of its response
// body to the earlier of the caller's deadline and timeout.
func (o *Operation) AttemptContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, timeout)
}

// Finish records the result of the operation, after the caller's own retries
// have ended.
func (o *Operation) Finish(outcome Outcome) {
	if o == nil || o.r == nil {
		return
	}
	o.once.Do(func() {
		r := o.r
		now := r.now()
		r.mu.Lock()
		defer r.mu.Unlock()
		e := r.entries[o.id]
		if e == nil {
			return
		}
		if e.active > 0 {
			e.active--
		}
		e.touched = now
		wasProbe := e.probe
		if wasProbe {
			e.probe = false
		}
		switch outcome {
		case Success, TerminalFailure:
			e.failures = 0
			e.openUntil = time.Time{}
		case TransientFailure:
			if wasProbe {
				e.openUntil = now.Add(r.cooldown)
				return
			}
			e.failures++
			if e.failures >= r.threshold {
				e.openUntil = now.Add(r.cooldown)
			}
		case Canceled:
			// A canceled probe proves neither recovery nor another failure.
			// After the old cooldown, another caller may probe.
		}
	})
}

func (r *Registry) identity(target Target) string {
	h := hmac.New(sha256.New, r.key)
	write := func(s string) {
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], uint64(len(s)))
		_, _ = h.Write(n[:])
		_, _ = h.Write([]byte(s))
	}
	write(target.Kind)
	for _, s := range target.Identity {
		write(s)
	}
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil)[:18])
}

// evict makes room for a new entry without dropping a call in progress or a
// half-open probe. It reports whether there is room.
func (r *Registry) evict() bool {
	if len(r.entries) < r.max {
		return true
	}
	var oldestID string
	var oldest time.Time
	for id, e := range r.entries {
		if e.active != 0 || e.probe {
			continue
		}
		if oldestID == "" || e.touched.Before(oldest) {
			oldestID, oldest = id, e.touched
		}
	}
	if oldestID != "" {
		delete(r.entries, oldestID)
		return true
	}
	return false
}
