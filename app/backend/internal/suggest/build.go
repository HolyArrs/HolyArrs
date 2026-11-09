package suggest

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// MemberBudget is the most member-specific outside calls one pool build makes.
// Shared lists are counted apart.
const MemberBudget = 30

// PoolVersion is the row set a pool was built with; an older pool is rebuilt.
const PoolVersion = 2

// maxCollections caps how many collections one pool looks up.
const maxCollections = 8

// poolItems is how many items a pool keeps per row.
const poolItems = 40

// coldGenres fill in when the trending lists name too few genres.
var coldGenres = []string{"Drama", "Comedy", "Action", "Thriller", "Animation", "Documentary", "Crime", "Family"}

// coldDecades are the two decades the cold start shows as Popular rows.
var coldDecades = []int{1990, 2000}

// KidGenres are the family rows a Kid page always gets.
var KidGenres = []string{"Family", "Animation"}

// Pool is one member's scored candidate rows for one type switch and day.
type Pool struct {
	Version int    `json:"v"`
	Member  int64  `json:"member"`
	Switch  string `json:"switch"`
	Date    string `json:"date"`
	Seeds   int    `json:"seeds"`
	Kid     bool   `json:"kid"`
	Rows    []Row  `json:"rows"`
	// Taste are the member's strongest genres, for taste-match titles.
	Taste []string `json:"taste,omitempty"`
	// Leans are the genres at LeanShare or more of the member's taste; the
	// hero's genre exclusions skip them.
	Leans []string  `json:"leans,omitempty"`
	Built time.Time `json:"built"`
}

// Request is one outside list the pool build asks for. It carries only
// title ids, genres, dates, services and the region, never anything about
// a member.
type Request struct {
	// List is the kind of list: similar (recommendations and similar of Ref),
	// trending, upcoming (movies coming out, shows airing now), best, discover,
	// person (credits of the person named Ref) or collection (the movies of
	// TMDB collection Ref).
	List  string
	Kind  string
	Ref   string
	Genre string
	// From and To are release dates, YYYY-MM-DD.
	From, To         string
	Services, Region string
	MinVote          float64
	MinVotes         int
	MaxVotes         int
	// Sort is popularity (default), rating or recent (newest out first).
	Sort string
	// AirFrom and AirTo keep shows with an episode airing in that window.
	// Types reads From and To as release dates of those TMDB release types
	// ("4|3" is digital or cinema) in Region, for movies.
	AirFrom, AirTo, Types string
	// Keywords are TMDB keyword names joined by "|", any of them.
	Keywords string
	// Genres and Without are raw TMDB genre ids, "|" for any of and "," for
	// all of. Lang is an original language and Cert the highest US movie
	// certification.
	Genres, Without, Lang, Cert string
	// Scope is ScopeShared for region-wide lists. It is left out of the cache
	// key so it never splits a shared list.
	Scope string
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
	// Owned holds the kind:tmdb keys of titles in the Library.
	Owned map[string]bool
	// OwnedPeople counts Library titles per credited person.
	OwnedPeople map[string]int
	// OwnedDecadeGenre counts Library titles per "1990s|Drama".
	OwnedDecadeGenre map[string]int
	Household        []HouseholdTitle
	// MyAsks are the member's open asks, newest first.
	MyAsks           []Item
	Members          int
	Services, Region string
	// Collections are the TMDB collections the Library has a movie of, most
	// owned first.
	Collections []LibCollection
	// Returning are Library shows whose next episode airs in 7 to 60 days,
	// soonest first.
	Returning []Item
	// Allow is the Kid limit at candidate time; nil allows all.
	Allow func(Item) bool
	// Today is the pool already built today, whose row freshness is kept. Prev
	// is the newest pool from an earlier day.
	Today, Prev *Pool
}

// HouseholdTitle is one title that people in the household asked for in the
// last 30 days.
type HouseholdTitle struct {
	Item    Item
	Asks    int
	Members int
}

// LibCollection is a TMDB collection the Library owns movies of.
type LibCollection struct {
	TMDBID string
	Name   string
	Owned  int
}

// Source answers Requests, in the source's order, best first.
type Source interface {
	List(ctx context.Context, q Request) ([]Item, error)
}

// CacheChecker is a Source that can say a Request would be answered without
// an outside call. Such Requests do not count against the budget.
type CacheChecker interface {
	Cached(ctx context.Context, q Request) bool
}

// Build fetches and scores every candidate row for one member. The Kid limit
// is applied to each list before scoring. Shared lists are asked for like any
// other; the Source is expected to cache them.
func Build(ctx context.Context, src Source, in Input) (Pool, error) {
	kinds := Kinds(in.Switch)
	// allow keeps the kinds of the switch that pass the Kid limit and marks owned titles.
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
	// spend keeps member lists inside MemberBudget. Cached lists cost nothing.
	// Callers ask in priority order, so the rows dropped are the least wanted.
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
	pool := Pool{Version: PoolVersion, Member: in.Member, Switch: in.Switch, Date: Day(in.Now), Seeds: nSeeds, Kid: in.Kid, Built: in.Now}
	var rows []Row
	addRow := func(r Row) {
		if len(r.Items) > 0 {
			rows = append(rows, r)
		}
	}
	var errs []string
	// soft records a failed list and reports whether it worked. A cancelled
	// context is not recorded because the build stops on it.
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

	// Shared rows.
	trending, err := fetch(Request{List: "trending", Scope: ScopeShared})
	if !soft(err) && ctx.Err() != nil {
		return pool, ctx.Err()
	}
	addRow(Row{ID: TypeTrending, Type: TypeTrending, Items: trending})
	if in.Services != "" {
		items, err := fetch(Request{List: "discover", Services: in.Services, Region: in.Region, From: Day(in.Now.AddDate(0, 0, -30)), Scope: ScopeShared})
		soft(err)
		addRow(Row{ID: TypeNewServices, Type: TypeNewServices, Items: items})
	}
	switch in.Switch {
	case "movies":
		upcoming, err := fetch(Request{List: "upcoming", Kind: "movie", Scope: ScopeShared})
		soft(err)
		var coming []Item
		for _, it := range upcoming {
			if d, e := time.Parse("2006-01-02", it.Released); e == nil && !d.Before(in.Now.AddDate(0, 0, -1)) && !d.After(in.Now.AddDate(0, 0, 90)) {
				coming = append(coming, it)
			}
		}
		addRow(Row{ID: TypeCinemas, Type: TypeCinemas, Items: coming})
		// Finish the collection: released parts the Library lacks.
		var missing []Item
		for _, c := range in.Collections[:min(maxCollections, len(in.Collections))] {
			parts, err := fetch(Request{List: "collection", Kind: "movie", Ref: c.TMDBID, Scope: ScopeShared})
			if !soft(err) {
				continue
			}
			id, _ := strconv.ParseInt(c.TMDBID, 10, 64)
			for _, it := range parts {
				if it.Owned || it.Released == "" || it.Released > Day(in.Now) {
					continue
				}
				it.Collection = id
				missing = append(missing, it)
			}
		}
		addRow(Row{ID: TypeCollection, Type: TypeCollection, Items: missing})
	case "shows":
		// New seasons of Library shows on the air now, then new shows.
		airing, err := fetch(Request{List: "upcoming", Kind: "show", Scope: ScopeShared})
		soft(err)
		premieres, err := fetch(Request{List: "discover", Kind: "show", From: Day(in.Now.AddDate(0, 0, -30)), To: Day(in.Now.AddDate(0, 0, 14)), Scope: ScopeShared})
		soft(err)
		var seasons []Item
		for _, it := range airing {
			if it.Owned {
				seasons = append(seasons, it)
			}
		}
		for _, it := range premieres {
			if !it.Owned {
				seasons = append(seasons, it)
			}
		}
		addRow(Row{ID: TypeNewSeasons, Type: TypeNewSeasons, Items: seasons})
		addRow(Row{ID: TypeReturning, Type: TypeReturning, Items: allow(in.Returning)})
	}
	best, err := fetch(Request{List: "best", Scope: ScopeShared})
	soft(err)
	addRow(Row{ID: TypeBest, Type: TypeBest, Items: best})
	hidden, err := fetch(Request{List: "discover", MinVote: 7.3, MinVotes: 100, MaxVotes: 1500, Sort: "rating", Services: in.Services, Region: in.Region, Scope: ScopeShared})
	soft(err)
	addRow(Row{ID: TypeHidden, Type: TypeHidden, Items: hidden})

	// The household row needs at least 2 members; a Kid only gets it when every item passes.
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
		// Cold start: popular rows that need no member data.
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
		items, err := fetch(Request{List: "discover", To: Day(in.Now.AddDate(-20, 0, 0)), MinVotes: 1000, Sort: "rating", Scope: ScopeShared})
		soft(err)
		addRow(Row{ID: TypeClassics, Type: TypeClassics, Items: items})
	}
	if in.Kid {
		for _, g := range KidGenres {
			id := TypePopularGenre + ":" + g
			if hasRow(rows, id) {
				continue
			}
			items, err := fetch(Request{List: "discover", Genre: g, MinVotes: 200, Scope: ScopeShared})
			soft(err)
			addRow(Row{ID: id, Type: TypePopularGenre, Genre: g, Title: RowTitle(TypePopularGenre, g), Items: items})
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
	// Because you asked for: the member's two newest open asks.
	nAsked := 0
	for _, ask := range in.MyAsks {
		if nAsked == 2 {
			break
		}
		if !kindIn(kinds, ask.Kind) || hasRow(rows, TypeBecauseHave+":"+ask.Key()) {
			continue
		}
		q := Request{List: "similar", Kind: ask.Kind, Ref: ask.TMDBID}
		if !spend(q) {
			break
		}
		nAsked++
		items, err := fetch(q)
		if !soft(err) {
			continue
		}
		seed := Seed{TMDBID: ask.TMDBID, Kind: ask.Kind, Name: ask.Name, Genres: ask.Genres}
		t := TypeBecauseAsked
		addRow(Row{ID: t + ":" + ask.Key(), Type: t, Seed: ask.Key(), SeedName: ask.Name, Title: RowTitle(t, ask.Name), Items: dropKey(similarity(seed, items), ask.Key())})
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

	// Score: popularity is normalised within the candidate set.
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
	pool.Leans = profile.TopGenres(100, LeanShare)
	if len(errs) > 0 && len(rows) == 0 {
		return pool, fmt.Errorf("explore: no rows: %s", errs[0])
	}
	return pool, nil
}

// Day returns the pool date of t, in UTC.
func Day(t time.Time) string { return t.UTC().Format("2006-01-02") }

// Kinds returns the title kinds a switch shows.
func Kinds(sw string) []string {
	switch sw {
	case "movies":
		return []string{"movie"}
	case "shows":
		return []string{"show"}
	}
	return []string{"movie", "show"}
}

// String is the cache key of the Request. It leaves out Scope, so a shared
// list is never split from an identical member one.
func (q Request) String() string {
	if q.AirFrom != "" || q.AirTo != "" || q.Types != "" {
		return fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%.1f|%d|%d|%s|%s|a=%s-%s|t=%s", q.List, q.Kind, q.Ref, q.Genre, q.From, q.To, q.Services, q.Region, q.MinVote, q.MinVotes, q.MaxVotes, q.Sort, q.Keywords, q.AirFrom, q.AirTo, q.Types)
	}
	if q.Genres != "" || q.Without != "" || q.Lang != "" || q.Cert != "" {
		return fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%.1f|%d|%d|%s|%s|g=%s|w=%s|l=%s|c=%s", q.List, q.Kind, q.Ref, q.Genre, q.From, q.To, q.Services, q.Region, q.MinVote, q.MinVotes, q.MaxVotes, q.Sort, q.Keywords, q.Genres, q.Without, q.Lang, q.Cert)
	}
	if q.Keywords != "" {
		return fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%.1f|%d|%d|%s|%s", q.List, q.Kind, q.Ref, q.Genre, q.From, q.To, q.Services, q.Region, q.MinVote, q.MinVotes, q.MaxVotes, q.Sort, q.Keywords)
	}
	return fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%.1f|%d|%d|%s", q.List, q.Kind, q.Ref, q.Genre, q.From, q.To, q.Services, q.Region, q.MinVote, q.MinVotes, q.MaxVotes, q.Sort)
}

// freshness is 1 when the row changed since the previous day's pool or there
// is no previous pool, else 0.5. A rebuild on the same date keeps the value
// stored that day, so row order stays stable within a day.
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

// similarity blends rank (60 percent) with facet overlap to the seed (40 percent).
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

// regionGenres returns the n genres the trending lists name most, padded
// with the defaults.
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

// topCounts returns up to n keys with a count of at least least, most first.
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

func hasRow(rows []Row, id string) bool {
	for _, r := range rows {
		if r.ID == id {
			return true
		}
	}
	return false
}

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
