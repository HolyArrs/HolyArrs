package api

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs/internal/explore"
	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/supervisor"
)

// The trailer pass: the only place trailers are looked up on YouTube. On a
// fixed schedule it covers what Explore shows first (each stored pool's
// hero candidates, then the first cards of its first rows), then the titles
// asked for elsewhere that nothing was kept for (noted, never looked up on
// the ask), within a budget per pass, and looks kept addresses up again
// before they expire. Pages only read what it kept.

const (
	trailerPassEvery  = 15 * time.Minute
	trailerPassFirst  = 3 * time.Minute    // the first pass after start
	trailerPassBudget = 30                 // lookups per pass, one after another
	trailerPassLangs  = 3                  // languages a pass covers
	trailerPassTitle  = 40 * time.Second   // one title's lookup at most
	trailerAskedMax   = 500                // noted titles at most (the oldest goes)
	trailerAskedKeep  = 7 * 24 * time.Hour // a noted title no pass reached is dropped after this
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
		wait = trailerPassEvery
		a.trailerPass(metadata.Ahead(ctx), trailerPassBudget)
	}
}

// trailerPass looks up, one after another and at most budget times, the
// trailers of the pass's titles in each noted language. A title with a
// fresh address, a kept miss or a lookup under way is skipped; an address
// that would expire before the next two passes is looked up again. What is
// left over waits for the next pass, and so does everything once YouTube's
// door fails. It answers how many lookups ran.
func (a *API) trailerPass(ctx context.Context, budget int) int {
	if a.s.Store == nil || !a.uiSettings(ctx).AutoplayTrailers {
		return 0
	}
	langs := a.trailerPassLanguages(ctx)
	var jobs [][3]string
	for _, t := range a.trailerPassTitles(ctx) {
		for _, lang := range langs {
			jobs = append(jobs, [3]string{t[0], t[1], lang})
		}
	}
	jobs = append(jobs, a.trailerAsks(time.Now())...)
	n := 0
	seen := map[string]bool{}
	for _, j := range jobs {
		ckey := j[0] + "|" + j[1] + "|" + j[2]
		if seen[ckey] || len(a.kindsOn(j[0])) == 0 {
			continue
		}
		seen[ckey] = true
		if n >= budget || ctx.Err() != nil {
			return n
		}
		now := time.Now()
		due, renew := a.trailerPassDue(ctx, ckey, now)
		if !due {
			// Kept or under way: a noted title is done, unless what is kept
			// is a short miss (YouTube's door) a later pass asks again.
			if e, ok := a.trailers.get(ckey, now); !ok || e.miss.IsZero() || e.miss.Sub(now) > trailerMiss {
				a.trailerAsked.drop(ckey)
			}
			continue
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
			return n
		}
		a.trailerAsked.drop(ckey)
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

// trailerPassDue says whether the pass looks the title up (nothing kept,
// or a kept address that expires within the next two passes) and whether
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
	return e.s.ExpiresAt.Add(-trailerEarly).Sub(now) < 2*trailerPassEvery, true
}

// trailerPassTitles are the titles of every stored Explore pool (Kid pools
// left out): every pool's hero candidates first, then its row cards, each
// title once.
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
	var heroes, cards [][2]string
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
		cards = append(cards, exploreTrailerTitles(nil, pg.Head)...)
	}
	var out [][2]string
	seen := map[[2]string]bool{}
	for _, t := range append(heroes, cards...) {
		if !seen[t] {
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
