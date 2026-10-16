// Package outside contains the small circuit breaker used at calls to linked
// and public services. It does not retry calls; callers keep their existing
// retry and pacing rules and report one result for the completed operation.
package outside

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"net"
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

// Outcome is the availability result of one completed logical operation.
type Outcome uint8

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

// Target identifies an endpoint without retaining its address or credentials.
// Identity values may contain sensitive configuration: only their keyed digest
// is kept.
type Target struct {
	Kind     string
	Identity []string
}

// State is the safe health view of an endpoint currently being suppressed or
// probed. ID is an opaque process-local digest.
type State struct {
	ID     string
	Kind   string
	Status string
	Reason string
}

type entry struct {
	kind      string
	failures  int
	openUntil time.Time
	probe     bool
	active    int
	touched   time.Time
}

// Registry holds bounded process-local breaker state.
type Registry struct {
	mu        sync.Mutex
	key       []byte
	entries   map[string]*entry
	threshold int
	cooldown  time.Duration
	max       int
	now       func() time.Time
}

// New makes an independent registry. key must be an unpredictable process key;
// an empty key is replaced with a random one.
func New(key []byte, opt Options) *Registry {
	if len(key) == 0 {
		key = make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			panic("outside: random identity key unavailable")
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

var shared = New(nil, Options{})

// Begin admits one logical operation through the process registry.
func Begin(ctx context.Context, target Target) (*Operation, error) {
	return shared.Begin(ctx, target)
}

// States returns the process registry's safe health states.
func States() []State { return shared.States() }

// Operation is one admitted logical call. Finish is safe to call more than
// once; only the first result changes breaker state.
type Operation struct {
	r    *Registry
	id   string
	once sync.Once
}

// Begin admits a closed endpoint call or the only half-open probe.
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

// AttemptContext gives one existing network attempt and its response-body read
// the earlier of the caller's deadline and timeout.
func (o *Operation) AttemptContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, timeout)
}

// Finish records the completed logical operation after the caller's own retries
// have finished.
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
			// A canceled probe proves neither recovery nor another failure. Once
			// the old cooldown has elapsed, another caller may probe.
		}
	})
}

// HTTPOutcome classifies transport, response-body and status results. caller is
// the original operation context, not the per-attempt deadline context.
func HTTPOutcome(status int, err error, caller context.Context) Outcome {
	if caller != nil && caller.Err() != nil {
		return Canceled
	}
	if err != nil {
		var ne net.Error
		if errors.As(err, &ne) || errors.Is(err, context.DeadlineExceeded) {
			return TransientFailure
		}
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

// States returns only endpoints whose breaker is open or probing.
func (r *Registry) States() []State {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]State, 0)
	for id, e := range r.entries {
		if e.openUntil.IsZero() && !e.probe {
			continue
		}
		out = append(out, State{ID: id, Kind: e.kind, Status: "retrying", Reason: "It does not answer; HolyArrs tries again on its own."})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].ID < out[j].ID
	})
	return out
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

// evict makes room without removing calls in progress or a half-open probe.
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
