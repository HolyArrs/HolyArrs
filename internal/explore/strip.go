package explore

import (
	"hash/fnv"
	"math"
	"math/rand/v2"
	"sort"
	"strings"
	"time"
)

// The category strip under the Explore hero: StripMax Browse categories
// picked for one member. Each candidate is scored by the member's taste
// (genres and keywords of their own asks and follows, the household
// Library weaker), popularity, the member's own opens of the strip,
// the season and the hour, and fresh releases. About a fifth of the
// tiles are an explore slot: categories the member was not shown in the
// last 7 days, so the strip never narrows. At most stripSectionCap tiles
// come from one section and one from a near-duplicate (KIN) group.

// StripMax is how many categories the strip shows before All categories.
const StripMax = 18

// stripSectionCap is the most tiles one section gets.
const stripSectionCap = 3

// StripExploreShare is the share of the strip kept for categories the
// member was not shown in the last StripRecentDays.
const StripExploreShare = 0.2

// StripRecentDays: a category shown within this many days is not new to
// the member (the explore slot skips it).
const StripRecentDays = 7

// StripCountDays is how far back the member's strip counts are read.
const StripCountDays = 30

// stripSkipDecay is the days over which a skip (shown, not opened) fades.
const stripSkipDecay = 14.0

// stripFreshDays: a category whose top titles include a release this
// recent gets the freshness boost.
const stripFreshDays = 14

// stripKin groups near-duplicates; the strip shows one of a group.
var stripKin = [][]string{
	{"thriller", "edge-of-your-seat", "adrenaline", "suspense", "conspiracies", "spies", "assassins", "spy"},
	{"horror", "scary", "zombies", "vampires", "monsters", "haunted houses", "serial killers"},
	{"comedy", "laugh out loud", "quirky", "satirical", "dark comedy", "sitcoms"},
	{"romance", "swoon-worthy", "love triangles", "weddings"},
	{"crime", "gritty", "heists", "gangsters", "cops", "cop procedurals", "con artists", "prison"},
	{"mystery", "whodunits", "mind-bending", "nordic noir"},
	{"drama", "tearjerkers", "slow burn", "thought-provoking"},
	{"feel-good", "uplifting", "inspiring", "cozy", "nostalgic"},
	{"family", "kids", "animals", "fairy tales", "friendship", "family ties", "90s family", "japanese family films"},
	{"animation", "classic animation", "french animation", "animated adventures"},
	{"history", "world war ii", "cold war", "victorian era", "ancient worlds", "medieval", "classics before 1970"},
	{"space", "aliens", "robots & ai", "cyberpunk", "parallel worlds", "time travel", "dystopia", "post-apocalyptic", "sci-fi"},
	{"fantasy", "fantasy worlds", "magic", "dragons", "epic"},
	{"dark", "revenge"},
	{"japanese", "anime"},
	{"spanish", "latin american"},
	{"sports", "sports stories"},
	{"medical", "hospitals"},
	{"legal", "courtroom"},
	{"teen", "high school", "coming of age"},
	{"islands", "underwater", "summer & beach"},
	{"holiday", "snow and ice"},
	{"war"},
	{"documentary", "based on a true story"},
	{"disaster"},
}

// StripKin is a category name's near-duplicate group, or the name itself.
func StripKin(name string) string {
	n := strings.ToLower(name)
	for i, g := range stripKin {
		for _, x := range g {
			if x == n {
				return "kin" + string(rune('a'+i))
			}
		}
	}
	return n
}

// stripSeason are the categories boosted in a month.
var stripSeason = map[time.Month][]string{
	time.October:  {"halloween", "horror", "scary", "haunted", "zombies", "vampires", "monsters"},
	time.June:     {"summer", "islands", "underwater", "road-trips"},
	time.July:     {"summer", "islands", "underwater", "road-trips"},
	time.August:   {"summer", "islands", "underwater", "road-trips"},
	time.November: {"holiday", "cozy", "family"},
	time.December: {"holiday", "snow", "cozy", "family"},
}

// stripEvening are boosted from 18:00 to midnight, local.
var stripEvening = []string{"cozy", "feel-good", "family", "uplifting"}

// stripWeekendMorning are boosted on Saturday and Sunday from 6:00 to noon.
var stripWeekendMorning = []string{"kids", "animation", "family", "classic-animation", "fairy-tales", "animals", "french-animation", "world-adventures", "japanese-family"}

// StripDefaults are the editor's picks for a member with no taste signal
// yet, every section covered; a Kid gets their own.
var StripDefaults = []string{"action", "comedy", "drama", "thriller", "animation", "documentary", "feel-good", "mind-bending", "edge",
	"heists", "time-travel", "true-story", "space", "fantasy-worlds", "90s", "80s", "korean", "british", "books", "anime"}

// StripKidDefaults are a Kid's cold-start picks.
var StripKidDefaults = []string{"animation", "family", "comedy", "kids", "feel-good", "funny", "animals", "fairy-tales", "magic",
	"space", "underwater", "classic-animation", "french-animation", "books", "anime"}

// StripCand is one category the strip may show.
type StripCand struct {
	// Key is the category id, or the lower-case genre name for a page
	// genre tile.
	Key  string
	Tile CategoryTile
	// Pop is the popularity of its top titles; Fresh says one of them was
	// released in the last stripFreshDays.
	Pop   float64
	Fresh bool
	// Genres are its top titles' genres (repeats count); Keywords the
	// category's own TMDB keyword names.
	Genres, Keywords []string
}

// NewStripCand makes a candidate from a tile and the category's titles.
func NewStripCand(t CategoryTile, items []Item, now time.Time) StripCand {
	c := StripCand{Key: t.Category, Tile: t}
	if c.Key == "" {
		c.Key = strings.ToLower(t.Name)
		c.Genres = []string{t.GenreName}
	}
	if cat, ok := CategoryByID(t.Category); ok {
		for _, q := range []*CatQuery{cat.Movie, cat.Show} {
			if q != nil {
				c.Keywords = append(c.Keywords, q.Keywords...)
			}
		}
	}
	top := append([]Item{}, items...)
	sort.SliceStable(top, func(i, j int) bool { return top[i].Pop > top[j].Pop })
	if len(top) > BrowseMin {
		top = top[:BrowseMin]
	}
	cut := now.AddDate(0, 0, -stripFreshDays).Format("2006-01-02")
	today := now.Format("2006-01-02")
	for _, it := range top {
		c.Pop += it.Pop
		if t.Category != "" {
			c.Genres = append(c.Genres, it.Genres...)
		}
		if it.Released != "" && it.Released >= cut && it.Released <= today {
			c.Fresh = true
		}
	}
	return c
}

// StripStats are one category's decayed strip counts for a member.
type StripStats struct{ Shown, Opened float64 }

// StripOpenScore is the smoothed open rate (opened+1)/(shown+5).
func StripOpenScore(s StripStats) float64 { return (s.Opened + 1) / (s.Shown + 5) }

// stripPrior is StripOpenScore with no counts.
const stripPrior = 0.2

// CategoryDay is one row of a member's strip counts.
type CategoryDay struct {
	Category      string
	Day           time.Time
	Shown, Opened int
}

// StripStatsFrom turns the member's rows into decayed stats (the last
// StripCountDays; a skip fades over stripSkipDecay days, an open does
// not) and the categories shown in the last StripRecentDays.
func StripStatsFrom(days []CategoryDay, now time.Time) (map[string]StripStats, map[string]bool) {
	stats, recent := map[string]StripStats{}, map[string]bool{}
	for _, d := range days {
		age := now.Sub(d.Day).Hours() / 24
		if age < 0 {
			age = 0
		}
		if age >= StripCountDays {
			continue
		}
		if d.Shown > 0 && age < StripRecentDays {
			recent[d.Category] = true
		}
		skips := math.Max(0, float64(d.Shown-d.Opened))
		s := stats[d.Category]
		s.Opened += float64(d.Opened)
		s.Shown += float64(d.Opened) + skips*math.Exp(-age/stripSkipDecay)
		stats[d.Category] = s
	}
	return stats, recent
}

// TasteTitle is one title of a member's taste: its genres and TMDB
// keywords, weighed (own asks most, follows, then the household Library).
type TasteTitle struct {
	Genres, Keywords []string
	Weight           float64
}

// StripTaste is a member's genre and keyword weights, each 0-1 (the
// strongest is 1).
type StripTaste struct {
	Genres, Keywords map[string]float64
}

// Empty says there is no taste signal.
func (t StripTaste) Empty() bool { return len(t.Genres) == 0 && len(t.Keywords) == 0 }

// NewStripTaste sums the titles' weights per genre and keyword.
func NewStripTaste(titles []TasteTitle) StripTaste {
	t := StripTaste{Genres: map[string]float64{}, Keywords: map[string]float64{}}
	for _, x := range titles {
		if x.Weight <= 0 {
			continue
		}
		for _, g := range x.Genres {
			t.Genres[g] += x.Weight
		}
		for _, k := range x.Keywords {
			t.Keywords[strings.ToLower(k)] += x.Weight
		}
	}
	norm := func(m map[string]float64) {
		top := 0.0
		for _, w := range m {
			top = math.Max(top, w)
		}
		for k := range m {
			m[k] /= top
		}
	}
	norm(t.Genres)
	norm(t.Keywords)
	return t
}

// GenreMatch is how well the candidate's genres match the taste, 0-1.
func (t StripTaste) GenreMatch(c StripCand) float64 {
	if len(c.Genres) == 0 {
		return 0
	}
	s := 0.0
	for _, g := range c.Genres {
		s += t.Genres[g]
		if alt, ok := browseNames[g]; ok {
			s += t.Genres[alt]
		}
	}
	return clamp(s / float64(len(c.Genres)))
}

// KeywordMatch is the overlap of the category's keywords with the
// member's, 0-1.
func (t StripTaste) KeywordMatch(c StripCand) float64 {
	s := 0.0
	for _, k := range c.Keywords {
		s += t.Keywords[strings.ToLower(k)]
	}
	return clamp(s)
}

// keywordSections are scored by keyword overlap more than by genre.
var keywordSections = map[string]bool{"moods": true, "stories": true, "worlds": true}

// StripIn is everything one strip pick needs.
type StripIn struct {
	Cands []StripCand
	Taste StripTaste
	Stats map[string]StripStats
	// Recent are the categories shown in the last StripRecentDays.
	Recent map[string]bool
	// Rows are the titles of the Explore rows below the strip.
	Rows []string
	// Local is the member's local time; Seed is per member and day.
	Local time.Time
	Seed  string
	Kid   bool
	Max   int
}

// stripRand is a repeatable random from the seed.
func stripRand(seed string) *rand.Rand {
	h := fnv.New64a()
	_, _ = h.Write([]byte(seed))
	v := h.Sum64()
	return rand.New(rand.NewPCG(v, v^0x9e3779b97f4a7c15))
}

// InRows says a row below already names the category (whole words).
func InRows(name string, rows []string) bool {
	n := " " + strings.ToLower(name) + " "
	for _, r := range rows {
		words := strings.FieldsFunc(strings.ToLower(r), func(c rune) bool {
			return !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '&' || c > 127)
		})
		if strings.Contains(" "+strings.Join(words, " ")+" ", n) {
			return true
		}
	}
	return false
}

func has(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// StripBoost is the season, hour and freshness boost of a candidate.
func StripBoost(c StripCand, local time.Time) float64 {
	b := 0.0
	if has(stripSeason[local.Month()], c.Key) {
		b += 1
	}
	h := local.Hour()
	if h >= 18 && has(stripEvening, c.Key) {
		b += 0.4
	}
	if wd := local.Weekday(); (wd == time.Saturday || wd == time.Sunday) && h >= 6 && h < 12 && has(stripWeekendMorning, c.Key) {
		b += 0.5
	}
	if c.Fresh {
		b += 0.3
	}
	return b
}

// StripScore is a candidate's score; pop is its popularity 0-1.
func StripScore(c StripCand, in StripIn, pop float64, cold bool) float64 {
	g, k := in.Taste.GenreMatch(c), in.Taste.KeywordMatch(c)
	taste := 0.6*g + 0.4*k
	if keywordSections[c.Tile.Section] {
		taste = 0.4*g + 0.6*k
	}
	learn := 0.0
	if st, ok := in.Stats[c.Key]; ok {
		learn = math.Max(-1, math.Min(1, (StripOpenScore(st)-stripPrior)/0.3))
	}
	s := 1.2*taste + 0.6*pop + 0.8*learn + StripBoost(c, in.Local)
	if cold {
		defaults := StripDefaults
		if in.Kid {
			defaults = StripKidDefaults
		}
		if has(defaults, c.Key) {
			s += 1
		}
	}
	return s
}

// PickStrip picks and orders the member's strip.
func PickStrip(in StripIn) []CategoryTile {
	max := in.Max
	if max <= 0 {
		max = StripMax
	}
	rnd := stripRand(in.Seed)
	cold := in.Taste.Empty()
	for _, st := range in.Stats {
		if st.Opened > 0 {
			cold = false
		}
	}
	topPop := 0.0
	for _, c := range in.Cands {
		topPop = math.Max(topPop, math.Log1p(c.Pop))
	}
	type cand struct {
		c     StripCand
		score float64
		rows  bool
	}
	var list []cand
	seen := map[string]bool{}
	for _, c := range in.Cands {
		if cat, ok := CategoryByID(c.Key); ok && !cat.InSeason(in.Local) {
			continue
		}
		name := strings.ToLower(c.Tile.Name)
		if seen[name] || seen[c.Key] {
			continue
		}
		seen[name], seen[c.Key] = true, true
		pop := 0.0
		if topPop > 0 {
			pop = math.Log1p(c.Pop) / topPop
		}
		list = append(list, cand{c: c, score: StripScore(c, in, pop, cold) + rnd.Float64()*0.3, rows: InRows(c.Tile.Name, in.Rows)})
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].score > list[j].score })

	taken := map[string]bool{}
	perSection, perKin := map[string]int{}, map[string]bool{}
	fits := func(c StripCand, sectionCap int) bool {
		return !taken[c.Key] && perSection[c.Tile.Section] < sectionCap && !perKin[StripKin(c.Tile.Name)]
	}
	take := func(c StripCand) {
		taken[c.Key] = true
		perSection[c.Tile.Section]++
		perKin[StripKin(c.Tile.Name)] = true
	}
	nExplore := int(math.Round(float64(max) * StripExploreShare))
	var main, explore []StripCand
	// The best scored, within the limits, leaving room for the explore slot.
	for _, x := range list {
		if len(main) >= max-nExplore {
			break
		}
		if !x.rows && fits(x.c, stripSectionCap) {
			take(x.c)
			main = append(main, x.c)
		}
	}
	// The explore slot: a seeded draw from what the member was not shown lately.
	var pool []StripCand
	for _, x := range list {
		if !x.rows && !in.Recent[x.c.Key] && !taken[x.c.Key] {
			pool = append(pool, x.c)
		}
	}
	rnd.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	for _, c := range pool {
		if len(explore) >= nExplore {
			break
		}
		if fits(c, stripSectionCap) {
			take(c)
			explore = append(explore, c)
		}
	}
	// Short: fill by score, then let a section go over its cap, then the
	// categories the rows below already show.
	fill := func(sectionCap int, rows bool) {
		for _, x := range list {
			if len(main)+len(explore) >= max {
				return
			}
			if x.rows == rows && fits(x.c, sectionCap) {
				take(x.c)
				main = append(main, x.c)
			}
		}
	}
	fill(stripSectionCap, false)
	fill(max, false)
	fill(max, true)
	// The explore tiles sit among the others (every fifth place from the third).
	out := make([]CategoryTile, 0, max)
	ei := 0
	for mi := 0; mi < len(main) || ei < len(explore); {
		if ei < len(explore) && (len(out)%5 == 2 || mi >= len(main)) {
			out = append(out, explore[ei].Tile)
			ei++
			continue
		}
		out = append(out, main[mi].Tile)
		mi++
	}
	return out
}
