package musicscore

import (
	"sort"
	"sync"
	"time"
)

const (
	// PlanMax is the most rows a plan holds.
	PlanMax = 30
	// PlanBatch is how many rows one page fills.
	PlanBatch = 6
	// PlanVersions is how many plan versions are kept per member and view.
	PlanVersions = 2
	// PartialTTL is how long a plan built while a source was down stays usable.
	PartialTTL = 10 * time.Minute
	// planOffset is the number of slots before the plan's first row, taken by
	// the Spotlight.
	planOffset = 1
)

// Plan is one member's row plan for one day and view. Rows already served
// never change, and the unserved slots are re-ranked after each batch.
type Plan struct {
	mu      sync.Mutex
	Version int
	Member  int64
	Date    string
	Partial bool
	Built   time.Time
	// Meta is the caller's own data for the plan.
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

// Loader fills a cold row's items when its slot is reached.
type Loader func(Row) Row

// PlanKey names one member's plan for one UTC day and view.
type PlanKey struct {
	Member int64
	Date   string
	View   string
}

// PlanStore keeps plans, at most PlanVersions per key, until 04:00 UTC the
// next day. It is separate from Days so that a library change marks plans
// stale for the next first-page load without dropping a plan that an open
// page is still paging.
type PlanStore struct {
	mu    sync.Mutex
	m     map[PlanKey][]*Plan
	last  map[PlanKey]int
	stale map[PlanKey]bool
}

// NewPlan orders the candidate rows. Head rows come first in the order
// given, then the rest greedily by strength under the variety rules: no two
// adjacent rows of one family, no family over its maximum (maxOf, 1 when
// absent), at most 2 chart rows in any 6 consecutive slots, and a discovery
// row at every 4th slot from the 8th when one remains. seen may hold keys
// already placed, such as the Spotlight. Rows that do not fit become
// reserves.
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

// Page returns batch k (zero-based), filling it on first ask, and whether
// more rows follow.
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

// All fills and returns every row, for households with few rows.
func (p *Plan) All(load Loader) []Row {
	p.mu.Lock()
	defer p.mu.Unlock()
	for len(p.slots) > 0 {
		p.fill(PlanMax, load)
	}
	return append([]Row{}, p.served...)
}

// Total returns the plan's row count when it was made. It stays steady for
// the day even if a slot closes later.
func (p *Plan) Total() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.planned
}

// Planned returns the ids of the served rows followed by the unserved
// slots, in order. It is meant for tests.
func (p *Plan) Planned() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append(p.servedIDs(), p.slots...)
}

// Current returns the newest live plan for k, unless it is stale.
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

// Get returns one pinned version while it is kept and live.
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

// Put keeps p as the newest version for k and evicts the oldest past
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

// MarkStale marks one member's plans stale, or everyone's when member is
// negative.
func (s *PlanStore) MarkStale(member int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k := range s.m {
		if member < 0 || k.Member == member {
			s.stale[k] = true
		}
	}
}

// MarkStaleKey marks the plans for one key stale.
func (s *PlanStore) MarkStaleKey(k PlanKey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stale == nil {
		return
	}
	if _, ok := s.m[k]; ok {
		s.stale[k] = true
	}
}

// Newest returns the member's newest kept plan for view on any kept day,
// even a stale or expired one. It is the last good plan, served while a new
// one is built.
func (s *PlanStore) Newest(member int64, view string, now time.Time) (*Plan, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweep(now)
	var best *Plan
	bestDate := ""
	for k, l := range s.m {
		if k.Member != member || k.View != view || len(l) == 0 || k.Date < bestDate {
			continue
		}
		best, bestDate = l[len(l)-1], k.Date
	}
	return best, best != nil
}

// rOf returns a row's strength over its items not yet placed. A cold row
// with no items yet is guessed at half its prior, and is filled when its
// slot is reached.
func (p *Plan) rOf(r Row) float64 {
	if len(r.Items) == 0 {
		return r.Prior * 0.5
	}
	var s []float64
	var unseen []Item
	for _, it := range r.Items {
		if !p.taken(it) {
			s = append(s, it.Score)
			unseen = append(unseen, it)
		}
	}
	if len(s) < r.Gate() {
		return 0
	}
	if r.ScoreFn != nil {
		return r.ScoreFn(unseen)
	}
	return Strength(s, r.Prior)
}

// taken reports whether the item is already placed, by key, release group
// or fold key.
func (p *Plan) taken(it Item) bool {
	return p.seen[it.Key] || it.Album != "" && p.seen["rg:"+it.Album] || it.FoldKey != "" && p.seen["fold:"+it.FoldKey]
}

// sortPool orders row ids by strength, then by a daily hash.
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

// maxFor returns the most rows a family may have in the plan.
func (p *Plan) maxFor(fam string) int {
	if m, ok := p.maxOf[fam]; ok {
		return m
	}
	return 1
}

// count returns how many rows in order belong to the family.
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
	// Chart rows are capped at 2 in any 6 consecutive slots.
	if r.chartLike() {
		c := 0
		for _, x := range order[max(0, len(order)-5):] {
			if p.rows[x].chartLike() {
				c++
			}
		}
		if c >= 2 {
			return false
		}
	}
	return true
}

// greedy places pool rows, already in strength order, after placed, up to
// PlanMax. It returns the new slots and the rows left over.
func (p *Plan) greedy(placed, pool []string) (slots, rest []string) {
	order := append([]string{}, placed...)
	pool = append([]string{}, pool...)
	for len(order) < PlanMax {
		slot := len(order) + 1 + planOffset
		pick := -1
		// Discovery rule: a discovery row at every 4th slot from the 8th.
		if slot >= 8 && slot%4 == 0 {
			for i, id := range pool {
				if p.rows[id].Disc && p.fits(order, id) {
					pick = i
					break
				}
			}
		}
		// Shape rule: a round or wide row in every 8 slots when one fits.
		if pick < 0 && len(order) >= 7 && !p.anyShaped(order[len(order)-7:]) {
			for i, id := range pool {
				if p.rows[id].shaped() && p.fits(order, id) && p.rOf(p.rows[id]) > 0 {
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

// anyShaped reports whether any of the rows is a round or wide row.
func (p *Plan) anyShaped(ids []string) bool {
	for _, id := range ids {
		if p.rows[id].shaped() {
			return true
		}
	}
	return false
}

// take fills one row against the shared seen and seed sets. It commits only
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
	if !r.Passes(len(kept)) || r.MinArtists > 0 && len(per) < r.MinArtists {
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

// servedIDs returns the ids of the rows served so far.
func (p *Plan) servedIDs() []string {
	ids := make([]string, len(p.served))
	for i, r := range p.served {
		ids[i] = r.ID
	}
	return ids
}

// get returns the row, loading a cold one on first use.
func (p *Plan) get(id string, load Loader) Row {
	r := p.rows[id]
	if len(r.Items) == 0 && load != nil {
		r = load(r)
		p.rows[id] = r
	}
	return r
}

// fill serves up to n more rows in plan order. A row that fails its gate
// after dedup gives its slot to the first reserve that fits. With none, the
// slot closes and later rows move up.
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
		// A failed slot changes what follows, so re-rank to keep the variety
		// rules.
		p.rerank()
	}
	p.rerank()
}

// rerank re-orders the unserved rows by their strength over what is still
// unplaced, under the same rules. Served rows stay.
func (p *Plan) rerank() {
	pool := append(append([]string{}, p.slots...), p.reserve...)
	p.sortPool(pool)
	p.slots, p.reserve = p.greedy(p.servedIDs(), pool)
}

// live reports whether a plan is usable: complete, or partial and recent.
func (s *PlanStore) live(p *Plan, now time.Time) bool {
	return !p.Partial || now.Sub(p.Built) < PartialTTL
}

// sweep drops plans for days that are over, keeping each until 04:00 UTC
// the next day so late pages still work.
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
