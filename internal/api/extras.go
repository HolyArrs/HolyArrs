package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/parts"
	"github.com/CoderLuii/holyarrs/internal/ratings"
)

// TitleExtras is the optional extra artwork (fanart.tv) and outside
// ratings (MDBList) for a title page. Empty fields mean none: the page
// keeps TMDB's art and hides the ratings strip.
type TitleExtras struct {
	Logo  string `json:"logo,omitempty"`
	Thumb string `json:"thumb,omitempty"`
	// Banner is the wide fanart.tv banner (about 1000x185) the Library
	// table shows per row; empty when fanart has none.
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
	key := kind + ":" + id
	c := titleExtrasCache
	c.mu.Lock()
	e, ok := c.seen[key]
	c.mu.Unlock()
	if ok && time.Since(e.at) < 6*time.Hour {
		writeJSON(w, http.StatusOK, e.v)
		return
	}
	v := TitleExtras{Scores: []OutsideScore{}}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	// Only settled answers are kept; a server hiccup is asked again.
	settled := true
	if a.s.ExtraArt != nil {
		art, err := a.s.ExtraArt(ctx, kind, id)
		if err == nil {
			v.Logo, v.Thumb, v.Banner, v.Backgrounds = art.Logo, art.Thumb, art.Banner, art.Backgrounds
		}
		settled = settled && (err == nil || errors.Is(err, metadata.ErrNotFound))
	}
	if a.s.ExtraRatings != nil && kind != "artist" {
		rt, err := a.s.ExtraRatings(ctx, kind, id)
		if err == nil {
			v.Scores = shownScores(rt.Scores)
		}
		settled = settled && (err == nil || errors.Is(err, metadata.ErrNotFound))
	}
	if !settled {
		writeJSON(w, http.StatusOK, v)
		return
	}
	c.mu.Lock()
	if len(c.seen) > 5000 {
		c.seen = map[string]extrasEntry{}
	}
	c.seen[key] = extrasEntry{v: v, at: time.Now()}
	c.mu.Unlock()
	writeJSON(w, http.StatusOK, v)
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
