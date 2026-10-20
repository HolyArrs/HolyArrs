package explore

import (
	"math"
	"sort"
	"strings"
	"time"
)

// LibTitle is one movie or show in the Library, as a seed source.
type LibTitle struct {
	TMDBID, Kind, Name string
	Year               int
	Genres, People     []string
	Rating             string
	Added              time.Time
}

// AskSeed is one person on one ask. Kid says the person is a Kid.
type AskSeed struct {
	TMDBID, Kind, Name string
	Year               int
	Genres             []string
	Member             int64
	Kid                bool
	At                 time.Time
}

// Seed is a title with a weight for one member.
type Seed struct {
	TMDBID, Kind, Name string
	Year               int
	Genres, People     []string
	Weight             float64
	// Recent says it was added or asked within 365 days, so it may name a
	// Because row.
	Recent bool
}

// Key is kind:tmdb id.
func (s Seed) Key() string { return s.Kind + ":" + s.TMDBID }

func decay(now, at time.Time) float64 {
	days := now.Sub(at).Hours() / 24
	if days < 0 {
		days = 0
	}
	return math.Exp(-days / 120)
}

// Seeds weighs the member's signals (5.1): own ask 2.0, another member's
// 0.5; Library 1.0 when added within 365 days, else 0.5; each times
// exp(-days/120). A Kid's seeds are their own asks only, and a Kid's asks
// never seed an adult. lib is passed already filtered for a Kid's limit.
// Strongest first.
func Seeds(member int64, kid bool, lib []LibTitle, asks []AskSeed, now time.Time) []Seed {
	by := map[string]*Seed{}
	add := func(s Seed, w float64) {
		if s.TMDBID == "" || (s.Kind != "movie" && s.Kind != "show") || w == 0 {
			return
		}
		cur, ok := by[s.Key()]
		if !ok {
			s.Weight = 0
			cur = &s
			by[s.Key()] = cur
		}
		cur.Weight += w
		cur.Recent = cur.Recent || s.Recent
		if len(cur.Genres) == 0 {
			cur.Genres = s.Genres
		}
		if len(cur.People) == 0 {
			cur.People = s.People
		}
		if cur.Year == 0 {
			cur.Year = s.Year
		}
	}
	for _, t := range lib {
		w := 0.5
		if now.Sub(t.Added) <= 365*24*time.Hour {
			w = 1.0
		}
		add(Seed{TMDBID: t.TMDBID, Kind: t.Kind, Name: t.Name, Year: t.Year, Genres: t.Genres, People: t.People, Recent: w == 1.0}, w*decay(now, t.Added))
	}
	for _, a := range asks {
		w := 0.5
		switch {
		case a.Member == member:
			w = 2.0
		case kid || a.Kid:
			continue
		}
		add(Seed{TMDBID: a.TMDBID, Kind: a.Kind, Name: a.Name, Year: a.Year, Genres: a.Genres, Recent: now.Sub(a.At) <= 365*24*time.Hour}, w*decay(now, a.At))
	}
	out := make([]Seed, 0, len(by))
	for _, s := range by {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Weight != out[j].Weight {
			return out[i].Weight > out[j].Weight
		}
		return out[i].Key() < out[j].Key()
	})
	return out
}

// SeedCount counts the seeds that add signal (positive weight).
func SeedCount(seeds []Seed) int {
	n := 0
	for _, s := range seeds {
		if s.Weight > 0 {
			n++
		}
	}
	return n
}

// TailCap is the most tail rows a page gets (5.4): under 3 seeds 8, 3 to 9
// seeds 20, 10 or more 40. A Kid's tail stops at 8 (a short page).
func TailCap(seeds int, kid bool) int {
	n := 8
	switch {
	case seeds >= 10:
		n = 40
	case seeds >= 3:
		n = 20
	}
	if kid && n > 8 {
		n = 8
	}
	return n
}

// Profile is a member's taste: a genre vector, a set of people and the
// median seed year.
type Profile struct {
	genres map[string]float64
	people map[string]bool
	year   int
}

// NewProfile builds the profile from seeds. Negative seeds take their
// genre weights away at half strength.
func NewProfile(seeds []Seed) Profile {
	p := Profile{genres: map[string]float64{}, people: map[string]bool{}}
	var years []int
	for _, s := range seeds {
		w := s.Weight
		if w < 0 {
			w /= 2
		}
		for _, g := range s.Genres {
			p.genres[g] += w
		}
		if s.Weight > 0 {
			for _, n := range s.People {
				p.people[strings.ToLower(n)] = true
			}
			if s.Year > 0 {
				years = append(years, s.Year)
			}
		}
	}
	if len(years) > 0 {
		sort.Ints(years)
		p.year = years[len(years)/2]
	}
	return p
}

// Empty says the profile has no signal.
func (p Profile) Empty() bool { return len(p.genres) == 0 && len(p.people) == 0 && p.year == 0 }

// TopGenres are the member's strongest genres with weight share >= min.
func (p Profile) TopGenres(n int, minShare float64) []string {
	total := 0.0
	for _, w := range p.genres {
		if w > 0 {
			total += w
		}
	}
	type gw struct {
		g string
		w float64
	}
	var list []gw
	for g, w := range p.genres {
		if w > 0 && total > 0 && w/total >= minShare {
			list = append(list, gw{g, w})
		}
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].w != list[j].w {
			return list[i].w > list[j].w
		}
		return list[i].g < list[j].g
	})
	out := []string{}
	for i := 0; i < len(list) && i < n; i++ {
		out = append(out, list[i].g)
	}
	return out
}

// Affinity is 0.5·genreMatch + 0.3·peopleMatch + 0.2·eraMatch (5.3).
func (p Profile) Affinity(it Item) float64 {
	// genreMatch: cosine of the item's genres (each 1) with the vector.
	var dot, norm float64
	for _, w := range p.genres {
		norm += w * w
	}
	for _, g := range it.Genres {
		dot += p.genres[g]
	}
	genre := 0.0
	if norm > 0 && len(it.Genres) > 0 {
		genre = dot / (math.Sqrt(norm) * math.Sqrt(float64(len(it.Genres))))
	}
	genre = clamp(genre)
	people := 0.0
	for _, n := range it.People {
		if p.people[strings.ToLower(n)] {
			people = 1
			break
		}
	}
	era := 0.0
	if p.year > 0 && it.Year > 0 {
		era = clamp(1 - math.Abs(float64(it.Year-p.year))/40)
	}
	return 0.5*genre + 0.3*people + 0.2*era
}

func clamp(x float64) float64 { return math.Max(0, math.Min(1, x)) }
