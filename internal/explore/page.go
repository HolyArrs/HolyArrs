package explore

import (
	"encoding/binary"
	"hash/fnv"
	"sort"
)

// HeadRows is the head's most rows: Just arrived, Top 10, Your asks, Top
// picks, Trending, Coming soon, then the best tail row (revision 4).
const HeadRows = 7

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

// headAnchors are the fixed head slots, in order.
var headAnchors = []string{TypeJustArrived, TypeTop10, TypeYourAsks, TypeTopPicks, TypeTrending, TypeComingSoon}

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
// title at most once in the head; at most twice on the page and at least
// 4 rows apart; once in a row and one per franchise. keepAll (Top 10)
// keeps its list whole. Items past the shown ones fill gaps.
func (l *ledger) fill(r Row, at int, head, keepAll bool) Row {
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
		if !keepAll {
			if franchise[it.Franchise()] {
				continue
			}
			seen := l.rows[k]
			if len(seen) > 0 && (head || len(seen) >= 2 || at-seen[len(seen)-1] < 4) {
				continue
			}
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
// the head is Top 10, Top picks (3+ seeds), Trending, On your services,
// Coming soon, then the best tail row; a slot that cannot fill lets the
// next candidate up, so the head is always 6 rows when rows exist. The
// tail follows rowScore order, capped by seed count.
func Assemble(p Pool, top10 []Item, member int64) Page {
	return AssembleAt(p, top10, member, Moment{})
}

// AssembleAt is Assemble with the viewer's moment, which may lift a fitting
// tail row's score by at most BoostMax (time and occasion never change a
// row's items).
func AssembleAt(p Pool, top10 []Item, member int64, m Moment) Page {
	pg := Page{Seeds: p.Seeds, Cap: TailCap(p.Seeds, p.Kid), Kid: p.Kid, Taste: p.Taste}
	by := map[string]Row{}
	var tail []Row
	for _, r := range p.Rows {
		switch r.Type {
		case TypeJustArrived, TypeYourAsks, TypeTopPicks, TypeTrending, TypeComingSoon:
			by[r.Type] = r
		default:
			tail = append(tail, r)
		}
	}
	ordered := orderTail(tail, member, p.Date, m, p.Kid)
	if p.Seeds < 3 {
		// Cold start: Best new releases and Hidden gems take the head's last slots.
		sort.SliceStable(ordered, func(i, j int) bool {
			ci := ordered[i].Type == TypeBest || ordered[i].Type == TypeHidden
			cj := ordered[j].Type == TypeBest || ordered[j].Type == TypeHidden
			return ci && !cj
		})
	}
	l := &ledger{rows: map[string][]int{}}
	at := 0
	place := func(r Row, head, keepAll bool) bool {
		got := l.fill(r, at, head, keepAll)
		if len(got.Items) < MinItems(r.Type) {
			return false
		}
		l.record(got, at)
		at++
		if head {
			pg.Head = append(pg.Head, got)
		} else {
			pg.Tail = append(pg.Tail, got)
		}
		return true
	}
	if len(top10) > 0 {
		by[TypeTop10] = Row{ID: TypeTop10, Type: TypeTop10, Title: RowTitle(TypeTop10, ""), Items: top10}
	}
	for _, t := range headAnchors {
		r, ok := by[t]
		if !ok || (t == TypeTopPicks && p.Seeds < 3) {
			continue
		}
		place(r, true, t == TypeTop10)
	}
	i := 0
	for ; i < len(ordered) && len(pg.Head) < HeadRows; i++ {
		place(ordered[i], true, false)
	}
	for ; i < len(ordered) && len(pg.Tail) < pg.Cap; i++ {
		place(ordered[i], false, false)
	}
	return pg
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
