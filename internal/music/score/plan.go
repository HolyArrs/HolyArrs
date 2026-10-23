package musicscore

import (
	"sort"
	"sync"
	"time"
)

// The v7 row plan (flag music_rows_v7, plan 2026-10-23 §4): one member's
// rows for the day are ordered once (the plan), then filled lazily, a batch
// at a time, in plan order with one shared seen set, so dedup is exact
// across batches and a reload replays the same rows.

const (
	// PlanMax is the most rows a plan holds.
	PlanMax = 30
	// PlanBatch is the rows one page fills.
	PlanBatch = 6
	// PlanVersions is the plan versions kept per member and view.
	PlanVersions = 2
	// PartialTTL is how long a plan that lost a source is kept.
	PartialTTL = 10 * time.Minute
	// planOffset is the slots before the plan's first row (the Spotlight).
	planOffset = 1
)

// Plan is one member's row plan for one day and view. Served rows never
// change; unserved slots are re-ranked after each batch.
type Plan struct {
	mu      sync.Mutex
	Version int
	Member  int64
	Date    string
	Partial bool
	Built   time.Time
	// Meta is the caller's data for the plan (cards, names).
	Meta    any
	maxOf   map[string]int
	rows    map[string]Row
	slots   []string
	reserve []string
	seen    map[string]bool
	seeds   map[string]bool
	served  []Row
	planned int
}

// NewPlan orders the candidate rows: head rows first (in the order given,
// when present), then the rest greedily by R under the variety rules: no
// two adjacent rows of one family, no family over its maximum (maxOf, 1
// when absent), at most 2 chart rows in any 6 consecutive slots, and a
// discovery row at every 4th slot from 8 when one remains. seen may hold
// keys already placed (the Spotlight). The rest become reserves.
func NewPlan(rows []Row, head []string, maxOf map[string]int, member int64, date string, seen map[string]bool) *Plan {
	p := &Plan{Member: member, Date: date, maxOf: maxOf, rows: map[string]Row{}, seen: map[string]bool{}, seeds: map[string]bool{}}
	for k, v := range seen {
		p.seen[k] = v
	}
	var pool []string
	for _, r := range rows {
		if _, dup := p.rows[r.ID]; dup {
			continue
		}
		p.rows[r.ID] = r
		pool = append(pool, r.ID)
	}
	var placed []string
	for _, id := range head {
		for i, x := range pool {
			if x == id {
				placed = append(placed, id)
				pool = append(pool[:i], pool[i+1:]...)
				break
			}
		}
	}
	p.sortPool(pool)
	tail, rest := p.greedy(placed, pool)
	p.slots = append(placed, tail...)
	p.reserve = rest
	p.planned = len(p.slots)
	return p
}

// rOf is a row's R over its items not yet placed; a row with no items yet
// (cold: filled when its slot is reached) is prior · 0.5.
func (p *Plan) rOf(r Row) float64 {
	if len(r.Items) == 0 {
		return r.Prior * 0.5
	}
	var s []float64
	for _, it := range r.Items {
		if !p.taken(it) {
			s = append(s, it.Score)
		}
	}
	if len(s) < r.Gate() {
		return 0
	}
	return Strength(s, r.Prior)
}

func (p *Plan) taken(it Item) bool {
	return p.seen[it.Key] || it.Album != "" && p.seen["rg:"+it.Album] || it.FoldKey != "" && p.seen["fold:"+it.FoldKey]
}

func (p *Plan) sortPool(pool []string) {
	r := map[string]float64{}
	for _, id := range pool {
		r[id] = p.rOf(p.rows[id])
	}
	sort.SliceStable(pool, func(i, j int) bool {
		if r[pool[i]] != r[pool[j]] {
			return r[pool[i]] > r[pool[j]]
		}
		return Tie(p.Member, p.Date, pool[i]) < Tie(p.Member, p.Date, pool[j])
	})
}

func (p *Plan) maxFor(fam string) int {
	if m, ok := p.maxOf[fam]; ok {
		return m
	}
	return 1
}

func (p *Plan) count(order []string, fam string) int {
	n := 0
	for _, x := range order {
		if p.rows[x].family() == fam {
			n++
		}
	}
	return n
}

// fits reports whether row id may follow the rows in order.
func (p *Plan) fits(order []string, id string) bool {
	r := p.rows[id]
	fam := r.family()
	n := 0
	for _, x := range order {
		if p.rows[x].family() == fam {
			n++
		}
	}
	if n >= p.maxFor(fam) {
		return false
	}
	if len(order) > 0 && p.rows[order[len(order)-1]].family() == fam {
		return false
	}
	if r.Prior == PriorChart {
		c := 0
		for _, x := range order[max(0, len(order)-5):] {
			if p.rows[x].Prior == PriorChart {
				c++
			}
		}
		if c >= 2 {
			return false
		}
	}
	return true
}

// greedy places pool rows (already in R order) after placed, up to PlanMax.
func (p *Plan) greedy(placed, pool []string) (slots, rest []string) {
	order := append([]string{}, placed...)
	pool = append([]string{}, pool...)
	for len(order) < PlanMax {
		slot := len(order) + 1 + planOffset
		pick := -1
		if slot >= 8 && slot%4 == 0 {
			for i, id := range pool {
				if p.rows[id].Disc && p.fits(order, id) {
					pick = i
					break
				}
			}
		}
		if pick < 0 {
			for i, id := range pool {
				if p.fits(order, id) {
					pick = i
					break
				}
			}
		}
		if pick < 0 {
			break
		}
		order = append(order, pool[pick])
		slots = append(slots, pool[pick])
		pool = append(pool[:pick], pool[pick+1:]...)
	}
	return slots, pool
}

// take fills one row against the shared seen and seed sets; it commits only
// when the row passes its gate.
func (p *Plan) take(r Row) (Row, bool) {
	if r.Seed != "" && p.seeds[r.Seed] {
		return r, false
	}
	per := map[string]int{}
	local := map[string]bool{}
	var kept []Item
	for _, it := range r.Items {
		if len(kept) >= RowMax {
			break
		}
		if p.taken(it) || local[it.Key] || it.Album != "" && local["rg:"+it.Album] || it.FoldKey != "" && local["fold:"+it.FoldKey] {
			continue
		}
		if !r.ArtistRow && it.Artist != "" && per[it.Artist] >= PerArtist {
			continue
		}
		per[it.Artist]++
		kept = append(kept, it)
		local[it.Key] = true
		if it.Album != "" {
			local["rg:"+it.Album] = true
		}
		if it.FoldKey != "" {
			local["fold:"+it.FoldKey] = true
		}
	}
	if !r.Passes(len(kept)) {
		return r, false
	}
	for k := range local {
		p.seen[k] = true
	}
	if r.Seed != "" {
		p.seeds[r.Seed] = true
	}
	r.Items = kept
	return r, true
}

func (p *Plan) servedIDs() []string {
	ids := make([]string, len(p.served))
	for i, r := range p.served {
		ids[i] = r.ID
	}
	return ids
}

// load fills a cold row's items when its slot is reached.
type Loader func(Row) Row

func (p *Plan) get(id string, load Loader) Row {
	r := p.rows[id]
	if len(r.Items) == 0 && load != nil {
		r = load(r)
		p.rows[id] = r
	}
	return r
}

// fill serves up to n more rows in plan order. A row that fails its gate
// after dedup gives its slot to the first reserve that fits; with none the
// slot closes and later rows move up. Unserved rows are re-ranked after.
func (p *Plan) fill(n int, load Loader) {
	done := 0
	for done < n && len(p.slots) > 0 {
		id := p.slots[0]
		p.slots = p.slots[1:]
		if r, ok := p.take(p.get(id, load)); ok {
			p.served = append(p.served, r)
			done++
			continue
		}
		for i := 0; i < len(p.reserve); i++ {
			rid := p.reserve[i]
			if !p.fits(p.servedIDs(), rid) || p.count(p.slots, p.rows[rid].family()) > 0 && p.count(append(p.servedIDs(), p.slots...), p.rows[rid].family()) >= p.maxFor(p.rows[rid].family()) {
				continue
			}
			if r, ok := p.take(p.get(rid, load)); ok {
				p.reserve = append(p.reserve[:i], p.reserve[i+1:]...)
				p.served = append(p.served, r)
				done++
				break
			}
		}
		// A failed slot changes what follows: re-rank the rest so the next
		// rows still keep the variety rules.
		p.rerank()
	}
	p.rerank()
}

// rerank re-orders the unserved rows (slots and reserves) by their R over
// what is still unplaced, under the same rules; served rows stay.
func (p *Plan) rerank() {
	pool := append(append([]string{}, p.slots...), p.reserve...)
	p.sortPool(pool)
	p.slots, p.reserve = p.greedy(p.servedIDs(), pool)
}

// Page answers batch k (0-based): its rows, filled on first ask, and
// whether more rows follow.
func (p *Plan) Page(k int, load Loader) ([]Row, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	end := (k + 1) * PlanBatch
	if len(p.served) < end && len(p.slots) > 0 {
		p.fill(end-len(p.served), load)
	}
	from := min(k*PlanBatch, len(p.served))
	to := min(end, len(p.served))
	return append([]Row{}, p.served[from:to]...), len(p.served) > to || len(p.slots) > 0
}

// All fills and answers every row (households with few rows: one answer).
func (p *Plan) All(load Loader) []Row {
	p.mu.Lock()
	defer p.mu.Unlock()
	for len(p.slots) > 0 {
		p.fill(PlanMax, load)
	}
	return append([]Row{}, p.served...)
}

// Total is the plan's row count when it was made (stable for the day; a
// slot that closes later does not change it).
func (p *Plan) Total() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.planned
}

// Planned answers the unserved slots' row ids in order (tests).
func (p *Plan) Planned() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append(p.servedIDs(), p.slots...)
}

// PlanKey is one member's plan key: day (UTC) and view (region).
type PlanKey struct {
	Member int64
	Date   string
	View   string
}

// PlanStore keeps the plans, at most PlanVersions per key, until 04:00 UTC
// the next day. It is separate from Days: a library change marks plans
// stale (the next page-1 load builds a new version) and never drops one an
// open page is paging.
type PlanStore struct {
	mu    sync.Mutex
	m     map[PlanKey][]*Plan
	last  map[PlanKey]int
	stale map[PlanKey]bool
}

func (s *PlanStore) live(p *Plan, now time.Time) bool {
	return !p.Partial || now.Sub(p.Built) < PartialTTL
}

func (s *PlanStore) sweep(now time.Time) {
	for k := range s.m {
		d, err := time.Parse("2006-01-02", k.Date)
		if err != nil || !now.Before(d.Add(28*time.Hour)) {
			delete(s.m, k)
			delete(s.last, k)
			delete(s.stale, k)
		}
	}
}

// Current answers the newest live plan, unless it is stale.
func (s *PlanStore) Current(k PlanKey, now time.Time) (*Plan, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweep(now)
	l := s.m[k]
	if len(l) == 0 || s.stale[k] {
		return nil, false
	}
	p := l[len(l)-1]
	if !s.live(p, now) {
		return nil, false
	}
	return p, true
}

// Get answers one pinned version while it is kept and live.
func (s *PlanStore) Get(k PlanKey, version int, now time.Time) (*Plan, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweep(now)
	for _, p := range s.m[k] {
		if p.Version == version {
			return p, s.live(p, now)
		}
	}
	return nil, false
}

// Put keeps p as the key's newest version, evicting the oldest past
// PlanVersions.
func (s *PlanStore) Put(k PlanKey, p *Plan, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m, s.last, s.stale = map[PlanKey][]*Plan{}, map[PlanKey]int{}, map[PlanKey]bool{}
	}
	s.sweep(now)
	s.last[k]++
	p.Version = s.last[k]
	p.Built = now
	l := append(s.m[k], p)
	if len(l) > PlanVersions {
		l = l[len(l)-PlanVersions:]
	}
	s.m[k] = l
	delete(s.stale, k)
}

// MarkStale marks one member's plans stale (member < 0: everyone's).
func (s *PlanStore) MarkStale(member int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k := range s.m {
		if member < 0 || k.Member == member {
			s.stale[k] = true
		}
	}
}
