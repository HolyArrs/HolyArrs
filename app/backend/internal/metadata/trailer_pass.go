package metadata

import (
	"cmp"
	"context"
	"errors"
	"log"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
)

// The trailer pass: the only place trailers are looked up on YouTube. On a
// fixed schedule it covers every movie and show Explore shows (the titles
// Wants lists), then the titles asked for elsewhere that nothing was kept
// for (noted, never looked up on the ask), and the artists whose music
// videos are kept ready. It spreads the work, a few lookups per pass with a
// gap between them, so YouTube sees a steady trickle, and looks kept
// addresses up again before they expire (they last 6 hours). Pages only
// read what it kept.

const (
	// TrailerPassBudget is how many lookups a pass makes, one after another.
	TrailerPassBudget = 20
	trailerPassTitle  = 40 * time.Second   // one title's lookup at most
	trailerAskedMax   = 500                // noted titles at most (the oldest goes)
	trailerAskedKeep  = 7 * 24 * time.Hour // a noted title no pass reached is dropped after this
)

// TrailerPassLangs is how many languages a pass covers.
const TrailerPassLangs = 3

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

// TrailerPassTiming sets when the trailer pass first runs after start and
// how often it runs after that, for the browser tests; a zero or negative
// value keeps the one in use. Call it before the trailer pass starts.
func TrailerPassTiming(first, every time.Duration) {
	if first > 0 {
		trailerPassFirst = first
	}
	if every > 0 {
		trailerPassEvery = every
	}
}

// SetTrailerPassGap sets the wait between two lookups of a pass (for
// tests) and answers the one it replaced.
func SetTrailerPassGap(d time.Duration) time.Duration {
	old := trailerPassGap
	trailerPassGap = d
	return old
}

// SetTrailerPassMax sets the most pool titles x languages a pass lists
// (for tests) and answers the one it replaced.
func SetTrailerPassMax(n int) int {
	old := trailerPassMax
	trailerPassMax = n
	return old
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

// Loops are the trailer pass's loop.
func (t *Trailers) Loops() []jobs.Loop {
	return []jobs.Loop{{Name: "trailer_pass", Label: "Trailer read-ahead", Run: t.passLoop}}
}

func (t *Trailers) passLoop(ctx context.Context) error {
	wait := trailerPassFirst
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(wait):
		}
		// Every trailerPassEvery from one start to the next (the pass's own time
		// counts), never back to back.
		began := time.Now()
		t.Pass(Ahead(ctx), TrailerPassBudget)
		wait = max(trailerPassEvery-time.Since(began), trailerPassEvery/10)
	}
}

// Pass looks up, one after another and at most budget times, the
// trailers of the pass's titles in each noted language, trailerPassGap apart. A
// title with a fresh address, a kept miss or a lookup under way is
// skipped; an address that would expire before RenewAhead is looked up
// again, and those renewals go first, so new titles never push a kept
// address out. What is left over waits for the next pass, and so does
// everything once YouTube's door fails. It logs one line when anything was
// due and answers how many lookups ran.
func (t *Trailers) Pass(ctx context.Context, budget int) int {
	if t.Store == nil || !t.Settings.AutoplayTrailers(ctx) {
		return 0
	}
	langs := t.Wants.PassLanguages(ctx)
	// The noted titles (asked for or on screen with nothing kept) go first:
	// people are looking at them. There are at most trailerAskedMax, and
	// the cap applies to the pool titles after them only, so none starves.
	var list [][3]string
	asked := map[string]bool{}
	seen := map[string]bool{}
	add := func(j [3]string) bool {
		ckey := j[0] + "|" + j[1] + "|" + j[2]
		if seen[ckey] || (j[0] != "artist" && len(t.KindsOn(j[0])) == 0) {
			return false
		}
		seen[ckey] = true
		list = append(list, j)
		return true
	}
	for _, j := range t.Asks(time.Now()) {
		if add(j) {
			asked[j[0]+"|"+j[1]+"|"+j[2]] = true
		}
	}
	// Artists whose music videos are kept ready, ranked like the noted
	// titles.
	if t.Videos.On() {
		for _, mbid := range t.Videos.Asks(ctx, time.Now()) {
			j := [3]string{"artist", mbid, ""}
			if add(j) {
				asked[j[0]+"|"+j[1]+"|"+j[2]] = true
			}
		}
	}
	pooled, capped := 0, 0
	for _, ti := range t.Wants.PassTitles(ctx) {
		for _, lang := range langs {
			j := [3]string{ti[0], ti[1], lang}
			if pooled >= trailerPassMax {
				if !seen[j[0]+"|"+j[1]+"|"+j[2]] && len(t.KindsOn(j[0])) > 0 {
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
	var dueList []passJob
	for _, j := range list {
		if ctx.Err() != nil {
			return 0
		}
		ckey := j[0] + "|" + j[1] + "|" + j[2]
		now := time.Now()
		if j[0] == "artist" {
			due, renew, urgent := t.Videos.Due(ctx, j[1], now)
			if !due {
				continue
			}
			rank := 1
			switch {
			case urgent:
				rank = 0
			case renew:
				rank = 2
			}
			dueList = append(dueList, passJob{j, ckey, renew, rank})
			continue
		}
		ok, renew := t.PassDue(ctx, ckey, now)
		if !ok {
			// Kept or under way: a noted title is done, unless what is kept
			// is a short miss (YouTube's door) a later pass asks again.
			if e, ok := t.Get(ckey, now); !ok || e.Miss.IsZero() || e.Miss.Sub(now) > TrailerMiss {
				t.asked.drop(ckey)
			}
			continue
		}
		// Renewals that would lapse before the next pass first, then the
		// noted titles (people are looking at them), then the other
		// renewals (they get one more pass), then the pool's first lookups.
		rank := 3
		switch {
		case renew && t.renewUrgent(ctx, ckey, now):
			rank = 0
		case asked[ckey]:
			rank = 1
		case renew:
			rank = 2
		}
		dueList = append(dueList, passJob{j, ckey, renew, rank})
	}
	due := dueList
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
		failed := t.failed.Load()
		lctx, cancel := context.WithTimeout(ctx, trailerPassTitle)
		var err error
		switch {
		case j[0] == "artist":
			err = t.Videos.Pass(lctx, j[1], renew)
		case renew:
			_, err = t.LookupShared(Renewing(lctx), j[0], j[1], j[2])
		default:
			_, err = t.ResolveTitle(lctx, j[0], j[1], j[2])
		}
		cancel()
		// YouTube refusing this server, or no key to be had (the metadata
		// server or TMDB failing): the rest would fail the same way, and a
		// noted title stays noted for the next pass.
		var m *TrailerMissed
		if t.failed.Load() > failed || (err != nil && !errors.Is(err, ErrNoTrailer) && !errors.As(err, &m)) {
			stopped = true
			continue
		}
		t.asked.drop(ckey)
	}
	t.Videos.SaveAsks(ctx)
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

// Note notes titles for the next pass; no lookup here.
func (t *Trailers) Note(lang string, titles [][2]string) {
	a := &t.asked
	now := time.Now()
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.m == nil {
		a.m = map[string]trailerAsk{}
	}
	// Titles already noted are refreshed first (they keep their place), so
	// making room for a new one in the same call never drops one of them.
	var fresh []string
	for _, ti := range titles {
		if (ti[0] != "movie" && ti[0] != "show") || !DigitsOnly(ti[1]) || !trailerLangRe.MatchString(lang) {
			continue
		}
		k := ti[0] + "|" + ti[1] + "|" + lang
		if e, ok := a.m[k]; ok {
			e.at = now
			a.m[k] = e
			continue
		}
		if !slices.Contains(fresh, k) {
			fresh = append(fresh, k)
		}
	}
	for _, k := range fresh {
		if _, ok := a.m[k]; !ok && len(a.m) >= trailerAskedMax {
			// Full: the one asked for least lately goes (not the one that
			// has waited longest, which may have been asked for just now).
			oldest := ""
			for k2, e := range a.m {
				if o, ok := a.m[oldest]; !ok || e.at.Before(o.at) || (e.at.Equal(o.at) && e.n < o.n) {
					oldest = k2
				}
			}
			delete(a.m, oldest)
		}
		a.n++
		a.m[k] = trailerAsk{at: now, n: a.n}
	}
}

// Asks are the noted titles, oldest first; ones no pass reached in
// trailerAskedKeep are dropped.
func (t *Trailers) Asks(now time.Time) [][3]string {
	a := &t.asked
	a.mu.Lock()
	defer a.mu.Unlock()
	var keys []string
	for k, e := range a.m {
		if now.Sub(e.at) > trailerAskedKeep {
			delete(a.m, k)
			continue
		}
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(x, y string) int { return cmp.Compare(a.m[x].n, a.m[y].n) })
	out := make([][3]string, 0, len(keys))
	for _, k := range keys {
		p := strings.SplitN(k, "|", 3)
		out = append(out, [3]string{p[0], p[1], p[2]})
	}
	return out
}

// DropAsk forgets a noted title.
func (t *Trailers) DropAsk(ckey string) { t.asked.drop(ckey) }

func (a *trailerAsked) drop(ckey string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.m, ckey)
}

// RenewAhead is how long before a kept address is dropped the pass looks
// it up again: two passes apart, plus a full pass with every lookup at its
// time limit (far longer than the gaps between them), so a renewal gets at
// least two chances.
func (t *Trailers) RenewAhead() time.Duration { return TrailerRenewAhead() }

// TrailerRenewAhead is RenewAhead at the pass timing in use.
func TrailerRenewAhead() time.Duration {
	return 2*trailerPassEvery + time.Duration(TrailerPassBudget)*trailerPassTitle
}

// TrailerPassEvery is how often the pass runs.
func TrailerPassEvery() time.Duration { return trailerPassEvery }

// renewUrgent says whether a kept address is dropped before the next pass
// could renew it (its last chance, of the two RenewAhead gives it).
func (t *Trailers) renewUrgent(ctx context.Context, ckey string, now time.Time) bool {
	e, ok := t.Kept(ctx, ckey, now)
	return ok && e.Miss.IsZero() && e.Stream.ExpiresAt.Add(-TrailerEarly).Sub(now) < t.RenewAhead()-trailerPassEvery
}

// PassDue says whether the pass looks the title up (nothing kept, or a
// kept address that is dropped within RenewAhead) and whether that renews
// a kept address.
func (t *Trailers) PassDue(ctx context.Context, ckey string, now time.Time) (due, renew bool) {
	if t.InFlight(ckey) {
		return false, false
	}
	e, ok := t.Kept(ctx, ckey, now)
	switch {
	case !ok:
		return true, false
	case !e.Miss.IsZero():
		return false, false
	}
	return e.Stream.ExpiresAt.Add(-TrailerEarly).Sub(now) < t.RenewAhead(), true
}

// ValidTrailerLanguage says whether l is a language the pass may cover.
func ValidTrailerLanguage(l string) bool { return trailerLangRe.MatchString(l) }
