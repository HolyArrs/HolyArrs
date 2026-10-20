package explore

import (
	"encoding/binary"
	"hash/fnv"
	"sort"
)

// HeadRows is the head's most rows: the tab's slots (HeadOrder),
// then the best tail rows (row order of 2026-10-20).
const HeadRows = 12

// HeadDedupe is how many rows from the top show a title at most once.
const HeadDedupe = 8

// TailPage is how many tail rows one tail page holds.
const TailPage = 4

// Page is one member's assembled Explore page.
type Page struct {
	Head  []Row `json:"head"`
	Tail  []Row `json:"tail"`
	Seeds int   `json:"seeds"`
	Cap   int   `json:"cap"`
	// Kid and Taste carry the pool's Kid flag and taste for titling.
	Kid   bool     `json:"-"`
	Taste []string `json:"-"`
}

// jitter is hash(member, date, row) mapped to 0.95-1.05, stable in a day.
func jitter(member int64, date, id string) (float64, uint64) {
	h := fnv.New64a()
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], uint64(member))
	h.Write(b[:])
	h.Write([]byte("|" + date + "|" + id))
	v := h.Sum64()
	return 0.95 + 0.10*float64(v%10007)/10006, v
}

// headOrders are each tab's head slots, in order (rows review of
// 2026-10-20); a slot without data is skipped. A slot names a row type, or
// one row's id. Explore shows new stuff only: Just arrived, Your asks and
// Coming soon are Home rows.
var headOrders = map[string][]string{
	"all":    {TypeTopServices, TypeTopPicks, TypeTrending, TypeBecauseHave, TypeBest, TypeBecauseAsked, TypeHousehold},
	"movies": {TypeTopServices, TypeTopPicks, TypeCollection, TypeTrending, TypeBecauseHave, TypeBest, TypeCinemas, TypeHousehold},
	"shows":  {TypeTopServices, TypeTopPicks, TypeNewSeasons, TypeTrending, TypeBecauseHave, TypeReturning, TypeHousehold},
}

// coldOrder is the head with no taste yet (under 3 seeds): the charts,
// Best new releases, then popular genres and Classics.
var coldOrder = []string{TypeTopServices, TypeTrending, TypeBest, TypePopularGenre, TypePopularGenre, TypeClassics}

// kidOrder is a Kid's short head: family rows instead of the charts.
var kidOrder = []string{TypeTopServices, TypeTopPicks, TypePopularGenre + ":" + KidGenres[0], TypePopularGenre + ":" + KidGenres[1], TypeBecauseHave, TypeHousehold}

// KidHeadRows is the most head rows a Kid gets.
const KidHeadRows = 7

// HeadOrder is the head slots for a tab, a Kid and a seed count.
func HeadOrder(sw string, kid bool, seeds int) []string {
	switch {
	case kid:
		return kidOrder
	case seeds < 3:
		order := append([]string{}, coldOrder[:3]...)
		// The tab's own rows still show after the charts.
		switch sw {
		case "movies":
			order = append(order, TypeCollection, TypeCinemas)
		case "shows":
			order = append(order, TypeNewSeasons, TypeReturning)
		}
		return append(order, coldOrder[3:]...)
	}
	if o, ok := headOrders[sw]; ok {
		return o
	}
	return headOrders["all"]
}

// fixedRowTypes are pool rows that only show in their head slot.
var fixedRowTypes = map[string]bool{TypeTopPicks: true, TypeTrending: true, TypeCollection: true, TypeCinemas: true, TypeNewSeasons: true, TypeReturning: true}

// Popular says a row is one of the three popularity charts.
func Popular(t string) bool { return t == TypeTopServices || t == TypeTrending || t == TypeBest }

// SpreadWindow is how many head rows from the top never hold two charts
// next to each other.
const SpreadWindow = 4

// spread moves charts apart: in the first SpreadWindow rows, a chart right
// under another trades places with the nearest row below that is not one.
func spread(rows []Row) []Row {
	out := append([]Row{}, rows...)
	for i := 1; i < min(SpreadWindow, len(out)); i++ {
		if !Popular(out[i].Type) || !Popular(out[i-1].Type) {
			continue
		}
		for j := i + 1; j < len(out); j++ {
			if !Popular(out[j].Type) {
				moved := out[j]
				copy(out[i+1:j+1], out[i:j])
				out[i] = moved
				break
			}
		}
	}
	return out
}

// orderTail orders tail candidates (5.4): rowScore with the daily jitter,
// then diversity (the next row of a type picked already ×0.75 per pick; a
// row sharing a seed or genre with a picked row ×0.5), never two rows of
// the same type back to back while another type is left.
func orderTail(rows []Row, member int64, date string, m Moment, kid bool) []Row {
	type cand struct {
		r   Row
		s   float64
		tie uint64
	}
	left := make([]cand, 0, len(rows))
	for _, r := range rows {
		j, tie := jitter(member, date, r.ID)
		left = append(left, cand{r, r.Score * j * MomentBoost(r, m, kid), tie})
	}
	sort.SliceStable(left, func(i, j int) bool {
		if left[i].s != left[j].s {
			return left[i].s > left[j].s
		}
		return left[i].tie < left[j].tie
	})
	picked := map[string]int{}
	seeds, genres := map[string]bool{}, map[string]bool{}
	var out []Row
	last := ""
	for len(left) > 0 {
		best, bestS := -1, -1.0
		for pass := 0; pass < 2 && best < 0; pass++ {
			for i, c := range left {
				if pass == 0 && c.r.Type == last {
					continue
				}
				s := c.s
				for n := 0; n < picked[c.r.Type]; n++ {
					s *= 0.75
				}
				if (c.r.Seed != "" && seeds[c.r.Seed]) || (c.r.Genre != "" && genres[c.r.Genre]) {
					s *= 0.5
				}
				if s > bestS || (s == bestS && c.tie < left[best].tie) {
					best, bestS = i, s
				}
			}
		}
		c := left[best]
		left = append(left[:best], left[best+1:]...)
		out = append(out, c.r)
		picked[c.r.Type]++
		last = c.r.Type
		if c.r.Seed != "" {
			seeds[c.r.Seed] = true
		}
		if c.r.Genre != "" {
			genres[c.r.Genre] = true
		}
	}
	return out
}

// ledger is the per-page dedup record: where each title showed.
type ledger struct {
	rows map[string][]int // key -> row indexes it showed in
}

// fill takes up to RowMax items for row index at, by the rules of 5.5: a
// title at most once in the first HeadDedupe rows; at most twice on the page and at least
// 4 rows apart; once in a row and one per franchise (Finish the
// collection is one franchise by design). Items past the shown ones fill
// gaps.
func (l *ledger) fill(r Row, at int, head bool) Row {
	out := make([]Item, 0, RowMax)
	inRow, franchise := map[string]bool{}, map[string]bool{}
	for _, it := range r.Items {
		if len(out) == RowMax {
			break
		}
		k := it.Key()
		if inRow[k] {
			continue
		}
		if franchise[it.Franchise()] && r.Type != TypeCollection {
			continue
		}
		if seen := l.rows[k]; len(seen) > 0 && (head || len(seen) >= 2 || at-seen[len(seen)-1] < 4) {
			continue
		}
		inRow[k], franchise[it.Franchise()] = true, true
		out = append(out, it)
	}
	r.Items = out
	return r
}

func (l *ledger) record(r Row, at int) {
	for _, it := range r.Items {
		l.rows[it.Key()] = append(l.rows[it.Key()], at)
	}
}

// Assemble builds the page from a pool and the region's Top 10 (5.5, 6):
// the tab's head slots (HeadOrder), then the best tail rows; a slot that
// cannot fill is skipped. The tail follows rowScore order, capped by seed
// count.
func Assemble(p Pool, top10 []Item, member int64) Page {
	return AssembleAt(p, top10, member, Moment{})
}

// AssembleAt is Assemble with the viewer's moment, which may lift a fitting
// tail row's score by at most BoostMax (time and occasion never change a
// row's items). The Top 10 items are split by kind.
func AssembleAt(p Pool, top10 []Item, member int64, m Moment) Page {
	tops := map[string][]Item{}
	for _, it := range top10 {
		tops[it.Kind] = append(tops[it.Kind], it)
	}
	return AssembleTabAt(p, tops, nil, member, m)
}

// AssembleTabAt builds the page from a pool, the region's Top 10 per kind
// ("movie", "show") and the titles of Trending on your services (svcTop:
// the rows under it never repeat them in the first HeadDedupe rows), in
// the tab's head order (HeadOrder), with the charts spread apart.
func AssembleTabAt(p Pool, tops map[string][]Item, svcTop []Item, member int64, m Moment) Page {
	pg := Page{Seeds: p.Seeds, Cap: TailCap(p.Seeds, p.Kid), Kid: p.Kid, Taste: p.Taste}
	by := map[string]Row{}
	var tail []Row
	for _, r := range p.Rows {
		switch {
		case MinItems(r.Type) == 0:
			// A row type this build no longer makes (an older pool).
		case fixedRowTypes[r.Type]:
			by[r.Type] = r
		default:
			tail = append(tail, r)
		}
	}
	ordered := orderTail(tail, member, p.Date, m, p.Kid)
	// The marker carries the Netflix Top 10 titles (for the hero only); the
	// page fills the row itself, so it is never deduped against.
	var marker []Item
	for _, kind := range Kinds(p.Switch) {
		marker = append(marker, tops[kind]...)
	}
	by[TypeTopServices] = Row{ID: TypeTopServices, Type: TypeTopServices, Title: TabTitle(TypeTopServices, p.Switch), Items: marker}
	if t, ok := by[TypeTrending]; ok {
		t.Title = TabTitle(TypeTrending, p.Switch)
		by[TypeTrending] = t
	}
	headMax := HeadRows
	if p.Kid {
		headMax = KidHeadRows
	}
	// Pick each slot's row: a fixed row by type, else the best-ordered tail
	// row whose id or type the slot names.
	var picks []Row
	for _, slot := range HeadOrder(p.Switch, p.Kid, p.Seeds) {
		if r, ok := by[slot]; ok {
			if slot != TypeTopPicks || p.Seeds >= 3 {
				picks = append(picks, r)
			}
			delete(by, slot)
			continue
		}
		for i, r := range ordered {
			if r.ID == slot || (r.Type == slot && !fixedRowTypes[slot]) {
				picks = append(picks, r)
				ordered = append(ordered[:i], ordered[i+1:]...)
				break
			}
		}
	}
	lay := func(head, rest []Row) (Page, []Row) {
		out := pg
		l := &ledger{rows: map[string][]int{}}
		for _, it := range svcTop {
			// Shown in row 2 already: the head never repeats them.
			l.rows[it.Key()] = []int{-HeadDedupe}
		}
		at := 0
		var shown []Row
		place := func(r Row, toHead bool) {
			if r.Type == TypeTopServices {
				// Not counted in at: the dedupe window covers eight rows with titles.
				out.Head = append(out.Head, r)
				shown = append(shown, r)
				return
			}
			got := l.fill(r, at, at < HeadDedupe)
			if len(got.Items) < MinItems(r.Type) {
				return
			}
			l.record(got, at)
			at++
			if toHead {
				out.Head = append(out.Head, got)
				shown = append(shown, r)
			} else {
				out.Tail = append(out.Tail, got)
			}
		}
		for _, r := range head {
			place(r, true)
		}
		i := 0
		for ; i < len(rest) && len(out.Head) < headMax; i++ {
			place(rest[i], true)
		}
		for ; i < len(rest) && len(out.Tail) < out.Cap; i++ {
			place(rest[i], false)
		}
		return out, shown
	}
	// Lay the page out once to learn which head rows fill, spread the
	// charts in that order, then lay it out again in the spread order.
	first, shown := lay(picks, ordered)
	spreadHead := spread(shown)
	same := true
	inHead := map[string]bool{}
	for i, r := range spreadHead {
		same = same && r.ID == shown[i].ID
		inHead[r.ID] = true
	}
	if same {
		return first
	}
	var rest []Row
	for _, r := range ordered {
		if !inHead[r.ID] {
			rest = append(rest, r)
		}
	}
	final, _ := lay(spreadHead, rest)
	return final
}

// Filter applies the Kid limit at response time: items allow refuses are
// dropped, and rows left under their minimum go.
func (pg Page) Filter(allow func(Item) bool) Page {
	keep := func(rows []Row) []Row {
		out := []Row{}
		for _, r := range rows {
			var items []Item
			for _, it := range r.Items {
				if allow(it) {
					items = append(items, it)
				}
			}
			if len(items) >= MinItems(r.Type) {
				r.Items = items
				out = append(out, r)
			}
		}
		return out
	}
	pg.Head, pg.Tail = keep(pg.Head), keep(pg.Tail)
	return pg
}

// TailSlice is tail page n (from 0), TailPage rows each; more says more follow.
func (pg Page) TailSlice(n int) (rows []Row, more bool) {
	from := n * TailPage
	if n < 0 || from >= len(pg.Tail) {
		return []Row{}, false
	}
	to := min(from+TailPage, len(pg.Tail))
	return pg.Tail[from:to], to < len(pg.Tail)
}
