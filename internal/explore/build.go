package explore

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Request is one outside list the pool build asks for. It carries only
// title ids, genres, dates, services and the region, never anything about
// a member.
type Request struct {
	// List: similar (recommendations + similar of Ref), trending,
	// upcoming, best (Best new releases), discover, person (credits of
	// the person named Ref).
	List  string
	Kind  string
	Ref   string
	Genre string
	// Discover filters.
	From, To         string // release dates, YYYY-MM-DD
	Services, Region string
	MinVote          float64
	MinVotes         int
	MaxVotes         int
	Sort             string // popularity (default) or rating
	// Scope is ScopeShared for region-wide lists; it is not part of the
	// cache key, so it never splits a shared list.
	Scope string
}

func (q Request) String() string {
	return fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%.1f|%d|%d|%s", q.List, q.Kind, q.Ref, q.Genre, q.From, q.To, q.Services, q.Region, q.MinVote, q.MinVotes, q.MaxVotes, q.Sort)
}

// Source answers Requests, in the source's order (best first).
type Source interface {
	List(ctx context.Context, q Request) ([]Item, error)
}

// CacheChecker is a Source that can say a Request would be answered
// without an outside call; such Requests do not count against the budget.
type CacheChecker interface {
	Cached(ctx context.Context, q Request) bool
}

// MemberBudget is the most member-specific outside calls one pool build
// makes (plan 5.2); region-wide lists are counted apart.
const MemberBudget = 30

// HouseholdTitle is one title asked for by people in the household in
// the last 30 days.
type HouseholdTitle struct {
	Item    Item
	Asks    int
	Members int
}

// Input is everything one pool build knows about a member. It never goes
// to the Source; only Requests do.
type Input struct {
	Member int64
	Kid    bool
	// Switch is all, movies or shows.
	Switch string
	Now    time.Time
	Seeds  []Seed
	// Owned and Monitored are kind:tmdb keys.
	Owned     map[string]bool
	Monitored map[string]bool
	// OwnedPeople counts Library titles per credited person.
	OwnedPeople map[string]int
	// OwnedDecadeGenre counts Library titles per "1990s|Drama".
	OwnedDecadeGenre map[string]int
	Household        []HouseholdTitle
	// JustArrived are Library adds from the last 14 days, newest first;
	// MyAsks the member's open asks, newest first.
	JustArrived, MyAsks []Item
	Members             int
	Services, Region    string
	// Coming are monitored Library releases in the next 90 days.
	Coming []Item
	// Allow is the Kid limit at candidate time; nil allows all.
	Allow func(Item) bool
	// Today is the pool already built today (its typeFreshness is kept);
	// Prev is the newest pool from an earlier day.
	Today, Prev *Pool
}

// Pool is one member's scored candidate rows for one type switch and day.
type Pool struct {
	Member int64  `json:"member"`
	Switch string `json:"switch"`
	Date   string `json:"date"`
	Seeds  int    `json:"seeds"`
	Kid    bool   `json:"kid"`
	Rows   []Row  `json:"rows"`
	// Taste are the member's strongest genres, for taste-match titles.
	Taste []string  `json:"taste,omitempty"`
	Built time.Time `json:"built"`
}

// Day is the pool date of t (UTC).
func Day(t time.Time) string { return t.UTC().Format("2006-01-02") }

// Kinds are the title kinds a switch shows.
func Kinds(sw string) []string {
	switch sw {
	case "movies":
		return []string{"movie"}
	case "shows":
		return []string{"show"}
	}
	return []string{"movie", "show"}
}

// coldGenres are used when the trending lists name too few genres.
var coldGenres = []string{"Drama", "Comedy", "Action", "Thriller", "Animation", "Documentary", "Crime", "Family"}

// coldDecades are the two region defaults for Popular <decade> <genre>.
var coldDecades = []int{1990, 2000}

const poolItems = 40

// Build fetches and scores every candidate row for one member (5.2). The
// Kid limit is applied to each list before scoring. Region-wide lists are
// asked for like any other; the Source is expected to cache them.
func Build(ctx context.Context, src Source, in Input) (Pool, error) {
	kinds := Kinds(in.Switch)
	allow := func(items []Item) []Item {
		out := make([]Item, 0, len(items))
		for _, it := range items {
			if it.Kind != "movie" && it.Kind != "show" || !kindIn(kinds, it.Kind) {
				continue
			}
			if in.Allow != nil && !in.Allow(it) {
				continue
			}
			it.Owned = in.Owned[it.Key()]
			it.Monitored = in.Monitored[it.Key()]
			out = append(out, it)
		}
		return out
	}
	// fetch asks for one list per kind and takes turns between them. A
	// person's credits cover both kinds, so they are asked for once.
	fetch := func(q Request) ([]Item, error) {
		if q.List == "person" {
			items, err := src.List(ctx, q)
			if err != nil {
				return nil, err
			}
			return allow(items), nil
		}
		var lists [][]Item
		for _, k := range kinds {
			if q.Kind != "" && q.Kind != k {
				continue
			}
			qq := q
			qq.Kind = k
			items, err := src.List(ctx, qq)
			if err != nil {
				return nil, err
			}
			lists = append(lists, allow(items))
		}
		return interleave(lists), nil
	}
	// spend keeps member-specific lists inside MemberBudget outside calls;
	// lists the Source already has cached cost nothing. Callers ask in
	// typePrior order, so the cheapest-to-lose rows are the ones dropped.
	cached, _ := src.(CacheChecker)
	used := 0
	spend := func(q Request) bool {
		if q.Shared() {
			return true
		}
		cost := 0
		for _, k := range kinds {
			if q.List == "person" {
				if cached == nil || !cached.Cached(ctx, q) {
					cost = q.Outside()
				}
				break
			}
			if q.Kind != "" && q.Kind != k {
				continue
			}
			qq := q
			qq.Kind = k
			if cached == nil || !cached.Cached(ctx, qq) {
				cost += qq.Outside()
			}
		}
		if used+cost > MemberBudget {
			return false
		}
		used += cost
		return true
	}
	seeds := []Seed{}
	for _, s := range in.Seeds {
		if kindIn(kinds, s.Kind) {
			seeds = append(seeds, s)
		}
	}
	nSeeds := SeedCount(seeds)
	profile := NewProfile(seeds)
	pool := Pool{Member: in.Member, Switch: in.Switch, Date: Day(in.Now), Seeds: nSeeds, Kid: in.Kid, Built: in.Now}
	var rows []Row
	addRow := func(r Row) {
		if len(r.Items) > 0 {
			rows = append(rows, r)
		}
	}
	var errs []string
	soft := func(err error) bool {
		if err == nil {
			return true
		}
		if ctx.Err() != nil {
			return false
		}
		errs = append(errs, err.Error())
		return false
	}

	// The member's own rows: kept in their order, not scored.
	addRow(Row{ID: TypeJustArrived, Type: TypeJustArrived, Items: allow(in.JustArrived)})
	addRow(Row{ID: TypeYourAsks, Type: TypeYourAsks, Items: allow(in.MyAsks)})

	// Region-wide rows.
	trending, err := fetch(Request{List: "trending", Scope: ScopeShared})
	if !soft(err) && ctx.Err() != nil {
		return pool, ctx.Err()
	}
	addRow(Row{ID: TypeTrending, Type: TypeTrending, Items: trending})
	upcoming, err := fetch(Request{List: "upcoming", Scope: ScopeShared})
	soft(err)
	var coming []Item
	for _, it := range append(allow(in.Coming), upcoming...) {
		if it.Kind == "movie" && it.Released != "" {
			if d, e := time.Parse("2006-01-02", it.Released); e == nil && (d.Before(in.Now.AddDate(0, 0, -1)) || d.After(in.Now.AddDate(0, 0, 90))) {
				continue
			}
		}
		coming = append(coming, it)
	}
	addRow(Row{ID: TypeComingSoon, Type: TypeComingSoon, Items: uniq(coming), Wide: true})
	svc := Request{List: "discover", Services: in.Services, Region: in.Region, Scope: ScopeShared}
	services, err := fetch(svc)
	soft(err)
	addRow(Row{ID: TypeServices, Type: TypeServices, Items: services})
	if in.Services != "" {
		q := svc
		q.From = Day(in.Now.AddDate(0, 0, -30))
		items, err := fetch(q)
		soft(err)
		addRow(Row{ID: TypeNewServices, Type: TypeNewServices, Items: items})
	}
	best, err := fetch(Request{List: "best", Scope: ScopeShared})
	soft(err)
	addRow(Row{ID: TypeBest, Type: TypeBest, Items: best})
	hidden, err := fetch(Request{List: "discover", MinVote: 7.3, MinVotes: 100, MaxVotes: 1500, Sort: "rating", Services: in.Services, Region: in.Region, Scope: ScopeShared})
	soft(err)
	addRow(Row{ID: TypeHidden, Type: TypeHidden, Items: hidden})

	// Household: at least 2 members; a Kid only when every item passes.
	if in.Members >= 2 && len(in.Household) > 0 {
		var hh []Item
		ok := true
		for _, h := range in.Household {
			it := h.Item
			it.HH = float64(h.Asks*2 + h.Members*3)
			if in.Kid && (in.Allow == nil || !in.Allow(it)) {
				ok = false
			}
			hh = append(hh, it)
		}
		if ok {
			addRow(Row{ID: TypeHousehold, Type: TypeHousehold, Items: allow(hh)})
		}
	}

	if nSeeds < 3 {
		// Cold start: non-personal Popular <genre> and Popular <decade> <genre>.
		genres := regionGenres(trending, 6)
		for _, g := range genres {
			items, err := fetch(Request{List: "discover", Genre: g, MinVotes: 200, Scope: ScopeShared})
			soft(err)
			addRow(Row{ID: TypePopularGenre + ":" + g, Type: TypePopularGenre, Genre: g, Title: RowTitle(TypePopularGenre, g), Items: items})
		}
		for i, dec := range coldDecades {
			g := genres[min(i, len(genres)-1)]
			items, err := fetch(Request{List: "discover", Genre: g, From: fmt.Sprintf("%d-01-01", dec), To: fmt.Sprintf("%d-12-31", dec+9), MinVotes: 200, Scope: ScopeShared})
			soft(err)
			label := fmt.Sprintf("Popular %ds %s", dec, strings.ToLower(g))
			addRow(Row{ID: fmt.Sprintf("%s:%d|%s", TypePopularDecade, dec, g), Type: TypePopularDecade, Genre: g, Decade: dec, Title: label, Items: items})
		}
	}

	// Seed rows: the top 10 seeds, at most 2 per franchise.
	perFranchise := map[string]int{}
	nSeedRows := 0
	maxSeed := 0.0
	for _, s := range seeds {
		if s.Weight > maxSeed {
			maxSeed = s.Weight
		}
	}
	for _, s := range seeds {
		if nSeedRows == 10 || s.Weight <= 0 {
			break
		}
		if !s.Recent {
			continue
		}
		f := Item{TMDBID: s.TMDBID, Kind: s.Kind, Name: s.Name}.Franchise()
		if perFranchise[f] == 2 {
			continue
		}
		q := Request{List: "similar", Kind: s.Kind, Ref: s.TMDBID}
		if !spend(q) {
			break
		}
		perFranchise[f]++
		nSeedRows++
		items, err := fetch(q)
		if !soft(err) {
			continue
		}
		items = similarity(s, items)
		t := TypeBecauseHave
		addRow(Row{ID: t + ":" + s.Key(), Type: t, Seed: s.Key(), SeedWeight: s.Weight, SeedName: s.Name, Title: RowTitle(t, s.Name), Items: dropKey(items, s.Key())})
	}
	if nSeeds >= 3 {
		for _, g := range profile.TopGenres(5, 0.10) {
			q := Request{List: "discover", Genre: g}
			if !spend(q) {
				break
			}
			items, err := fetch(q)
			soft(err)
			addRow(Row{ID: TypeGenre + ":" + g, Type: TypeGenre, Genre: g, Title: RowTitle(TypeGenre, g), Items: items})
		}
	}
	for _, p := range topCounts(in.OwnedPeople, 2, 3) {
		// The Library is the household's, so these lists are shared.
		q := Request{List: "person", Ref: p, Scope: ScopeShared}
		if !spend(q) {
			break
		}
		items, err := fetch(q)
		soft(err)
		addRow(Row{ID: TypePerson + ":" + p, Type: TypePerson, Person: p, Title: RowTitle(TypePerson, p), Items: items})
	}
	for _, dg := range topCounts(in.OwnedDecadeGenre, 3, 2) {
		dec, g, _ := strings.Cut(dg, "|")
		var y int
		fmt.Sscanf(dec, "%d", &y)
		q := Request{List: "discover", Genre: g, From: fmt.Sprintf("%d-01-01", y), To: fmt.Sprintf("%d-12-31", y+9), Scope: ScopeShared}
		if !spend(q) {
			break
		}
		items, err := fetch(q)
		soft(err)
		addRow(Row{ID: TypeDecade + ":" + dg, Type: TypeDecade, Genre: g, Decade: y, Title: fmt.Sprintf("%s %s", dec, strings.ToLower(g)), Items: items})
	}
	if ctx.Err() != nil {
		return pool, ctx.Err()
	}

	// Score: pop is normalised within the candidate set.
	sc := Scorer{Profile: profile, Now: in.Now}
	var union []Item
	for _, r := range rows {
		for _, it := range r.Items {
			sc.MaxPop = maxf(sc.MaxPop, it.Pop)
		}
	}
	hhBy := map[string]float64{}
	for _, h := range in.Household {
		v := float64(h.Asks*2 + h.Members*3)
		hhBy[h.Item.Key()] = v
		sc.MaxHH = maxf(sc.MaxHH, v)
	}
	for i := range rows {
		if rows[i].Title == "" {
			rows[i].Title = RowTitle(rows[i].Type, "")
		}
		if Personal(rows[i].Type) || rows[i].Type == TypeGenre {
			union = append(union, rows[i].Items...)
		}
	}
	if nSeeds >= 3 {
		u := uniq(union)
		for j := range u {
			u[j].HH = hhBy[u[j].Key()]
		}
		rows = append(rows, Row{ID: TypeTopPicks, Type: TypeTopPicks, Title: RowTitle(TypeTopPicks, ""), Items: u})
	}
	for i := range rows {
		r := &rows[i]
		r.Items = sc.Rank(r.Type, uniq(r.Items))
		if len(r.Items) > poolItems {
			r.Items = r.Items[:poolItems]
		}
		r.Fresh = freshness(*r, in.Today, in.Prev)
		r.Score = RowScore(*r, maxSeed)
	}
	pool.Rows = rows
	pool.Taste = profile.TopGenres(3, 0.15)
	if len(errs) > 0 && len(rows) == 0 {
		return pool, fmt.Errorf("explore: no rows: %s", errs[0])
	}
	return pool, nil
}

// typeFreshness: 1 if the row changed since the previous day's pool, else
// 0.5; 1 when there is no previous pool. A same-date rebuild keeps the
// value stored that day.
func freshness(r Row, today, prev *Pool) float64 {
	if today != nil {
		for _, t := range today.Rows {
			if t.ID == r.ID {
				return t.Fresh
			}
		}
	}
	if prev == nil {
		return 1
	}
	for _, p := range prev.Rows {
		if p.ID == r.ID {
			if contentHash(p) == contentHash(r) {
				return 0.5
			}
			return 1
		}
	}
	return 1
}

func contentHash(r Row) string {
	h := sha1.New()
	for i, it := range r.Items {
		if i == RowMax {
			break
		}
		h.Write([]byte(it.Key() + ","))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// similarity blends rank (1 - i/n) 60/40 with facet overlap to the seed.
func similarity(s Seed, items []Item) []Item {
	n := len(items)
	maxO := 0.0
	ov := make([]float64, n)
	for i, it := range items {
		o := 0.0
		for _, g := range it.Genres {
			for _, sg := range s.Genres {
				if g == sg {
					o++
				}
			}
		}
		for _, p := range it.People {
			for _, sp := range s.People {
				if strings.EqualFold(p, sp) {
					o += 2
				}
			}
		}
		ov[i] = o
		maxO = maxf(maxO, o)
	}
	out := make([]Item, n)
	for i, it := range items {
		o := 0.0
		if maxO > 0 {
			o = ov[i] / maxO
		}
		it.Sim = 0.6*(1-float64(i)/float64(n)) + 0.4*o
		out[i] = it
	}
	return out
}

// regionGenres are the genres the trending lists name most, then defaults.
func regionGenres(trending []Item, n int) []string {
	count := map[string]int{}
	for _, it := range trending {
		for _, g := range it.Genres {
			count[g]++
		}
	}
	var list []string
	for g := range count {
		list = append(list, g)
	}
	sort.Slice(list, func(i, j int) bool {
		if count[list[i]] != count[list[j]] {
			return count[list[i]] > count[list[j]]
		}
		return list[i] < list[j]
	})
	for _, g := range coldGenres {
		if !contains(list, g) {
			list = append(list, g)
		}
	}
	return list[:n]
}

// topCounts are the keys with count >= least, most first, at most n.
func topCounts(m map[string]int, least, n int) []string {
	var list []string
	for k, c := range m {
		if c >= least {
			list = append(list, k)
		}
	}
	sort.Slice(list, func(i, j int) bool {
		if m[list[i]] != m[list[j]] {
			return m[list[i]] > m[list[j]]
		}
		return list[i] < list[j]
	})
	return list[:min(n, len(list))]
}

func interleave(lists [][]Item) []Item {
	var out []Item
	for i := 0; ; i++ {
		added := false
		for _, l := range lists {
			if i < len(l) {
				out = append(out, l[i])
				added = true
			}
		}
		if !added {
			return out
		}
	}
}

func uniq(items []Item) []Item {
	seen := map[string]bool{}
	out := make([]Item, 0, len(items))
	for _, it := range items {
		if !seen[it.Key()] {
			seen[it.Key()] = true
			out = append(out, it)
		}
	}
	return out
}

func dropKey(items []Item, key string) []Item {
	out := make([]Item, 0, len(items))
	for _, it := range items {
		if it.Key() != key {
			out = append(out, it)
		}
	}
	return out
}

func kindIn(kinds []string, k string) bool { return contains(kinds, k) }

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func maxf(a, b float64) float64 {
	if b > a {
		return b
	}
	return a
}
