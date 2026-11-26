package hooks

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"sync"
	"time"
)

// Errors a caller of a provider hook may see.
var (
	ErrNoProvider = errors.New("the plugin is not running or does not provide this")
	ErrTimeout    = errors.New("the plugin did not answer in time")
	ErrDropped    = errors.New("the plugin was stopped")
	ErrBusy       = errors.New("the plugin has too much work waiting")
)

// Job is one piece of work handed to a plugin.
type Job struct {
	ID      string          `json:"id"`
	Op      string          `json:"op"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// Answer is a plugin's reply to one Job.
type Answer struct {
	Error string          `json:"error,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
}

const (
	queueMax   = 64               // jobs waiting per plugin and hook
	liveWithin = 90 * time.Second // a provider that polled this recently is live
)

type waiter struct {
	plugin, hook string
	ch           chan Answer
}

type lane struct {
	jobs chan Job
	seen time.Time
	ctx  context.Context
	stop context.CancelFunc
}

// Broker hands work to plugins and their answers back. Safe for concurrent
// use; the zero value is not usable, use NewBroker.
type Broker struct {
	// Granted reports the scopes a plugin holds right now; nil grants none.
	Granted func(plugin string) []string
	// Covers says whether held covers need (accounts.PluginScopesCover).
	Covers func(held []string, need string) bool

	mu      sync.Mutex
	lanes   map[string]*lane // plugin + "\x00" + hook
	waiting map[string]waiter
	now     func() time.Time
}

// NewBroker makes an empty Broker.
func NewBroker() *Broker {
	return &Broker{lanes: map[string]*lane{}, waiting: map[string]waiter{}, now: time.Now}
}

func key(plugin, hook string) string { return plugin + "\x00" + hook }

// holds says whether plugin holds the scope for hook right now.
func (b *Broker) holds(plugin, hook string) bool {
	need, ok := Scopes[hook]
	if !ok || b.Granted == nil || b.Covers == nil {
		return false
	}
	return b.Covers(b.Granted(plugin), need)
}

func (b *Broker) laneLocked(plugin, hook string) *lane {
	k := key(plugin, hook)
	l := b.lanes[k]
	if l == nil {
		ctx, stop := context.WithCancel(context.Background())
		l = &lane{jobs: make(chan Job, queueMax), ctx: ctx, stop: stop}
		b.lanes[k] = l
	}
	return l
}

// Providers lists the plugins that serve hook now: they hold its scope and
// asked for work within the last 90 seconds.
func (b *Broker) Providers(hook string) []string {
	b.mu.Lock()
	var ids []string
	for k, l := range b.lanes {
		p, h := splitKey(k)
		if h == hook && b.now().Sub(l.seen) < liveWithin {
			ids = append(ids, p)
		}
	}
	b.mu.Unlock()
	out := ids[:0]
	for _, p := range ids {
		if b.holds(p, hook) {
			out = append(out, p)
		}
	}
	return out
}

func splitKey(k string) (string, string) {
	for i := 0; i < len(k); i++ {
		if k[i] == 0 {
			return k[:i], k[i+1:]
		}
	}
	return k, ""
}

// Call hands plugin one job for hook and waits up to within for its answer.
func (b *Broker) Call(ctx context.Context, plugin, hook, op string, payload any, within time.Duration) (json.RawMessage, error) {
	if !b.holds(plugin, hook) {
		return nil, ErrNoProvider
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	j := Job{ID: rand.Text(), Op: op, Payload: raw}
	ch := make(chan Answer, 1)
	b.mu.Lock()
	l := b.lanes[key(plugin, hook)]
	if l == nil || b.now().Sub(l.seen) >= liveWithin {
		b.mu.Unlock()
		return nil, ErrNoProvider
	}
	select {
	case l.jobs <- j:
	default:
		b.mu.Unlock()
		return nil, ErrBusy
	}
	b.waiting[j.ID] = waiter{plugin: plugin, hook: hook, ch: ch}
	lctx := l.ctx
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		delete(b.waiting, j.ID)
		b.mu.Unlock()
	}()
	t := time.NewTimer(within)
	defer t.Stop()
	select {
	case a := <-ch:
		if a.Error != "" {
			return nil, errors.New(clip(a.Error, 300))
		}
		return a.Data, nil
	case <-lctx.Done():
		return nil, ErrDropped
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-t.C:
		return nil, ErrTimeout
	}
}

// Send hands plugin a job that needs no answer (an event). It is dropped
// when the plugin is not a live provider or has too much waiting.
func (b *Broker) Send(plugin, hook, op string, payload any) bool {
	raw, err := json.Marshal(payload)
	if err != nil || !b.holds(plugin, hook) {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	l := b.lanes[key(plugin, hook)]
	if l == nil || b.now().Sub(l.seen) >= liveWithin {
		return false
	}
	select {
	case l.jobs <- Job{ID: rand.Text(), Op: op, Payload: raw}:
		return true
	default:
		return false
	}
}

// Next waits up to within for plugin's next job on hook. ok is false when
// none came; the plugin then simply asks again.
func (b *Broker) Next(ctx context.Context, plugin, hook string, within time.Duration) (Job, bool, error) {
	if !b.holds(plugin, hook) {
		return Job{}, false, ErrNoProvider
	}
	b.mu.Lock()
	l := b.laneLocked(plugin, hook)
	l.seen = b.now()
	b.mu.Unlock()
	t := time.NewTimer(within)
	defer t.Stop()
	select {
	case j := <-l.jobs:
		return j, true, nil
	case <-l.ctx.Done():
		return Job{}, false, ErrDropped
	case <-ctx.Done():
		return Job{}, false, nil
	case <-t.C:
		return Job{}, false, nil
	}
}

// Answer delivers plugin's answer to job. A job of another plugin or hook
// is refused, so one plugin can't answer for another.
func (b *Broker) Answer(plugin, hook, job string, a Answer) error {
	b.mu.Lock()
	w, ok := b.waiting[job]
	b.mu.Unlock()
	if !ok || w.plugin != plugin || w.hook != hook {
		return errors.New("no such job waiting")
	}
	select {
	case w.ch <- a:
		return nil
	default:
		return errors.New("job already answered")
	}
}

// Drop ends all of plugin's work at once: waiting callers get ErrDropped and
// it is no longer a provider. Called when its token ends (stop, block,
// remove, scope change).
func (b *Broker) Drop(plugin string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for k, l := range b.lanes {
		if p, _ := splitKey(k); p == plugin {
			l.stop()
			delete(b.lanes, k)
		}
	}
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}
