package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs/internal/explore"
	"github.com/CoderLuii/holyarrs/internal/metadata"
)

// WikiFacts is a title's or person's awards and Wikipedia bio for the page
// (GET /api/v1/wiki/{kind}/{id}). Empty fields mean none.
type WikiFacts struct {
	// Badges are short labels ("Oscar winner", "3 Emmys"), most notable first.
	Badges     []string              `json:"badges"`
	Tallies    []metadata.WikiTally  `json:"tallies"`
	Awards     []metadata.WikiAward  `json:"awards"`
	Born       string                `json:"born,omitempty"`
	Died       string                `json:"died,omitempty"`
	BirthPlace string                `json:"birth_place,omitempty"`
	DeathPlace string                `json:"death_place,omitempty"`
	Bio        *metadata.WikiSummary `json:"bio"`
}

// wikiAwardsMax is how many awards the page lists (wins first).
const wikiAwardsMax = 30

type wikiCache struct {
	mu   sync.Mutex
	seen map[string]wikiEntry
}

type wikiEntry struct {
	v  WikiFacts
	at time.Time
}

var wikiFactsCache = &wikiCache{seen: map[string]wikiEntry{}}

// wikiFacts answers GET /api/v1/wiki/{kind}/{id} (movie, show or person;
// TMDB id). It never fails the page: anything the metadata server cannot
// give comes back empty.
func (a *API) wikiFacts(w http.ResponseWriter, r *http.Request) {
	kind, id := r.PathValue("kind"), r.PathValue("id")
	if kind != "movie" && kind != "show" && kind != "person" || !digitsOnly(id) {
		fail(w, http.StatusBadRequest, "unknown title or person")
		return
	}
	key := kind + ":" + id
	c := wikiFactsCache
	c.mu.Lock()
	e, ok := c.seen[key]
	c.mu.Unlock()
	if ok && time.Since(e.at) < 6*time.Hour {
		writeJSON(w, http.StatusOK, e.v)
		return
	}
	v := WikiFacts{Badges: []string{}, Tallies: []metadata.WikiTally{}, Awards: []metadata.WikiAward{}}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	var ent metadata.WikiEntity
	err := metadata.ErrNotFound
	if kind == "person" && a.s.WikiPerson != nil {
		ent, err = a.s.WikiPerson(ctx, id)
	} else if kind != "person" && a.s.WikiTitle != nil {
		ent, err = a.s.WikiTitle(ctx, kind, id)
	}
	if err == nil {
		if ent.Tallies != nil {
			v.Tallies = ent.Tallies
		}
		v.Badges = metadata.AwardBadges(v.Tallies)
		if len(ent.Awards) > 0 {
			v.Awards = ent.Awards[:min(len(ent.Awards), wikiAwardsMax)]
		}
		v.Born, v.Died, v.BirthPlace, v.DeathPlace = ent.Born, ent.Died, ent.BirthPlace, ent.DeathPlace
		if kind == "person" {
			v.Bio = ent.Summary
		}
	}
	// Only settled answers are kept; a server hiccup is asked again.
	if err != nil && !errors.Is(err, metadata.ErrNotFound) {
		writeJSON(w, http.StatusOK, v)
		return
	}
	c.mu.Lock()
	if len(c.seen) > 5000 {
		c.seen = map[string]wikiEntry{}
	}
	c.seen[key] = wikiEntry{v: v, at: time.Now()}
	c.mu.Unlock()
	writeJSON(w, http.StatusOK, v)
}

type awardSet struct {
	ids map[string]bool
	at  time.Time
	err error
}

var awardSets = struct {
	mu    sync.Mutex
	kinds map[string]awardSet
}{kinds: map[string]awardSet{}}

// awardWinners is the server's award-winning TMDB IDs for kind (movie or
// show), kept a day (a failure ten minutes; an older good copy wins).
func (a *API) awardWinners(ctx context.Context, kind string) (map[string]bool, bool) {
	if a.s.AwardWinners == nil {
		return nil, false
	}
	awardSets.mu.Lock()
	s, ok := awardSets.kinds[kind]
	awardSets.mu.Unlock()
	if ok && (s.err == nil && time.Since(s.at) < 24*time.Hour || s.err != nil && time.Since(s.at) < 10*time.Minute) {
		return s.ids, len(s.ids) > 0
	}
	list, err := a.s.AwardWinners(ctx, kind)
	if err != nil && ctx.Err() != nil {
		return s.ids, len(s.ids) > 0
	}
	next := awardSet{at: time.Now(), err: err, ids: s.ids}
	if err == nil {
		next.ids = make(map[string]bool, len(list))
		for _, w := range list {
			if w.TMDB > 0 {
				next.ids[strconv.FormatInt(w.TMDB, 10)] = true
			}
		}
	}
	awardSets.mu.Lock()
	awardSets.kinds[kind] = next
	awardSets.mu.Unlock()
	return next.ids, len(next.ids) > 0
}

// awardWindows spread the Award winners lists over the decades, so the
// collection is not only this year's films.
var awardWindows = [][2]string{{"", ""}, {"1960-01-01", "1989-12-31"}, {"1990-01-01", "2004-12-31"}, {"2005-01-01", "2014-12-31"}, {"2015-01-01", ""}}

// awardItems builds the Award winners collection: well-known titles per
// decade (and the best rated), kept when Wikidata lists a major award
// win for them. ok is false when the winner lists are not available, so
// the caller keeps the keyword lists.
func (a *API) awardItems(ctx context.Context, src exploreSource, c explore.Category, sw string, kid bool) ([]explore.Item, bool, error) {
	var lists [][]explore.Item
	found := false
	for _, base := range c.Requests(sw, kid) {
		ids, ok := a.awardWinners(ctx, base.Kind)
		if !ok {
			continue
		}
		found = true
		base.Keywords, base.MinVotes = "", 300
		var reqs []explore.Request
		for _, win := range awardWindows {
			r := base
			r.From, r.To = win[0], win[1]
			reqs = append(reqs, r)
		}
		best := base
		best.Sort, best.MinVotes = "rating", 2000
		reqs = append(reqs, best)
		for _, q := range reqs {
			items, err := src.List(ctx, q)
			if err != nil {
				return nil, true, err
			}
			kept := []explore.Item{}
			for _, it := range items {
				if ids[it.TMDBID] {
					kept = append(kept, it)
				}
			}
			lists = append(lists, kept)
		}
	}
	if !found {
		return nil, false, nil
	}
	return explore.MergeTop(lists, 80), true, nil
}
