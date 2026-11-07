package dlsync

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs/internal/downloader"
)

// Push updates (topic "dl" on /live). Each client's store rev is its
// sequence: a patch carries fromRev..toRev and only the fields that changed
// between the two states. Snapshots are always read over REST.

// Patch kinds.
const (
	KindPatch  = "patch"
	KindHealth = "health"
)

// Patch is one dl message.
type Patch struct {
	Kind    string           `json:"kind"`
	Boot    string           `json:"boot,omitempty"`
	Client  int64            `json:"client"`
	FromRev uint64           `json:"fromRev,omitempty"`
	ToRev   uint64           `json:"toRev,omitempty"`
	Upsert  []map[string]any `json:"upsert,omitempty"`
	Remove  []string         `json:"remove,omitempty"`
	Totals  *PatchTotals     `json:"totals,omitempty"`
	// Health events: the client's health as the Downloads view words it.
	Health string     `json:"health,omitempty"`
	Name   string     `json:"name,omitempty"`
	Stale  bool       `json:"stale,omitempty"`
	SeenAt *time.Time `json:"seen_at,omitempty"`
}

// PatchTotals are the speeds over every client (managers only).
type PatchTotals struct {
	Speed   int64 `json:"speed"`
	UpSpeed int64 `json:"up_speed"`
}

// MergePatch folds a newer dl message into one still waiting for a slow
// stream (live.Options.Merge). Two patches in sequence become one patch
// over both windows; anything else keeps the newer message, and the
// browser reads the page again when it sees the rev gap.
func MergePatch(old, cur json.RawMessage) json.RawMessage {
	var a, b Patch
	if json.Unmarshal(old, &a) != nil || json.Unmarshal(cur, &b) != nil {
		return cur
	}
	if a.Kind != KindPatch || b.Kind != KindPatch || a.Client != b.Client || a.Boot != b.Boot || a.ToRev != b.FromRev {
		return cur
	}
	out := Patch{Kind: KindPatch, Boot: b.Boot, Client: b.Client, FromRev: a.FromRev, ToRev: b.ToRev, Totals: b.Totals}
	if out.Totals == nil {
		out.Totals = a.Totals
	}
	order := []string{}
	ups := map[string]map[string]any{}
	removed := map[string]bool{}
	for _, k := range a.Remove {
		removed[k] = true
	}
	add := func(u map[string]any) {
		k, _ := u["key"].(string)
		if k == "" {
			return
		}
		if removed[k] {
			// Gone and back within the window: the newer upsert is a whole row.
			delete(removed, k)
		}
		m := ups[k]
		if m == nil {
			m = map[string]any{}
			ups[k] = m
			order = append(order, k)
		}
		for f, v := range u {
			m[f] = v
		}
	}
	for _, u := range a.Upsert {
		add(u)
	}
	for _, k := range b.Remove {
		removed[k] = true
		delete(ups, k)
	}
	for _, u := range b.Upsert {
		add(u)
	}
	for _, k := range order {
		if m := ups[k]; m != nil {
			out.Upsert = append(out.Upsert, m)
		}
	}
	for _, k := range a.Remove {
		if removed[k] {
			out.Remove = append(out.Remove, k)
			delete(removed, k)
		}
	}
	for _, k := range b.Remove {
		if removed[k] {
			out.Remove = append(out.Remove, k)
			delete(removed, k)
		}
	}
	data, err := json.Marshal(out)
	if err != nil {
		return cur
	}
	return data
}

// PublishWindow is how long changes of one client gather before one patch
// goes out (250-500 ms per the protocol).
var PublishWindow = 300 * time.Millisecond

// healthEvery is how often clients' health is looked at for health events.
var healthEvery = time.Second

// Publisher batches store changes per client into patches. Build makes and
// sends the patch for one client from its state at fromRev to its state at
// toRev; Health looks at every client's health. Both run on the
// publisher's goroutine only.
type Publisher struct {
	Store  *Store
	Build  func(ctx context.Context, client int64, fromRev, toRev uint64, prev, cur map[string]downloader.Job)
	Health func(ctx context.Context)
	Now    func() time.Time

	mu    sync.Mutex
	base  map[int64]pubBase
	due   map[int64]time.Time
	wake  chan struct{}
	ready bool
}

type pubBase struct {
	rev   uint64
	items map[string]downloader.Job
}

func (p *Publisher) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

func (p *Publisher) init() {
	if !p.ready {
		p.base, p.due, p.wake, p.ready = map[int64]pubBase{}, map[int64]time.Time{}, make(chan struct{}, 1), true
	}
}

// Note records that client id changed from prev (at prevRev). The first
// note of a client sets its base; the patch is due one window later.
func (p *Publisher) Note(id int64, prevRev uint64, prev map[string]downloader.Job) {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.init()
	if _, ok := p.base[id]; !ok {
		p.base[id] = pubBase{rev: prevRev, items: prev}
	}
	if _, ok := p.due[id]; !ok {
		p.due[id] = p.now().Add(PublishWindow)
	}
	p.mu.Unlock()
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

// Flush builds the patches that are due at now (all of them when force).
func (p *Publisher) Flush(ctx context.Context, force bool) {
	p.mu.Lock()
	p.init()
	now := p.now()
	var ids []int64
	for id, at := range p.due {
		if force || !now.Before(at) {
			ids = append(ids, id)
			delete(p.due, id)
		}
	}
	p.mu.Unlock()
	for _, id := range ids {
		cur := p.Store.Current(id)
		p.mu.Lock()
		b := p.base[id]
		p.base[id] = pubBase{rev: cur.Rev, items: cur.Items}
		p.mu.Unlock()
		if cur.Rev <= b.rev {
			continue // dropped or reset: browsers read the page again on the gap
		}
		if p.Build != nil {
			p.Build(ctx, id, b.rev, cur.Rev, b.items, cur.Items)
		}
	}
}

// next is when the earliest patch is due (zero when none).
func (p *Publisher) next() time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	var t time.Time
	for _, at := range p.due {
		if t.IsZero() || at.Before(t) {
			t = at
		}
	}
	return t
}

// Run sends patches until ctx ends.
func (p *Publisher) Run(ctx context.Context) {
	p.mu.Lock()
	p.init()
	wake := p.wake
	p.mu.Unlock()
	beat := time.NewTicker(healthEvery)
	defer beat.Stop()
	for {
		var timer <-chan time.Time
		var t *time.Timer
		if at := p.next(); !at.IsZero() {
			t = time.NewTimer(max(at.Sub(p.now()), 0))
			timer = t.C
		}
		select {
		case <-ctx.Done():
		case <-wake:
		case <-timer:
			p.Flush(ctx, false)
		case <-beat.C:
			if p.Health != nil {
				p.Health(ctx)
			}
		}
		if t != nil {
			t.Stop()
		}
		if ctx.Err() != nil {
			return
		}
	}
}
