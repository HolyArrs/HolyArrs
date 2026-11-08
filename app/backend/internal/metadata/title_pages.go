package metadata

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// extrasFresh is how long kept extras are served before a background refresh.
const extrasFresh = 6 * time.Hour

// wikiAwardsMax is how many awards the page lists (wins first).
const wikiAwardsMax = 30

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

// WikiFacts is a title's or person's awards and Wikipedia bio for the page
// (GET /api/v1/wiki/{kind}/{id}). Empty fields mean none.
type WikiFacts struct {
	// Badges are short labels ("Oscar winner", "3 Emmys"), most notable first.
	Badges     []string     `json:"badges"`
	Tallies    []WikiTally  `json:"tallies"`
	Awards     []WikiAward  `json:"awards"`
	Born       string       `json:"born,omitempty"`
	Died       string       `json:"died,omitempty"`
	BirthPlace string       `json:"birth_place,omitempty"`
	DeathPlace string       `json:"death_place,omitempty"`
	Bio        *WikiSummary `json:"bio"`
	// Image is a person's backup photo (Wikimedia Commons, else TVmaze), used when TMDB has none.
	Image string `json:"image,omitempty"`
}

// TitlePages serves the extras and wiki routes of the title and person
// pages, and keeps their answers in memory and in lookup_cache.
type TitlePages struct {
	Store *database.Store
	// Server returns the metadata server's per-title reads at call time.
	Server  func() ServerReads
	Ages    AgeLimits
	Explore ExploreCache
	Music   MusicAnswers
	// PartOn says the part of the app that shows kind (movie, show,
	// artist) is switched on.
	PartOn func(kind string) bool

	extras extrasCache
	wiki   wikiCache
	awards awardCache
}

type extrasCache struct {
	mu   sync.Mutex
	seen map[string]extrasEntry
}

type extrasEntry struct {
	v  TitleExtras
	at time.Time
}

type wikiCache struct {
	mu   sync.Mutex
	seen map[string]wikiEntry
}

type wikiEntry struct {
	v  WikiFacts
	at time.Time
}

type awardSet struct {
	ids map[string]bool
	at  time.Time
	err error
}

type awardCache struct {
	mu    sync.Mutex
	kinds map[string]awardSet
}

// minOutsideVotes is the fewest votes (or reviews) a score needs to be
// shown; a score with an unknown count is hidden.
var minOutsideVotes = map[string]int64{"imdb": 1000, "rt_critics": 20, "rt_audience": 250, "metacritic": 7, "letterboxd": 500, "trakt": 100}

// Routes registers the extras and wiki routes.
func (p *TitlePages) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/extras/{kind}/{id}", p.titleExtras)
	m.HandleFunc("GET /api/v1/wiki/{kind}/{id}", p.wikiFacts)
}

// titleExtras answers GET /api/v1/extras/{kind}/{id} (movie or show: TMDB
// id; artist: MusicBrainz id). It never fails the page: anything the
// metadata server cannot give comes back empty.
func (p *TitlePages) titleExtras(w http.ResponseWriter, r *http.Request) {
	kind, id := r.PathValue("kind"), strings.ToLower(r.PathValue("id"))
	if (kind == "movie" || kind == "show") && !DigitsOnly(id) || kind == "artist" && !ValidMBID(id) || kind != "movie" && kind != "show" && kind != "artist" {
		webserver.Fail(w, http.StatusBadRequest, "unknown title")
		return
	}
	// Part and Kid gates come before the shared cache: its key does not
	// carry the Kid limit.
	if !p.PartOn(kind) {
		webserver.Fail(w, http.StatusConflict, "that part is off")
		return
	}
	if p.Ages.Limited(w, r) {
		return
	}
	if max := p.Ages.Max(r); max != "" {
		// Music never passes a limit (see ratings).
		if kind == "artist" {
			webserver.Fail(w, http.StatusForbidden, AboveLimit)
			return
		}
		if !p.ratingAllows(w, r, max, kind, id) {
			return
		}
	}
	p.Explore.Caching(w, r, "")
	// An artist's answer carries its pictures' records (the logo's shape, the
	// wide background's colour), like every music answer.
	write := func(v TitleExtras) {
		if kind == "artist" {
			p.Music.WriteJSON(w, r, v)
			return
		}
		webserver.WriteJSON(w, http.StatusOK, v)
	}
	key := kind + ":" + id
	if e, ok := p.keptExtras(r.Context(), key); ok {
		// Stored answers are served at once; an old one is refreshed in
		// the background for the next view.
		if time.Since(e.at) >= extrasFresh {
			RefreshOnce("extras|"+key, func(ctx context.Context) {
				if v, settled := p.fetchExtras(ctx, kind, id); settled {
					p.keepExtras(ctx, key, v)
				}
			})
		}
		write(e.v)
		return
	}
	v, settled := p.fetchExtras(r.Context(), kind, id)
	if settled {
		p.keepExtras(r.Context(), key, v)
	} else {
		// A hiccup's answer is not kept; the browser asks again next view.
		w.Header().Set("Cache-Control", "private, no-cache")
	}
	write(v)
}

// WarmExtras stores a movie's or show's extras ahead when they are missing
// or old, for the title warmer.
func (p *TitlePages) WarmExtras(ctx context.Context, kind, id string) {
	s := p.Server()
	if s.Art == nil && s.Ratings == nil {
		return
	}
	key := kind + ":" + id
	p.extras.mu.Lock()
	e, ok := p.extras.seen[key]
	p.extras.mu.Unlock()
	if !ok {
		if k, found := LoadKept[TitleExtras](ctx, p.Store, "extras|"+key); found {
			e, ok = extrasEntry{v: k.V, at: k.At}, true
		}
	}
	if !ok || time.Since(e.at) >= extrasFresh {
		if v, settled := p.fetchExtras(ctx, kind, id); settled {
			p.keepExtras(ctx, key, v)
		}
		return
	}
	p.extras.keep(key, e)
}

// keptExtras reads a kept answer from memory, else from disk.
func (p *TitlePages) keptExtras(ctx context.Context, key string) (extrasEntry, bool) {
	p.extras.mu.Lock()
	e, ok := p.extras.seen[key]
	p.extras.mu.Unlock()
	if ok {
		return e, true
	}
	k, found := LoadKept[TitleExtras](ctx, p.Store, "extras|"+key)
	if !found {
		return extrasEntry{}, false
	}
	e = extrasEntry{v: k.V, at: k.At}
	p.extras.keep(key, e)
	return e, true
}

// keepExtras keeps a settled answer in memory and on disk.
func (p *TitlePages) keepExtras(ctx context.Context, key string, v TitleExtras) {
	p.extras.keep(key, extrasEntry{v: v, at: time.Now()})
	KeepOnDisk(ctx, p.Store, "extras|"+key, v)
}

func (c *extrasCache) keep(key string, e extrasEntry) {
	c.mu.Lock()
	if c.seen == nil {
		c.seen = map[string]extrasEntry{}
	}
	webserver.KeepUnder(c.seen, 5000, func(e extrasEntry) time.Time { return e.at })
	c.seen[key] = e
	c.mu.Unlock()
}

// fetchExtras asks the metadata server for a title's extras, art and
// ratings together; settled is false after a server hiccup (not kept).
func (p *TitlePages) fetchExtras(parent context.Context, kind, id string) (TitleExtras, bool) {
	v := TitleExtras{Scores: []OutsideScore{}}
	ctx, cancel := context.WithTimeout(parent, 8*time.Second)
	defer cancel()
	settledArt, settledRatings := true, true
	var wg sync.WaitGroup
	if kind == "artist" {
		// An artist's pictures come only from our metadata server: its
		// stored wide background, logo and photo, through the picture route.
		var wide, logo, thumb string
		var hasWide bool
		wg.Add(3)
		go func() { defer wg.Done(); wide, hasWide = p.Music.ArtistPicture(ctx, "wide", id, "1920") }()
		go func() { defer wg.Done(); logo, _ = p.Music.ArtistPicture(ctx, "logo", id, "1200") }()
		go func() { defer wg.Done(); thumb, _ = p.Music.ArtistPicture(ctx, "artist", id, "500") }()
		wg.Wait()
		if hasWide {
			v.Backgrounds = []string{wide}
		}
		v.Logo, v.Thumb = logo, thumb
		// An artist with no wide photo is asked again: the server's art
		// source may only have been out of budget for a moment.
		return v, hasWide
	}
	s := p.Server()
	if s.Art != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			art, err := s.Art(ctx, kind, id)
			if err == nil {
				v.Logo, v.Thumb, v.Banner = s.ArtLink(art.Logo), s.ArtLink(art.Thumb), s.ArtLink(art.Banner)
				for _, b := range art.Backgrounds {
					if l := s.ArtLink(b); l != "" {
						v.Backgrounds = append(v.Backgrounds, l)
					}
				}
			}
			settledArt = err == nil || errors.Is(err, ErrNotFound)
		}()
	}
	var scores []OutsideScore
	if s.Ratings != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rt, err := s.Ratings(ctx, kind, id)
			if err == nil {
				scores = shownScores(rt.Scores)
			}
			settledRatings = err == nil || errors.Is(err, ErrNotFound)
		}()
	}
	wg.Wait()
	if scores != nil {
		v.Scores = scores
	}
	return v, settledArt && settledRatings
}

func shownScores(in []ServerScore) []OutsideScore {
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

// wikiFacts answers GET /api/v1/wiki/{kind}/{id} (movie, show or person;
// TMDB id). It never fails the page: anything the metadata server cannot
// give comes back empty.
func (p *TitlePages) wikiFacts(w http.ResponseWriter, r *http.Request) {
	kind, id := r.PathValue("kind"), r.PathValue("id")
	if kind != "movie" && kind != "show" && kind != "person" || !DigitsOnly(id) {
		webserver.Fail(w, http.StatusBadRequest, "unknown title or person")
		return
	}
	// The Kid gate comes before the shared cache: its key does not carry
	// the Kid limit (the same gate as extras and strip).
	if p.Ages.Limited(w, r) {
		return
	}
	if max := p.Ages.Max(r); kind != "person" && max != "" && !p.ratingAllows(w, r, max, kind, id) {
		return
	}
	p.Explore.Caching(w, r, "")
	// A person's name lets the server look for a TVmaze photo; it is part of the cache key.
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if len(name) > 100 {
		name = ""
	}
	key := kind + ":" + id + ":" + name
	c := &p.wiki
	c.mu.Lock()
	e, ok := c.seen[key]
	c.mu.Unlock()
	// Only the name-free answer is shared on disk: a caller's name (which
	// can steer the photo lookup) never decides what another caller gets,
	// nor grows lookup_cache.
	disk := name == ""
	if !ok && disk {
		if k, found := LoadKept[WikiFacts](r.Context(), p.Store, "wiki|"+kind+":"+id); found {
			e, ok = wikiEntry{v: k.V, at: k.At}, true
			c.put(key, e, false)
		}
	}
	if ok {
		// Kept answers are served at once; an old one is refreshed in the
		// background for the next view.
		if time.Since(e.at) >= 6*time.Hour {
			RefreshOnce("wiki|"+key, func(ctx context.Context) {
				if v, settled := p.fetchWiki(ctx, kind, id, name); settled {
					c.put(key, wikiEntry{v: v, at: time.Now()}, false)
					if disk {
						KeepOnDisk(ctx, p.Store, "wiki|"+kind+":"+id, v)
					}
				}
			})
		}
		webserver.WriteJSON(w, http.StatusOK, e.v)
		return
	}
	v, settled := p.fetchWiki(r.Context(), kind, id, name)
	// Only settled answers are kept; a server hiccup is asked again.
	if !settled {
		w.Header().Set("Cache-Control", "private, no-cache")
		webserver.WriteJSON(w, http.StatusOK, v)
		return
	}
	c.put(key, wikiEntry{v: v, at: time.Now()}, true)
	if disk {
		KeepOnDisk(r.Context(), p.Store, "wiki|"+kind+":"+id, v)
	}
	webserver.WriteJSON(w, http.StatusOK, v)
}

// put keeps one answer; capped, a full cache starts over first.
func (c *wikiCache) put(key string, e wikiEntry, capped bool) {
	c.mu.Lock()
	if c.seen == nil || capped && len(c.seen) > 5000 {
		c.seen = map[string]wikiEntry{}
	}
	c.seen[key] = e
	c.mu.Unlock()
}

// fetchWiki asks the metadata server for a title's or person's facts;
// settled is false after a server hiccup.
func (p *TitlePages) fetchWiki(parent context.Context, kind, id, name string) (WikiFacts, bool) {
	v := WikiFacts{Badges: []string{}, Tallies: []WikiTally{}, Awards: []WikiAward{}}
	ctx, cancel := context.WithTimeout(parent, 8*time.Second)
	defer cancel()
	s := p.Server()
	var ent WikiEntity
	err := ErrNotFound
	if kind == "person" && s.WikiPerson != nil {
		ent, err = s.WikiPerson(ctx, id, name)
	} else if kind != "person" && s.WikiTitle != nil {
		ent, err = s.WikiTitle(ctx, kind, id)
	}
	if err == nil {
		if ent.Tallies != nil {
			v.Tallies = ent.Tallies
		}
		v.Badges = AwardBadges(v.Tallies)
		if len(ent.Awards) > 0 {
			v.Awards = ent.Awards[:min(len(ent.Awards), wikiAwardsMax)]
		}
		v.Born, v.Died, v.BirthPlace, v.DeathPlace = ent.Born, ent.Died, ent.BirthPlace, ent.DeathPlace
		if kind == "person" {
			v.Bio = ent.Summary
			if l := s.ArtLink(ent.Image); l != "" {
				v.Image = l
			} else if strings.HasPrefix(ent.Image, "https://upload.wikimedia.org/") || strings.HasPrefix(ent.Image, "https://static.tvmaze.com/") {
				v.Image = ent.Image
			}
		}
	}
	return v, err == nil || errors.Is(err, ErrNotFound)
}

// ratingAllows checks a movie or show against the Kid limit max: the
// Library's rating, else the title's details. It writes 403 and returns
// false when the title is above the limit or its rating is unknown.
func (p *TitlePages) ratingAllows(w http.ResponseWriter, r *http.Request, max, kind, id string) bool {
	rating, inLibrary := p.Ages.LibraryRating(r.Context(), kind, id)
	if !inLibrary {
		det, err := p.Explore.Details(r.Context(), kind, id)
		if err != nil {
			webserver.Fail(w, http.StatusForbidden, AboveLimit)
			return false
		}
		rating = det.Rating
	}
	if rating == "" || !RatingAllows(max, kind, rating) {
		webserver.Fail(w, http.StatusForbidden, AboveLimit)
		return false
	}
	return true
}

// AwardWinners is the server's award-winning TMDB IDs for kind (movie or
// show), kept a day (a failure ten minutes; an older good copy wins).
func (p *TitlePages) AwardWinners(ctx context.Context, kind string) (map[string]bool, bool) {
	read := p.Server().AwardWinners
	if read == nil {
		return nil, false
	}
	c := &p.awards
	c.mu.Lock()
	s, ok := c.kinds[kind]
	c.mu.Unlock()
	if ok && (s.err == nil && time.Since(s.at) < 24*time.Hour || s.err != nil && time.Since(s.at) < 10*time.Minute) {
		return s.ids, len(s.ids) > 0
	}
	list, err := read(ctx, kind)
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
	c.mu.Lock()
	if c.kinds == nil {
		c.kinds = map[string]awardSet{}
	}
	c.kinds[kind] = next
	c.mu.Unlock()
	return next.ids, len(next.ids) > 0
}
