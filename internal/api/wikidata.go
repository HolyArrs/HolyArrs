package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs/internal/explore"
	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/ratings"
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
	// Image is a person's backup photo (Wikimedia Commons, else TVmaze), used when TMDB has none.
	Image string `json:"image,omitempty"`
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
	// The Kid gate comes before the shared cache: its key does not carry
	// the Kid limit (the same gate as extras and strip).
	if a.kidLimited(w, r) || kind != "person" && !a.wikiKidAllows(w, r, kind, id) {
		return
	}
	a.exploreCaching(w, r, "")
	// A person's name lets the server look for a TVmaze photo; it is part of the cache key.
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if len(name) > 100 {
		name = ""
	}
	key := kind + ":" + id + ":" + name
	// The kept copy on disk is keyed without the caller's name, so a
	// caller cannot grow lookup_cache with names.
	c := wikiFactsCache
	c.mu.Lock()
	e, ok := c.seen[key]
	c.mu.Unlock()
	if !ok {
		if k, found := loadKept[WikiFacts](r.Context(), a, "wiki|"+kind+":"+id); found {
			e, ok = wikiEntry(k), true
			c.mu.Lock()
			c.seen[key] = e
			c.mu.Unlock()
		}
	}
	if ok {
		// Kept answers are served at once; an old one is refreshed in the
		// background for the next view.
		if time.Since(e.at) >= 6*time.Hour {
			refreshOnce("wiki|"+kind+":"+id, func(ctx context.Context) {
				if v, settled := a.fetchWiki(ctx, kind, id, name); settled {
					c.mu.Lock()
					c.seen[key] = wikiEntry{v: v, at: time.Now()}
					c.mu.Unlock()
					keepOnDisk(ctx, a, "wiki|"+kind+":"+id, v)
				}
			})
		}
		writeJSON(w, http.StatusOK, e.v)
		return
	}
	v, settled := a.fetchWiki(r.Context(), kind, id, name)
	// Only settled answers are kept; a server hiccup is asked again.
	if !settled {
		w.Header().Set("Cache-Control", "private, no-cache")
		writeJSON(w, http.StatusOK, v)
		return
	}
	c.mu.Lock()
	if len(c.seen) > 5000 {
		c.seen = map[string]wikiEntry{}
	}
	c.seen[key] = wikiEntry{v: v, at: time.Now()}
	c.mu.Unlock()
	keepOnDisk(r.Context(), a, "wiki|"+kind+":"+id, v)
	writeJSON(w, http.StatusOK, v)
}

// fetchWiki asks the metadata server for a title's or person's facts;
// settled is false after a server hiccup.
func (a *API) fetchWiki(parent context.Context, kind, id, name string) (WikiFacts, bool) {
	v := WikiFacts{Badges: []string{}, Tallies: []metadata.WikiTally{}, Awards: []metadata.WikiAward{}}
	ctx, cancel := context.WithTimeout(parent, 8*time.Second)
	defer cancel()
	var ent metadata.WikiEntity
	err := metadata.ErrNotFound
	if kind == "person" && a.s.WikiPerson != nil {
		ent, err = a.s.WikiPerson(ctx, id, name)
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
			if l := a.serverArtLink(ent.Image); l != "" {
				v.Image = l
			} else if strings.HasPrefix(ent.Image, "https://upload.wikimedia.org/") || strings.HasPrefix(ent.Image, "https://static.tvmaze.com/") {
				v.Image = ent.Image
			}
		}
	}
	return v, err == nil || errors.Is(err, metadata.ErrNotFound)
}

// wikiKidAllows checks a movie or show against the Kid limit, as extras
// does; it writes 403 and returns false when the title is above it.
func (a *API) wikiKidAllows(w http.ResponseWriter, r *http.Request, kind, id string) bool {
	max := a.kidMax(r)
	if max == "" {
		return true
	}
	rating, inLibrary := a.libraryRating(r.Context(), kind, id)
	if !inLibrary {
		d, ok := metadata.DiscovererFor(r.Context(), a.s.Metadata)
		if !ok {
			fail(w, http.StatusForbidden, aboveLimit)
			return false
		}
		dkey := "details|" + kind + "|" + id
		e, cached := a.discover.get(dkey)
		if !cached || time.Since(e.at) >= discoverTTL["details"] {
			det, err := d.Details(r.Context(), kind, id)
			if err != nil {
				fail(w, http.StatusForbidden, aboveLimit)
				return false
			}
			e = discoverEntry{at: time.Now(), details: &det}
			a.discover.put(dkey, e)
		}
		rating = e.details.Rating
	}
	if rating == "" || !ratings.Allows(max, kind, rating) {
		fail(w, http.StatusForbidden, aboveLimit)
		return false
	}
	return true
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
