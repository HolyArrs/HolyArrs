package api

import (
	"cmp"
	"context"
	"errors"
	"log"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs/internal/explore"
	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/supervisor"
)

// The trailer pass: the only place trailers are looked up on YouTube. On a
// fixed schedule it covers every movie and show Explore shows from the
// stored pools (each pool's hero candidates, then every card of every head
// and tail row in page order), then the titles asked for elsewhere that
// nothing was kept for (noted, never looked up on the ask). It spreads the
// work, a few lookups per pass with a gap between them, so YouTube sees a
// steady trickle, and looks kept addresses up again before they expire
// (they last 6 hours). Pages only read what it kept.

const (
	trailerPassBudget = 20                 // lookups per pass, one after another
	trailerPassLangs  = 3                  // languages a pass covers
	trailerPassTitle  = 40 * time.Second   // one title's lookup at most
	trailerAskedMax   = 500                // noted titles at most (the oldest goes)
	trailerAskedKeep  = 7 * 24 * time.Hour // a noted title no pass reached is dropped after this
)

var (
	// trailerPassEvery is how often the pass runs and trailerPassFirst when
	// it first runs after start (TrailerPassTiming changes them for tests).
	trailerPassEvery = 5 * time.Minute
	trailerPassFirst = 3 * time.Minute
	// trailerPassGap is the wait between two lookups of a pass (tests set 0).
	trailerPassGap = 2 * time.Second
	// trailerPassMax is the most pool titles x languages a pass lists (the
	// noted ones come on top); the rest waits until the list is shorter.
	trailerPassMax = 1000
)

// TrailerLoops are the trailer pass's loop.
func (a *API) TrailerLoops() []supervisor.Loop {
	return []supervisor.Loop{{Name: "trailer_pass", Label: "Trailer read-ahead", Run: a.trailerPassLoop}}
}

func (a *API) trailerPassLoop(ctx context.Context) error {
	wait := trailerPassFirst
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(wait):
		}
		// Every trailerPassEvery from one start to the next (the pass's own
		// time counts), never back to back.
		began := time.Now()
		a.trailerPass(metadata.Ahead(ctx), trailerPassBudget)
		wait = max(trailerPassEvery-time.Since(began), trailerPassEvery/10)
	}
}

// trailerPass looks up, one after another and at most budget times, the
// trailers of the pass's titles in each noted language, trailerPassGap
// apart. A title with a fresh address, a kept miss or a lookup under way is
// skipped; an address that would expire before trailerRenewAhead is looked
// up again, and those renewals go first, so new titles never push a kept
// address out. What is left over waits for the next pass, and so does
// everything once YouTube's door fails. It logs one line when anything was
// due and answers how many lookups ran.
func (a *API) trailerPass(ctx context.Context, budget int) int {
	if a.s.Store == nil || !a.uiSettings(ctx).AutoplayTrailers {
		return 0
	}
	langs := a.trailerPassLanguages(ctx)
	// The noted titles (asked for or on screen with nothing kept) go first:
	// people are looking at them. There are at most trailerAskedMax, and
	// the cap applies to the pool titles after them only, so none starves.
	var list [][3]string
	asked := map[string]bool{}
	seen := map[string]bool{}
	add := func(j [3]string) bool {
		ckey := j[0] + "|" + j[1] + "|" + j[2]
		if seen[ckey] || len(a.kindsOn(j[0])) == 0 {
			return false
		}
		seen[ckey] = true
		list = append(list, j)
		return true
	}
	for _, j := range a.trailerAsks(time.Now()) {
		if add(j) {
			asked[j[0]+"|"+j[1]+"|"+j[2]] = true
		}
	}
	pooled, capped := 0, 0
	for _, t := range a.trailerPassTitles(ctx) {
		for _, lang := range langs {
			j := [3]string{t[0], t[1], lang}
			if pooled >= trailerPassMax {
				if !seen[j[0]+"|"+j[1]+"|"+j[2]] && len(a.kindsOn(j[0])) > 0 {
					capped++
				}
				continue
			}
			if add(j) {
				pooled++
			}
		}
	}
	type passJob struct {
		j     [3]string
		ckey  string
		renew bool
		rank  int // the order below: 0 goes first
	}
	var due []passJob
	for _, j := range list {
		if ctx.Err() != nil {
			return 0
		}
		ckey := j[0] + "|" + j[1] + "|" + j[2]
		now := time.Now()
		ok, renew := a.trailerPassDue(ctx, ckey, now)
		if !ok {
			// Kept or under way: a noted title is done, unless what is kept
			// is a short miss (YouTube's door) a later pass asks again.
			if e, ok := a.trailers.get(ckey, now); !ok || e.miss.IsZero() || e.miss.Sub(now) > trailerMiss {
				a.trailerAsked.drop(ckey)
			}
			continue
		}
		// Renewals that would lapse before the next pass first, then the
		// noted titles (people are looking at them), then the other
		// renewals (they get one more pass), then the pool's first lookups.
		rank := 3
		switch {
		case renew && a.trailerRenewUrgent(ctx, ckey, now):
			rank = 0
		case asked[ckey]:
			rank = 1
		case renew:
			rank = 2
		}
		due = append(due, passJob{j, ckey, renew, rank})
	}
	slices.SortStableFunc(due, func(x, y passJob) int { return cmp.Compare(x.rank, y.rank) })
	n, left := 0, 0
	stopped := false
	for _, d := range due {
		if ctx.Err() != nil {
			return n
		}
		j, ckey, renew := d.j, d.ckey, d.renew
		if stopped || n >= budget {
			left++
			continue
		}
		if n > 0 && trailerPassGap > 0 {
			select {
			case <-ctx.Done():
				return n
			case <-time.After(trailerPassGap):
			}
		}
		n++
		failed := a.trailers.failed.Load()
		lctx, cancel := context.WithTimeout(ctx, trailerPassTitle)
		var err error
		if renew {
			_, err = a.lookupShared(lctx, j[0], j[1], j[2])
		} else {
			_, err = a.resolveTitle(lctx, j[0], j[1], j[2])
		}
		cancel()
		// YouTube refusing this server, or no key to be had (the metadata
		// server or TMDB failing): the rest would fail the same way, and a
		// noted title stays noted for the next pass.
		var m *trailerMissed
		if a.trailers.failed.Load() > failed || (err != nil && !errors.Is(err, errNoTrailer) && !errors.As(err, &m)) {
			stopped = true
			continue
		}
		a.trailerAsked.drop(ckey)
	}
	if n+left > 0 || capped > 0 {
		line := "trailers: read-ahead %d due, %d looked up, %d left for the next pass"
		args := []any{n + left, n, left}
		if capped > 0 {
			line += ", %d past the cap"
			args = append(args, capped)
		}
		log.Printf(line, args...)
	}
	return n
}

// trailerAsked are titles asked for (by "kind|id|lang") that nothing was
// kept for, outside Explore's pools too (a person's page, the Requests
// banner, New & Popular's hero): the next pass looks them up, and a title
// leaves once a pass reached it.
type trailerAsked struct {
	mu sync.Mutex
	m  map[string]trailerAsk
	n  uint64
}

// trailerAsk is when a title was last asked for, and in which order.
type trailerAsk struct {
	at time.Time
	n  uint64
}

// noteTrailerTitles notes titles for the next pass; no lookup here.
func (a *API) noteTrailerTitles(lang string, titles [][2]string) {
	t := &a.trailerAsked
	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.m == nil {
		t.m = map[string]trailerAsk{}
	}
	for _, ti := range titles {
		if (ti[0] != "movie" && ti[0] != "show") || !digitsOnly(ti[1]) || !trailerLangRe.MatchString(lang) {
			continue
		}
		k := ti[0] + "|" + ti[1] + "|" + lang
		if _, ok := t.m[k]; !ok && len(t.m) >= trailerAskedMax {
			oldest := ""
			for k2, e := range t.m {
				if oldest == "" || e.n < t.m[oldest].n {
					oldest = k2
				}
			}
			delete(t.m, oldest)
		}
		// Asked again: still recent, but it keeps its place in the queue
		// (a title people keep seeing does not slide to the back).
		if e, ok := t.m[k]; ok {
			e.at = now
			t.m[k] = e
			continue
		}
		t.n++
		t.m[k] = trailerAsk{at: now, n: t.n}
	}
}

// trailerAsks are the noted titles, oldest first; ones no pass reached in
// trailerAskedKeep are dropped.
func (a *API) trailerAsks(now time.Time) [][3]string {
	t := &a.trailerAsked
	t.mu.Lock()
	defer t.mu.Unlock()
	var keys []string
	for k, e := range t.m {
		if now.Sub(e.at) > trailerAskedKeep {
			delete(t.m, k)
			continue
		}
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(x, y string) int { return cmp.Compare(t.m[x].n, t.m[y].n) })
	out := make([][3]string, 0, len(keys))
	for _, k := range keys {
		p := strings.SplitN(k, "|", 3)
		out = append(out, [3]string{p[0], p[1], p[2]})
	}
	return out
}

func (t *trailerAsked) drop(ckey string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.m, ckey)
}

// trailerRenewAhead is how long before a kept address is dropped the pass
// looks it up again: two passes apart, plus a full pass with every lookup
// at its time limit (far longer than the gaps between them), so a renewal
// gets at least two chances.
func trailerRenewAhead() time.Duration {
	return 2*trailerPassEvery + time.Duration(trailerPassBudget)*trailerPassTitle
}

// trailerRenewUrgent says whether a kept address is dropped before the
// next pass could renew it (its last chance, of the two trailerRenewAhead
// gives it).
func (a *API) trailerRenewUrgent(ctx context.Context, ckey string, now time.Time) bool {
	e, ok := a.keptTrailer(ctx, ckey, now)
	return ok && e.miss.IsZero() && e.s.ExpiresAt.Add(-trailerEarly).Sub(now) < trailerRenewAhead()-trailerPassEvery
}

// trailerPassDue says whether the pass looks the title up (nothing kept,
// or a kept address that is dropped within trailerRenewAhead) and whether
// that renews a kept address.
func (a *API) trailerPassDue(ctx context.Context, ckey string, now time.Time) (due, renew bool) {
	if a.trailers.inFlight(ckey) {
		return false, false
	}
	e, ok := a.keptTrailer(ctx, ckey, now)
	switch {
	case !ok:
		return true, false
	case !e.miss.IsZero():
		return false, false
	}
	return e.s.ExpiresAt.Add(-trailerEarly).Sub(now) < trailerRenewAhead(), true
}

// trailerPassTitles are the movies and shows of every stored Explore pool
// (Kid pools left out) as each page shows them: every pool's hero
// candidates first, then every card of its head and tail rows in page order
// (Trending on streaming from its kept lists), each title once and none on
// the Never-add list.
func (a *API) trailerPassTitles(ctx context.Context) [][2]string {
	rows, err := a.s.Store.Read().QueryContext(ctx, "SELECT DISTINCT member_id, type_switch FROM explore_pool ORDER BY member_id, type_switch")
	if err != nil {
		return nil
	}
	var keys []poolKey
	for rows.Next() {
		var k poolKey
		if rows.Scan(&k.member, &k.sw) == nil {
			keys = append(keys, k)
		}
	}
	rows.Close()
	now := a.exploreNow()
	day := explore.Day(now)
	var heroes [][2]string
	var cards []explore.Item
	for _, k := range keys {
		m, err := a.loadMember(ctx, k.member)
		if err != nil || m.kid {
			continue
		}
		// Today's pool, else the newest older one, as Explore shows it.
		pool, _, err := a.readPool(ctx, k.member, k.sw, day, false)
		if err == nil && pool == nil {
			pool, _, err = a.readPool(ctx, k.member, k.sw, day, true)
		}
		if err != nil || pool == nil || pool.Kid {
			continue
		}
		pg := a.poolPage(*pool, k.member, k.sw, m.region, explore.Moment{Local: now.Local()})
		heroes = append(heroes, exploreTrailerTitles(heroCandidates(pg, k.sw == "all"), nil)...)
		for _, row := range append(append([]explore.Row{}, pg.Head...), pg.Tail...) {
			if row.Type == explore.TypeTopServices {
				// The page fills this row itself, from the kept lists.
				cards = append(cards, a.keptServiceTop(strings.ToUpper(m.region), k.sw)...)
				continue
			}
			cards = append(cards, row.Items...)
		}
	}
	items := make([]metadata.DiscoverItem, 0, len(heroes)+len(cards))
	for _, t := range heroes {
		items = append(items, metadata.DiscoverItem{Kind: t[0], TMDBID: t[1]})
	}
	for _, it := range cards {
		items = append(items, metadata.DiscoverItem{Kind: it.Kind, TMDBID: it.TMDBID})
	}
	items, err = a.dropNeverAdd(ctx, items)
	if err != nil {
		return nil
	}
	var out [][2]string
	seen := map[[2]string]bool{}
	for _, it := range items {
		t := [2]string{it.Kind, it.TMDBID}
		if (it.Kind == "movie" || it.Kind == "show") && it.TMDBID != "" && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}

// trailerPassLanguages are the languages pages asked for lately (most card
// pictures kept first, then the latest asked), at most trailerPassLangs.
// Before any page asked (just after a start) the card pictures' languages
// count instead; "en" when none.
func (a *API) trailerPassLanguages(ctx context.Context) []string {
	var asked []string
	at := map[string]time.Time{}
	c := &a.explore.cards
	c.mu.Lock()
	for l, t := range c.langs {
		if time.Since(t) < cardArtLangTTL {
			asked = append(asked, l)
			at[l] = t
		}
	}
	c.mu.Unlock()
	slices.SortFunc(asked, func(x, y string) int { return at[y].Compare(at[x]) })
	var out []string
	add := func(l string) {
		if len(out) < trailerPassLangs && len(l) == 2 && trailerLangRe.MatchString(l) && !slices.Contains(out, l) {
			out = append(out, l)
		}
	}
	if rows, err := a.s.Store.Read().QueryContext(ctx, "SELECT lang FROM card_art GROUP BY lang ORDER BY count(*) DESC, lang LIMIT 20"); err == nil {
		for rows.Next() {
			var l string
			// The filler keeps English pictures for every title; they count
			// only when a page asked for English.
			if rows.Scan(&l) == nil && (len(asked) == 0 || at[l] != (time.Time{})) {
				add(l)
			}
		}
		rows.Close()
	}
	for _, l := range asked {
		add(l)
	}
	if len(out) == 0 {
		out = []string{"en"}
	}
	return out
}
