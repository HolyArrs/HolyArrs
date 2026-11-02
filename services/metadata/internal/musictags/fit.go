// Package musictags builds the music catalog the app's row templates read:
// albums and artists with their listen counts, chart ranks and tags, and,
// per Browse key (genre, mood, season, decade, activity, substyle), the
// items that fit the key's recipe with a fit score T. Every install reads
// the same bundle; nothing in it depends on who asks.
package musictags

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Kinds of keys.
const (
	KindGenre    = "genre"
	KindMood     = "mood"
	KindSeason   = "season"
	KindDecade   = "decade"
	KindActivity = "activity"
	KindSub      = "sub"
	KindFacet    = "facet"
)

// Tag is one listener tag with its vote count.
type Tag struct {
	Name  string `json:"n"`
	Votes int    `json:"v"`
}

// Item is one catalog album (release group) or artist with what the fit
// needs.
type Item struct {
	ID          string
	Kind        string // "album" or "artist"
	Title       string
	Artist      string
	ArtistID    string
	Date        string
	Type        string
	AlbumTags   []Tag
	ArtistTags  []Tag
	ChartGenres []string
}

// Recipe says which items belong to a key: weighted tags (core 1, related
// 0.6), tags that drop an item, title words (seasons), chart genre names
// (genres), a year range (decades), and the tags that name the key itself
// (Own: an item matched by one of them is "tagged" with the key).
type Recipe struct {
	Key         string             `json:"key"`
	Name        string             `json:"name"`
	Kind        string             `json:"kind"`
	Tags        map[string]float64 `json:"tags,omitempty"`
	Own         []string           `json:"own,omitempty"`
	Exclude     []string           `json:"exclude,omitempty"`
	Words       []string           `json:"words,omitempty"`
	ChartGenres []string           `json:"chart_genres,omitempty"`
	From        int                `json:"from,omitempty"`
	To          int                `json:"to,omitempty"`
	MinT        float64            `json:"min_t,omitempty"`
	MinVotes    int                `json:"min_votes,omitempty"`
	// OlderThan and TopShare add albums by date and listens: released at
	// least OlderThan years ago and in the catalog's top TopShare by listens
	// (Throwback).
	OlderThan int     `json:"older_than,omitempty"`
	TopShare  float64 `json:"top_share,omitempty"`
	// Parent is a substyle's genre key.
	Parent string `json:"parent,omitempty"`
	// AlbumTagsOnly ignores artist tags (Pride: never a person's identity).
	AlbumTagsOnly bool `json:"album_tags_only,omitempty"`
}

func (r Recipe) minVotes() int {
	if r.MinVotes > 0 {
		return r.MinVotes
	}
	return 1
}

func (r Recipe) minT() float64 {
	if r.MinT > 0 {
		return r.MinT
	}
	if r.Kind == KindGenre {
		return 0.5
	}
	return 0.25
}

// strengths maps each tag with enough votes to votes / the most votes of
// any of the item's tags.
func strengths(ts []Tag, minVotes int) map[string]float64 {
	most := 0
	for _, t := range ts {
		most = max(most, t.Votes)
	}
	out := map[string]float64{}
	if most == 0 {
		return out
	}
	for _, t := range ts {
		if t.Votes >= minVotes {
			n := strings.ToLower(strings.TrimSpace(t.Name))
			out[n] = math.Max(out[n], float64(t.Votes)/float64(most))
		}
	}
	return out
}

var wordCache = map[string]*regexp.Regexp{}

func hasWord(title, word string) bool {
	re, ok := wordCache[word]
	if !ok {
		re = regexp.MustCompile(`(?i)(^|[^\p{L}\p{N}])` + regexp.QuoteMeta(word))
		wordCache[word] = re
	}
	return re.MatchString(title)
}

// Fit is the item's fit T (0..1) to the recipe, and whether it matched by
// a tag that names the key or a title word ("own").
func Fit(it Item, r Recipe) (float64, bool) {
	if r.Kind == KindDecade {
		if len(it.Date) < 4 {
			return 0, false
		}
		y, err := strconv.Atoi(it.Date[:4])
		if err != nil || y < r.From || y > r.To {
			return 0, false
		}
		return 1, true
	}
	mv := r.minVotes()
	al := strengths(it.AlbumTags, mv)
	var ar map[string]float64
	if !r.AlbumTagsOnly {
		ar = strengths(it.ArtistTags, max(mv, 2))
	}
	wAl, wAr := 0.65, 0.35
	if len(it.AlbumTags) < 3 {
		wAl, wAr = 0.4, 0.6
	}
	if len(ar) == 0 {
		wAl, wAr = 1, 0
		if len(it.AlbumTags) < 3 {
			wAl = 0.65
		}
	}
	profile := func(tag string) float64 { return wAl*al[tag] + wAr*ar[tag] }
	for _, x := range r.Exclude {
		if profile(x) >= 0.5 {
			return 0, false
		}
	}
	t := 0.0
	for tag, w := range r.Tags {
		t += w * profile(tag)
	}
	own := false
	for _, o := range r.Own {
		if al[o] > 0 || ar[o] > 0 {
			own = true
		}
	}
	for _, w := range r.Words {
		if it.Title != "" && hasWord(it.Title, w) {
			t += 0.5
			own = true
			break
		}
	}
	return math.Max(0, math.Min(1, t)), own
}

// Member is one item's place in a key: its fit and whether it is tagged
// with the key itself.
type Member struct {
	T   float64
	Own bool
}

// Members answers the keys the item belongs to. A genre needs T ≥ its
// MinT (0.5) and at least 0.75 of the item's best genre fit, so an album
// sits in its main genres only; other keys need T ≥ MinT (0.25). A chart
// genre name counts as a full genre fit.
func Members(it Item, rs []Recipe) map[string]Member {
	out := map[string]Member{}
	fits := make([]Member, len(rs))
	best := 0.0
	for i, r := range rs {
		t, own := Fit(it, r)
		if r.Kind == KindGenre {
			for _, g := range r.ChartGenres {
				for _, cg := range it.ChartGenres {
					if strings.EqualFold(g, cg) {
						t, own = 1, true
					}
				}
			}
			best = math.Max(best, t)
		}
		fits[i] = Member{t, own}
	}
	for i, r := range rs {
		m := fits[i]
		if m.T < r.minT() || m.T == 0 {
			continue
		}
		if r.Kind == KindGenre && m.T < 0.75*best {
			continue
		}
		out[r.Key] = m
	}
	return out
}
