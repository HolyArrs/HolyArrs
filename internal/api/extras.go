package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/parts"
	"github.com/CoderLuii/holyarrs/internal/ratings"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// TitleExtras is the optional extra artwork (the metadata server's stored
// logo, banner and wide backgrounds, as artwork cache links) and outside
// ratings (MDBList) for a title page. Empty fields mean none: the page
// keeps TMDB's art and hides the ratings strip.
type TitleExtras struct {
	Logo  string `json:"logo,omitempty"`
	Thumb string `json:"thumb,omitempty"`
	// Banner is the wide title banner (about 1000x185) the Library table
	// shows per row; empty when the server has none.
	Banner      string         `json:"banner,omitempty"`
	Backgrounds []string       `json:"backgrounds,omitempty"`
	Scores      []OutsideScore `json:"scores"`
}

// OutsideScore is one outside rating with enough votes to show.
type OutsideScore struct {
	Source string  `json:"source"`
	Value  float64 `json:"value"`
	Max    float64 `json:"max"`
	Votes  int64   `json:"votes"`
}

// minOutsideVotes is the fewest votes (or reviews) a score needs to be
// shown; a score with an unknown count is hidden.
var minOutsideVotes = map[string]int64{"imdb": 1000, "rt_critics": 20, "rt_audience": 250, "metacritic": 7, "letterboxd": 500, "trakt": 100}

func shownScores(in []metadata.ServerScore) []OutsideScore {
	out := []OutsideScore{}
	for _, s := range in {
		need, ok := minOutsideVotes[s.Source]
		if !ok || s.Value <= 0 || s.Max <= 0 || s.Value > s.Max || s.Votes < need {
			continue
		}
		out = append(out, OutsideScore{Source: s.Source, Value: s.Value, Max: s.Max, Votes: s.Votes})
	}
	return out
}

type extrasCache struct {
	mu   sync.Mutex
	seen map[string]extrasEntry
}

type extrasEntry struct {
	v  TitleExtras
	at time.Time
}

var titleExtrasCache = &extrasCache{seen: map[string]extrasEntry{}}

// titleExtras answers GET /api/v1/extras/{kind}/{id} (movie or show: TMDB
// id; artist: MusicBrainz id). It never fails the page: anything the
// metadata server cannot give comes back empty.
func (a *API) titleExtras(w http.ResponseWriter, r *http.Request) {
	kind, id := r.PathValue("kind"), strings.ToLower(r.PathValue("id"))
	if (kind == "movie" || kind == "show") && !digitsOnly(id) || kind == "artist" && !metadata.ValidMBID(id) || kind != "movie" && kind != "show" && kind != "artist" {
		fail(w, http.StatusBadRequest, "unknown title")
		return
	}
	// Part and Kid gates come before the shared cache: its key does not
	// carry the Kid limit.
	if p, _ := parts.ForTitleKind(kind); !a.partIsOn(p) {
		fail(w, http.StatusConflict, "that part is off")
		return
	}
	if a.kidLimited(w, r) {
		return
	}
	if max := a.kidMax(r); max != "" {
		// Music never passes a limit (see ratings).
		if kind == "artist" {
			fail(w, http.StatusForbidden, aboveLimit)
			return
		}
		rating, inLibrary := a.libraryRating(r.Context(), kind, id)
		if !inLibrary {
			d, ok := metadata.DiscovererFor(r.Context(), a.s.Metadata)
			if !ok {
				fail(w, http.StatusForbidden, aboveLimit)
				return
			}
			dkey := "details|" + kind + "|" + id
			e, cached := a.discover.get(dkey)
			if !cached || time.Since(e.at) >= discoverTTL["details"] {
				det, err := d.Details(r.Context(), kind, id)
				if err != nil {
					fail(w, http.StatusForbidden, aboveLimit)
					return
				}
				e = discoverEntry{at: time.Now(), details: &det}
				a.discover.put(dkey, e)
			}
			rating = e.details.Rating
		}
		if rating == "" || !ratings.Allows(max, kind, rating) {
			fail(w, http.StatusForbidden, aboveLimit)
			return
		}
	}
	a.exploreCaching(w, r, "")
	// An artist's answer carries its pictures' records (the logo's shape, the
	// wide background's colour), like every music answer.
	write := func(v TitleExtras) {
		if kind == "artist" {
			a.writeMusicJSON(w, r, v)
			return
		}
		writeJSON(w, http.StatusOK, v)
	}
	key := kind + ":" + id
	c := titleExtrasCache
	c.mu.Lock()
	e, ok := c.seen[key]
	c.mu.Unlock()
	if !ok {
		e, ok = loadKept[TitleExtras](r.Context(), a, "extras|"+key)
		if ok {
			c.keep(key, e)
		}
	}
	if ok {
		// Stored answers are served at once; an old one is refreshed in
		// the background for the next view.
		if time.Since(e.at) >= extrasFresh {
			refreshOnce("extras|"+key, func(ctx context.Context) {
				if v, settled := a.fetchExtras(ctx, kind, id); settled {
					c.keep(key, extrasEntry{v: v, at: time.Now()})
					keepOnDisk(ctx, a, "extras|"+key, v)
				}
			})
		}
		write(e.v)
		return
	}
	v, settled := a.fetchExtras(r.Context(), kind, id)
	if settled {
		c.keep(key, extrasEntry{v: v, at: time.Now()})
		keepOnDisk(r.Context(), a, "extras|"+key, v)
	} else {
		// A hiccup's answer is not kept; the browser asks again next view.
		w.Header().Set("Cache-Control", "private, no-cache")
	}
	write(v)
}

// extrasFresh is how long kept extras are served before a background refresh.
const extrasFresh = 6 * time.Hour

func (c *extrasCache) keep(key string, e extrasEntry) {
	c.mu.Lock()
	keepUnder(c.seen, 5000, func(e extrasEntry) time.Time { return e.at })
	c.seen[key] = e
	c.mu.Unlock()
}

// fetchExtras asks the metadata server for a title's extras, art and
// ratings together; settled is false after a server hiccup (not kept).
func (a *API) fetchExtras(parent context.Context, kind, id string) (TitleExtras, bool) {
	v := TitleExtras{Scores: []OutsideScore{}}
	ctx, cancel := context.WithTimeout(parent, 8*time.Second)
	defer cancel()
	settledArt, settledRatings := true, true
	var wg sync.WaitGroup
	if kind == "artist" {
		// An artist's pictures come only from our metadata server: its
		// stored wide background, logo and photo, through the picture route.
		var wide, logo, thumb bool
		wg.Add(3)
		go func() { defer wg.Done(); _, _, wide = a.musicTrendArtFile(ctx, "wide", id, "1920") }()
		go func() { defer wg.Done(); _, _, logo = a.musicTrendArtFile(ctx, "logo", id, "1200") }()
		go func() { defer wg.Done(); _, _, thumb = a.musicTrendArtFile(ctx, "artist", id, "500") }()
		wg.Wait()
		if wide {
			v.Backgrounds = []string{musicArtPrefix + "wide/" + id}
		}
		if logo {
			v.Logo = musicArtPrefix + "logo/" + id
		}
		if thumb {
			v.Thumb = musicArtPrefix + "artist/" + id
		}
		// An artist with no wide photo is asked again: the server's art
		// source may only have been out of budget for a moment.
		return v, wide
	}
	if a.s.ExtraArt != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			art, err := a.s.ExtraArt(ctx, kind, id)
			if err == nil {
				v.Logo, v.Thumb, v.Banner = a.serverArtLink(art.Logo), a.serverArtLink(art.Thumb), a.serverArtLink(art.Banner)
				for _, b := range art.Backgrounds {
					if l := a.serverArtLink(b); l != "" {
						v.Backgrounds = append(v.Backgrounds, l)
					}
				}
			}
			settledArt = err == nil || errors.Is(err, metadata.ErrNotFound)
		}()
	}
	var scores []OutsideScore
	if a.s.ExtraRatings != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rt, err := a.s.ExtraRatings(ctx, kind, id)
			if err == nil {
				scores = shownScores(rt.Scores)
			}
			settledRatings = err == nil || errors.Is(err, metadata.ErrNotFound)
		}()
	}
	wg.Wait()
	if scores != nil {
		v.Scores = scores
	}
	return v, settledArt && settledRatings
}

// keptOnDisk is how long a stored outside answer stays in SQLite; it is
// refreshed long before (in the background, on view).
const keptOnDisk = 30 * 24 * time.Hour

type keptBody[T any] struct {
	V  T         `json:"v"`
	At time.Time `json:"at"`
}

// keepOnDisk stores v under key in lookup_cache with when it was fetched.
func keepOnDisk[T any](ctx context.Context, a *API, key string, v T) {
	if a.s.Store == nil {
		return
	}
	now := time.Now()
	if b, err := json.Marshal(keptBody[T]{V: v, At: now}); err == nil {
		ctx := context.WithoutCancel(ctx)
		_ = store.LookupCache{Store: a.s.Store}.Put(ctx, key, b, now, now.Add(keptOnDisk))
		capKept(ctx, a, key)
	}
}

// keptPerPrefix caps the rows each kind of kept answer ("extras|",
// "wiki|", "details|") holds in lookup_cache; the oldest go first.
var keptPerPrefix = 20000

// capKept drops the oldest rows of key's prefix past keptPerPrefix.
func capKept(ctx context.Context, a *API, key string) {
	prefix, _, ok := strings.Cut(key, "|")
	if !ok {
		return
	}
	like := prefix + "|%"
	_ = a.s.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`DELETE FROM lookup_cache WHERE key IN (SELECT key FROM lookup_cache WHERE key LIKE ?
			ORDER BY fetched_at DESC LIMIT -1 OFFSET ?)`, like, keptPerPrefix)
		return err
	})
}

// loadKept reads an answer keepOnDisk stored, as a memory entry.
func loadKept[T any](ctx context.Context, a *API, key string) (struct {
	v  T
	at time.Time
}, bool) {
	var out struct {
		v  T
		at time.Time
	}
	if a.s.Store == nil || a.s.Store.Read() == nil {
		return out, false
	}
	b, ok, err := store.LookupCache{Store: a.s.Store}.Get(ctx, key, time.Now())
	var k keptBody[T]
	if err != nil || !ok || json.Unmarshal(b, &k) != nil {
		return out, false
	}
	out.v, out.at = k.V, k.At
	return out, true
}

var refreshing sync.Map

// refreshOnce runs fn in the background unless a refresh for key is
// already running; the visitor never waits for it.
func refreshOnce(key string, fn func(ctx context.Context)) {
	if _, busy := refreshing.LoadOrStore(key, true); busy {
		return
	}
	go func() {
		defer refreshing.Delete(key)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		fn(ctx)
	}()
}

func digitsOnly(s string) bool {
	if s == "" || len(s) > 12 || s[0] == '0' {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// titleWarmer fills title details, extras and trailer links ahead, on the
// server's own schedule, for every title Explore and New & Popular list,
// so no visitor action makes the first outside call. Two workers at most;
// trailers stay within the install's ahead-of-time budget.
type titleWarmer struct {
	once  sync.Once
	mu    sync.Mutex
	queue chan warmJob
	// listed is when each title was last listed and warmed.
	listed map[string]warmMark
}

type warmJob struct{ kind, id, lang string }

type warmMark struct{ listed, warmed time.Time }

const (
	warmWorkers = 2
	warmEvery   = time.Hour          // the schedule re-warms what went stale
	warmListed  = 7 * 24 * time.Hour // titles listed this recently stay warm
	warmMax     = 5000
)

// warmTitles queues titles (kind movie or show, TMDB id) to be warmed; a
// title warmed within extrasFresh is skipped. It never blocks.
func (a *API) warmTitles(lang string, titles [][2]string) {
	w := &a.warm
	w.once.Do(func() {
		w.queue = make(chan warmJob, 1000)
		w.listed = map[string]warmMark{}
		for range warmWorkers {
			go a.warmWorker()
		}
		go a.warmSchedule()
	})
	now := time.Now()
	for _, t := range titles {
		if (t[0] != "movie" && t[0] != "show") || !digitsOnly(t[1]) {
			continue
		}
		k := t[0] + "|" + t[1] + "|" + lang
		w.mu.Lock()
		m := w.listed[k]
		fresh := now.Sub(m.warmed) < extrasFresh
		m.listed = now
		if !fresh {
			m.warmed = now // queued; a failure is tried again on the schedule
		}
		if len(w.listed) < warmMax || w.listed[k] != (warmMark{}) {
			w.listed[k] = m
		}
		w.mu.Unlock()
		if fresh {
			continue
		}
		select {
		case w.queue <- warmJob{t[0], t[1], lang}:
		default: // full: the schedule picks it up later
		}
	}
}

func (a *API) warmSchedule() {
	tick := time.NewTicker(warmEvery)
	defer tick.Stop()
	for range tick.C {
		now := time.Now()
		var due []warmJob
		w := &a.warm
		w.mu.Lock()
		for k, m := range w.listed {
			if now.Sub(m.listed) > warmListed {
				delete(w.listed, k)
				continue
			}
			if now.Sub(m.warmed) >= extrasFresh {
				parts := strings.SplitN(k, "|", 3)
				due = append(due, warmJob{parts[0], parts[1], parts[2]})
				m.warmed = now
				w.listed[k] = m
			}
		}
		w.mu.Unlock()
		for _, j := range due {
			w.queue <- j
		}
	}
}

func (a *API) warmWorker() {
	for j := range a.warm.queue {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		a.warmOne(ctx, j)
		cancel()
	}
}

// warmOne stores one title's details and extras (when missing or old) and
// looks its trailer up within the ahead budget.
func (a *API) warmOne(ctx context.Context, j warmJob) {
	if d, ok := metadata.DiscovererFor(ctx, a.s.Metadata); ok {
		key := "details|" + j.kind + "|" + j.id
		e, cached := a.discover.get(key)
		if !cached || e.details == nil {
			if k, found := loadKept[metadata.Details](ctx, a, key); found {
				e, cached = discoverEntry{at: k.at, details: &k.v}, true
				a.discover.put(key, e)
			}
		}
		if !cached || e.details == nil || time.Since(e.at) >= detailsTTL(e) {
			if det, err := d.Details(ctx, j.kind, j.id); err == nil {
				a.discover.put(key, discoverEntry{at: time.Now(), details: &det})
				keepOnDisk(ctx, a, key, det)
			}
		}
	}
	if a.s.ExtraArt != nil || a.s.ExtraRatings != nil {
		key := j.kind + ":" + j.id
		c := titleExtrasCache
		c.mu.Lock()
		e, ok := c.seen[key]
		c.mu.Unlock()
		if !ok {
			e, ok = loadKept[TitleExtras](ctx, a, "extras|"+key)
		}
		if !ok || time.Since(e.at) >= extrasFresh {
			if v, settled := a.fetchExtras(ctx, j.kind, j.id); settled {
				c.keep(key, extrasEntry{v: v, at: time.Now()})
				keepOnDisk(ctx, a, "extras|"+key, v)
			}
		} else {
			c.keep(key, e)
		}
	}
	if !a.uiSettings(ctx).AutoplayTrailers {
		return
	}
	ckey := j.kind + "|" + j.id + "|" + j.lang
	now := time.Now()
	if _, ok := a.trailers.get(ckey, now); ok || a.trailerStored(ctx, ckey, now) || a.trailers.inFlight(ckey) || !a.trailers.allowAhead(now) {
		return
	}
	_, _ = a.resolveTitle(ctx, j.kind, j.id, j.lang)
}
