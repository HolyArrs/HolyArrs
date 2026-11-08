package downloads

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

// Patch kinds.
const (
	KindPatch  = "patch"
	KindHealth = "health"
)

// PublishWindow is how long changes of one client gather before one patch
// goes out. The live protocol asks for 250 to 500 ms.
var PublishWindow = 300 * time.Millisecond

// healthEvery is how often every client's health is checked for events.
var healthEvery = time.Second

// Patch is one message on the "dl" live topic. A patch covers fromRev to
// toRev of one client's store rev and carries only the fields that changed
// between the two states; snapshots are always read over REST.
type Patch struct {
	Kind    string           `json:"kind"`
	Boot    string           `json:"boot,omitempty"`
	Client  int64            `json:"client"`
	FromRev uint64           `json:"fromRev,omitempty"`
	ToRev   uint64           `json:"toRev,omitempty"`
	Upsert  []map[string]any `json:"upsert,omitempty"`
	Remove  []string         `json:"remove,omitempty"`
	Totals  *PatchTotals     `json:"totals,omitempty"`
	// Fields below are set on health events only.
	Health string     `json:"health,omitempty"`
	Name   string     `json:"name,omitempty"`
	Stale  bool       `json:"stale,omitempty"`
	SeenAt *time.Time `json:"seen_at,omitempty"`
}

// PatchTotals is the speed over every client, sent to managers only.
type PatchTotals struct {
	Speed   int64 `json:"speed"`
	UpSpeed int64 `json:"up_speed"`
}

// SnapshotPublisher batches store changes per client into patches. Build makes and
// sends the patch for one client from its state at fromRev to its state at
// toRev; Health checks every client's health. Both run on the publisher's
// goroutine only.
type SnapshotPublisher struct {
	Store  *Store
	Build  func(ctx context.Context, client int64, fromRev, toRev uint64, prev, cur map[string]Job)
	Health func(ctx context.Context)
	Now    func() time.Time

	mu    sync.Mutex
	base  map[int64]pubBase
	due   map[int64]time.Time
	wake  chan struct{}
	ready bool
}

// pubBase is a client's state at the last patch.
type pubBase struct {
	rev   uint64
	items map[string]Job
}

// MergePatch folds a newer dl message into an older one still waiting for a
// slow stream. Two patches in sequence become one patch over both windows.
// Anything else keeps the newer message, and the browser reads the page
// again when it sees the rev gap.
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
			// Removed and back within the window, so the newer upsert is a whole row.
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

// Note records that client id changed from prev, which was at prevRev. The
// first note of a client sets its base, and the patch is due one window later.
func (p *SnapshotPublisher) Note(id int64, prevRev uint64, prev map[string]Job) {
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

// Flush builds the patches that are due, or all of them when force is set.
func (p *SnapshotPublisher) Flush(ctx context.Context, force bool) {
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
			continue // the client was dropped or reset, so browsers read the page again on the gap
		}
		if p.Build != nil {
			p.Build(ctx, id, b.rev, cur.Rev, b.items, cur.Items)
		}
	}
}

// Run sends patches and health events until ctx ends.
func (p *SnapshotPublisher) Run(ctx context.Context) {
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

// now returns the clock, which tests can replace.
func (p *SnapshotPublisher) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

// init makes the maps on first use, so the zero value works.
func (p *SnapshotPublisher) init() {
	if !p.ready {
		p.base, p.due, p.wake, p.ready = map[int64]pubBase{}, map[int64]time.Time{}, make(chan struct{}, 1), true
	}
}

// next returns when the earliest patch is due, or zero when none is.
func (p *SnapshotPublisher) next() time.Time {
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
