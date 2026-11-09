package suggest

import (
	"encoding/binary"
	"hash/fnv"
	"sort"
)

// HeadRows is the most rows the head holds: the tab's slots, then the best
// tail rows.
const HeadRows = 12

// HeadDedupe is how many rows from the top show a title at most once.
const HeadDedupe = 8

// TailPage is how many tail rows one tail page holds.
const TailPage = 4

// KidHeadRows is the most head rows a kid gets.
const KidHeadRows = 7

// SpreadWindow is how many head rows from the top never hold two charts next
// to each other.
const SpreadWindow = 4

// headOrders are each tab's head slots in order. A slot names a row type or
// one row's id, and a slot without data is skipped. Explore shows new things
// only, so Just arrived, Your asks and Coming soon stay on Home.
var headOrders = map[string][]string{
	"all":    {TypeTopServices, TypeTopPicks, TypeTrending, TypeBecauseHave, TypeBest, TypeBecauseAsked, TypeHousehold},
	"movies": {TypeTopServices, TypeTopPicks, TypeCollection, TypeTrending, TypeBecauseHave, TypeBest, TypeCinemas, TypeHousehold},
	"shows":  {TypeTopServices, TypeTopPicks, TypeNewSeasons, TypeTrending, TypeBecauseHave, TypeReturning, TypeHousehold},
}

// coldOrder is the head before there is any taste (under 3 seeds): the
// charts, Best new releases, then popular genres and Classics.
var coldOrder = []string{TypeTopServices, TypeTrending, TypeBest, TypePopularGenre, TypePopularGenre, TypeClassics}

// kidOrder is a kid's short head, with family rows instead of the charts.
var kidOrder = []string{TypeTopServices, TypeTopPicks, TypePopularGenre + ":" + KidGenres[0], TypePopularGenre + ":" + KidGenres[1], TypeBecauseHave, TypeHousehold}

// fixedRowTypes are pool rows that only show in their own head slot.
var fixedRowTypes = map[string]bool{TypeTopPicks: true, TypeTrending: true, TypeCollection: true, TypeCinemas: true, TypeNewSeasons: true, TypeReturning: true}

// Page is one member's assembled Explore page.
type Page struct {
	Head  []Row `json:"head"`
	Tail  []Row `json:"tail"`
	Seeds int   `json:"seeds"`
	Cap   int   `json:"cap"`
	// Kid, Taste and Leans carry the pool's kid flag, taste and leaned-on
	// genres for titling.
	Kid   bool     `json:"-"`
	Taste []string `json:"-"`
	Leans []string `json:"-"`
}

// ledger is the per-page dedup record of where each title showed.
type ledger struct {
	// rows maps a title key to the row indexes it showed in.
	rows map[string][]int
	// above are the Trending on streaming titles: the head skips them unless a
	// row would fall under its minimum without them.
	above map[string]bool
}

// Assemble builds the page from a pool: the tab's head slots, then the best
// tail rows. A slot that cannot fill is skipped, and the tail is capped by
// seed count.
func Assemble(p Pool, member int64) Page {
	return AssembleAt(p, member, Moment{})
}

// AssembleAt is Assemble with the viewer's moment, which may lift a fitting
// tail row's score by at most BoostMax. Time and occasion never change a
// row's items.
func AssembleAt(p Pool, member int64, m Moment) Page {
	return AssembleTabAt(p, nil, member, m)
}

// AssembleTabAt builds the page from a pool and the Trending on streaming
// titles (svcTop), which the rows under it never repeat in the first
// HeadDedupe rows. Rows follow the tab's head order with the charts spread apart.
func AssembleTabAt(p Pool, svcTop []Item, member int64, m Moment) Page {
	pg := Page{Seeds: p.Seeds, Cap: TailCap(p.Seeds, p.Kid), Kid: p.Kid, Taste: p.Taste, Leans: p.Leans}
	by := map[string]Row{}
	var tail []Row
	for _, r := range p.Rows {
		switch {
		case MinItems(r.Type) == 0:
			// A row type this build no longer makes, from an older pool.
		case fixedRowTypes[r.Type]:
			by[r.Type] = r
		default:
			tail = append(tail, r)
		}
	}
	ordered := orderTail(tail, member, p.Date, m, p.Kid)
	// The page fills this row itself, so it is never deduped against.
	by[TypeTopServices] = Row{ID: TypeTopServices, Type: TypeTopServices, Title: TabTitle(TypeTopServices, p.Switch)}
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
		l := &ledger{rows: map[string][]int{}, above: map[string]bool{}}
		for _, it := range svcTop {
			l.above[it.Key()] = true
		}
		at := 0
		var shown []Row
		place := func(r Row, toHead bool) {
			if r.Type == TypeTopServices {
				// Not counted in at, so the dedupe window covers eight rows with titles.
				out.Head = append(out.Head, r)
				shown = append(shown, r)
				return
			}
			got := l.fill(r, at, at < HeadDedupe, true)
			if len(got.Items) < MinItems(r.Type) {
				if !Popular(r.Type) {
					return
				}
				// A chart mostly on the household's services still shows, with those titles.
				if got = l.fill(r, at, at < HeadDedupe, false); len(got.Items) < MinItems(r.Type) {
					return
				}
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
		// The rest in order, but never a row right under one of its type while
		// another type is left, since a skipped row could join two.
		left := append([]Row{}, rest...)
		next := func() Row {
			last := ""
			if n := len(out.Head) + len(out.Tail); n > 0 {
				if len(out.Tail) > 0 {
					last = out.Tail[len(out.Tail)-1].Type
				} else {
					last = out.Head[len(out.Head)-1].Type
				}
			}
			k := 0
			for j, r := range left {
				if r.Type != last {
					k = j
					break
				}
			}
			r := left[k]
			left = append(left[:k], left[k+1:]...)
			return r
		}
		for len(left) > 0 && len(out.Head) < headMax {
			place(next(), true)
		}
		for len(left) > 0 && len(out.Tail) < out.Cap {
			place(next(), false)
		}
		return out, shown
	}
	// Lay the page out once to learn which head rows fill, spread the charts in
	// that order, then lay it out again in the spread order.
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

// HeadOrder returns the head slots for a tab, a kid and a seed count.
func HeadOrder(sw string, kid bool, seeds int) []string {
	switch {
	case kid:
		return kidOrder
	case seeds < 3:
		// The tab's own rows still show after the charts.
		order := append([]string{}, coldOrder[:3]...)
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

// Popular reports whether a row type is one of the three popularity charts.
func Popular(t string) bool { return t == TypeTopServices || t == TypeTrending || t == TypeBest }

// Filter applies the kid limit at response time: items allow refuses are
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

// TailSlice returns tail page n, counting from 0, with TailPage rows. more
// says whether later pages follow.
func (pg Page) TailSlice(n int) (rows []Row, more bool) {
	from := n * TailPage
	if n < 0 || from >= len(pg.Tail) {
		return []Row{}, false
	}
	to := min(from+TailPage, len(pg.Tail))
	return pg.Tail[from:to], to < len(pg.Tail)
}

// jitter returns a hash of member, date and row id mapped to 0.95 through
// 1.05, plus the raw hash. It is stable within a day so rows do not shuffle
// on every load.
func jitter(member int64, date, id string) (float64, uint64) {
	h := fnv.New64a()
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], uint64(member))
	h.Write(b[:])
	h.Write([]byte("|" + date + "|" + id))
	v := h.Sum64()
	return 0.95 + 0.10*float64(v%10007)/10006, v
}

// spread moves charts apart. In the first SpreadWindow rows, a chart right
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

// orderTail orders tail candidates by row score with the daily jitter, then
// by diversity: each row of a type already picked scores x0.75 per pick, and
// a row sharing a seed or genre with a picked row scores x0.5. Two rows of one
// type never sit back to back while another type is left.
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

// fill takes up to RowMax items for the row at index at. A title shows at most
// once in the first HeadDedupe rows, at most twice on the page and at least 4
// rows apart, once in a row, and one per franchise (Finish the collection is
// one franchise by design). With skipAbove, no Trending on streaming title
// shows in the head. Items past the shown ones fill gaps.
func (l *ledger) fill(r Row, at int, head, skipAbove bool) Row {
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
		if skipAbove && head && l.above[k] {
			continue
		}
		inRow[k], franchise[it.Franchise()] = true, true
		out = append(out, it)
	}
	r.Items = out
	return r
}

// record notes the row index each of the row's titles showed in.
func (l *ledger) record(r Row, at int) {
	for _, it := range r.Items {
		l.rows[it.Key()] = append(l.rows[it.Key()], at)
	}
}
