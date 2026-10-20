package explore

import (
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Item is one movie or show candidate, with what the engine scores it on.
type Item struct {
	TMDBID   string   `json:"tmdb_id"`
	Kind     string   `json:"kind"`
	Name     string   `json:"name"`
	Year     int      `json:"year,omitempty"`
	Poster   string   `json:"poster,omitempty"`
	Backdrop string   `json:"backdrop,omitempty"`
	Overview string   `json:"overview,omitempty"`
	Genres   []string `json:"genres"`
	People   []string `json:"people,omitempty"`
	Pop      float64  `json:"pop,omitempty"`
	Vote     float64  `json:"vote,omitempty"`
	Votes    int      `json:"votes,omitempty"`
	Released string   `json:"released,omitempty"`
	// Collection is the TMDB collection (franchise) when known.
	Collection int64 `json:"collection,omitempty"`
	Owned      bool  `json:"owned,omitempty"`
	Monitored  bool  `json:"monitored,omitempty"`
	// HH is the household signal (asks).
	HH float64 `json:"hh,omitempty"`
	// Sim is the similarity to the row's seed, 0-1.
	Sim   float64 `json:"sim,omitempty"`
	Score float64 `json:"score"`
}

// Key is kind:tmdb id.
func (it Item) Key() string { return it.Kind + ":" + it.TMDBID }

var sequelTail = regexp.MustCompile(`(?i)(\s*[:\-–]\s.*$|\s+(part\s+)?([0-9]+|ii|iii|iv|v|vi)$)`)

// Franchise groups sequels: the TMDB collection when known; a show is
// its own franchise; otherwise the movie name without its sequel tail.
func (it Item) Franchise() string {
	if it.Collection > 0 {
		return "c" + strconv.FormatInt(it.Collection, 10)
	}
	if it.Kind == "show" {
		return it.Key()
	}
	stem := strings.ToLower(strings.TrimSpace(sequelTail.ReplaceAllString(it.Name, "")))
	if stem == "" {
		return it.Key()
	}
	return "m:" + stem
}

// Row types.
const (
	TypeJustArrived   = "just_arrived"
	TypeYourAsks      = "your_asks"
	TypeTop10         = "top10"
	TypeTopPicks      = "top_picks"
	TypeTrending      = "trending"
	TypeServices      = "services"
	TypeComingSoon    = "coming_soon"
	TypeBecauseHave   = "because_have"
	TypePerson        = "person"
	TypeGenre         = "genre"
	TypePopularGenre  = "popular_genre"
	TypePopularDecade = "popular_decade"
	TypeNewServices   = "new_services"
	TypeBest          = "best"
	TypeHousehold     = "household"
	TypeDecade        = "decade"
	TypeHidden        = "hidden"
)

// rowKind is what the catalog (section 4) says of one row type.
type rowKind struct {
	min      int
	prior    float64
	personal bool
}

var catalog = map[string]rowKind{
	TypeJustArrived:   {3, 0, false},
	TypeYourAsks:      {1, 0, false},
	TypeTop10:         {5, 0, false},
	TypeTopPicks:      {8, 0.95, true},
	TypeTrending:      {8, 0, false},
	TypeServices:      {8, 0.6, false},
	TypeComingSoon:    {4, 0, false},
	TypeBecauseHave:   {6, 0.8, true},
	TypePerson:        {6, 0.6, true},
	TypeGenre:         {8, 0.65, true},
	TypePopularGenre:  {8, 0.5, false},
	TypePopularDecade: {8, 0.45, false},
	TypeNewServices:   {6, 0.75, false},
	TypeBest:          {8, 0.55, false},
	TypeHousehold:     {4, 0.7, false},
	TypeDecade:        {8, 0.45, false},
	TypeHidden:        {6, 0.4, false},
}

// MinItems is the fewest items a row of type t shows.
func MinItems(t string) int { return catalog[t].min }

// Personal says a row of type t names a seed, a person or "for you".
func Personal(t string) bool { return catalog[t].personal }

// RowMax is the most cards a row shows.
const RowMax = 20

// Row is one row of the page.
type Row struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Title string `json:"title"`
	// Seed is the seed key (because rows) and SeedWeight its weight.
	Seed       string  `json:"seed,omitempty"`
	SeedWeight float64 `json:"seed_weight,omitempty"`
	Genre      string  `json:"genre,omitempty"`
	Items      []Item  `json:"items"`
	// Score is rowScore before the daily jitter; Fresh is typeFreshness.
	Score float64 `json:"score"`
	Fresh float64 `json:"fresh"`
	Wide  bool    `json:"wide,omitempty"`
	// SeedName, Person and Decade fill title slots of because, person
	// and decade rows.
	SeedName string `json:"seed_name,omitempty"`
	Person   string `json:"person,omitempty"`
	Decade   int    `json:"decade,omitempty"`
	// Set by TitlePage: the plain title, the small line under the title,
	// the template shown (if any) and its layer. Counted is the item count
	// a {count} title states.
	Plain    string `json:"plain,omitempty"`
	Subtitle string `json:"subtitle,omitempty"`
	TitleID  string `json:"title_id,omitempty"`
	Layer    string `json:"title_layer,omitempty"`
	Counted  int    `json:"-"`
}

// rating is the Bayesian-shrunk vote average, 0-1.
func rating(it Item) float64 {
	v := float64(it.Votes)
	return (v*it.Vote + 500*6.5) / (v + 500) / 10
}

// fresh is exp(-days since release / d); unknown dates count as old.
func fresh(it Item, now time.Time, d float64) float64 {
	t, err := time.Parse("2006-01-02", it.Released)
	if err != nil {
		return 0
	}
	days := now.Sub(t).Hours() / 24
	if days < 0 {
		days = 0
	}
	return math.Exp(-days / d)
}

// Scorer holds what item scores need: the member's profile, the time and
// the highest popularity in the candidate set (pop is normalised to it).
type Scorer struct {
	Profile Profile
	Now     time.Time
	MaxPop  float64
	MaxHH   float64
}

func (s Scorer) pop(it Item) float64 {
	if s.MaxPop <= 0 {
		return 0
	}
	return clamp(it.Pop / s.MaxPop)
}

// ItemScore scores one item for a row type (the catalog's Ranking column).
func (s Scorer) ItemScore(t string, it Item) float64 {
	aff, r, p := s.Profile.Affinity(it), rating(it), s.pop(it)
	switch t {
	case TypeTopPicks:
		hh := 0.0
		if s.MaxHH > 0 {
			hh = clamp(it.HH / s.MaxHH)
		}
		return 0.45*aff + 0.20*r + 0.15*p + 0.10*fresh(it, s.Now, 365) + 0.10*hh
	case TypeServices:
		return 0.40*p + 0.30*aff + 0.20*r + 0.10*fresh(it, s.Now, 730)
	case TypeComingSoon:
		return 0.6*p + 0.4*aff
	case TypeBecauseHave:
		return 0.50*it.Sim + 0.20*r + 0.15*aff + 0.15*p
	case TypePerson:
		return 0.5*r + 0.3*p + 0.2*aff
	case TypeGenre:
		return 0.35*aff + 0.30*r + 0.25*p + 0.10*fresh(it, s.Now, 1095)
	case TypePopularGenre, TypePopularDecade:
		// Non-personal: rating-weighted, well-known titles.
		return 0.6*r + 0.4*p
	case TypeNewServices:
		return 0.5*fresh(it, s.Now, 30) + 0.3*aff + 0.2*p
	case TypeBest:
		return 0.6*r + 0.25*aff + 0.15*p
	case TypeHousehold:
		return it.HH
	case TypeDecade:
		return 0.5*r + 0.3*aff + 0.2*p
	case TypeHidden:
		return 0.6*r + 0.4*aff
	}
	return 0
}

// dropsOwned are the row types that leave out Library titles.
var dropsOwned = map[string]bool{TypeTopPicks: true, TypePerson: true}

// Rank scores and orders a row's items for its type, applying the
// type's own rules (trending keeps upstream order with a tilt and its top
// 3 fixed; Coming soon puts owned or monitored first).
func (s Scorer) Rank(t string, items []Item) []Item {
	out := make([]Item, 0, len(items))
	for _, it := range items {
		if dropsOwned[t] && it.Owned {
			continue
		}
		out = append(out, it)
	}
	n := len(out)
	switch t {
	case TypeTop10, TypeJustArrived, TypeYourAsks:
		return out
	case TypeTrending:
		for i := range out {
			out[i].Score = 0.85*(1-float64(i)/float64(max(n, 1))) + 0.15*s.Profile.Affinity(out[i])
		}
		if n > 3 {
			rest := out[3:]
			sort.SliceStable(rest, func(i, j int) bool { return rest[i].Score > rest[j].Score })
		}
		// Owned titles stay out of the top 5 slots.
		var res, later []Item
		for _, it := range out {
			if len(res) < 5 && it.Owned {
				later = append(later, it)
				continue
			}
			if len(res) == 5 && len(later) > 0 {
				res, later = append(res, later...), nil
			}
			res = append(res, it)
		}
		return append(res, later...)
	case TypeComingSoon:
		for i := range out {
			out[i].Score = s.ItemScore(t, out[i])
		}
		sort.SliceStable(out, func(i, j int) bool {
			a, b := out[i].Owned || out[i].Monitored, out[j].Owned || out[j].Monitored
			if a != b {
				return a
			}
			if out[i].Score != out[j].Score {
				return out[i].Score > out[j].Score
			}
			return out[i].Released < out[j].Released
		})
		return out
	}
	for i := range out {
		out[i].Score = s.ItemScore(t, out[i])
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Key() < out[j].Key()
	})
	return out
}

// meanTop8 is the mean score of a row's best 8 items.
func meanTop8(items []Item) float64 {
	n := min(8, len(items))
	if n == 0 {
		return 0
	}
	sum := 0.0
	for _, it := range items[:n] {
		sum += it.Score
	}
	return sum / float64(n)
}

// RowScore is 0.40·seedWeight_norm + 0.30·meanTop8 + 0.15·typeFreshness +
// 0.15·typePrior (5.4). maxSeed normalises the seed weight.
func RowScore(r Row, maxSeed float64) float64 {
	sw := 0.0
	if maxSeed > 0 {
		sw = clamp(r.SeedWeight / maxSeed)
	}
	return 0.40*sw + 0.30*meanTop8(r.Items) + 0.15*r.Fresh + 0.15*catalog[r.Type].prior
}

// Title words for each row type.
func RowTitle(t, arg string) string {
	switch t {
	case TypeJustArrived:
		return "Just arrived"
	case TypeYourAsks:
		return "Your asks, on the way"
	case TypeTop10:
		return "Top 10 today"
	case TypeTopPicks:
		return "Top picks for you"
	case TypeTrending:
		return "Trending this week"
	case TypeServices:
		return "On your services"
	case TypeComingSoon:
		return "Coming soon"
	case TypeBecauseHave:
		return "Because you have " + arg
	case TypePerson:
		return "More from " + arg
	case TypeGenre:
		return arg + " picks for you"
	case TypePopularGenre:
		return "Popular " + strings.ToLower(arg)
	case TypePopularDecade, TypeDecade:
		return arg
	case TypeNewServices:
		return "New on your services"
	case TypeBest:
		return "Best new releases"
	case TypeHousehold:
		return "Popular in your household"
	case TypeHidden:
		return "Hidden gems"
	}
	return arg
}
