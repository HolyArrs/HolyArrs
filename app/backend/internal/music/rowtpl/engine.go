// Package rowtpl builds the music rows and Browse tiles. It fills a catalog of
// row templates from the server's catalog bundle, the region's list blocks and
// the install's own library, then ranks and dedupes the rows into pages.
package rowtpl

import (
	"encoding/json"
	"hash/fnv"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Positions in an item's listen rank array (Item.R).
const (
	rWeek = iota
	rMonth
	rThisYear
	rThisWeek
	rAllTime
)

// MinPictured is the fewest pictured items a row needs, because the first screen of a row must show pictures and only its tail may show initials.
const MinPictured = 5

// Item is one bundle entry: an album (kind "a") or an artist (kind "r").
type Item struct {
	ID       string         `json:"id"`
	Kind     string         `json:"k"`
	Title    string         `json:"t"`
	Artist   string         `json:"ar"`
	ArtistID string         `json:"ai"`
	Date     string         `json:"d"`
	Type     string         `json:"ty"`
	Listens  int64          `json:"l"`
	Users    int64          `json:"u"`
	R        [5]int         `json:"r"`
	C        map[string]int `json:"c"`
	CW       int            `json:"cw"`
	P        int            `json:"p"`
	M        int            `json:"m"`
	G        map[string]int `json:"g"`
	Debut    bool           `json:"deb"`
}

// Member places one item in a key, with its fit to that key times 1000.
type Member struct {
	I, T int
	Own  bool
}

// KeyInfo gives a bundle key its display name, kind and parent key.
type KeyInfo struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Parent string `json:"parent"`
}

// Bundle is the server's music catalog bundle with lookup indexes built on load.
type Bundle struct {
	Version string              `json:"version"`
	Built   string              `json:"built"`
	Items   []Item              `json:"items"`
	Keys    map[string][]Member `json:"keys"`
	Names   map[string]KeyInfo  `json:"names"`

	byKey map[string]map[int]Member
	// mainT is the best genre fit per item, times 1000.
	mainT    map[int]int
	artistIx map[string]int
	albumIx  map[string]int
	maxL     float64
}

// Lib is what the install knows about the member's library. It never leaves the server.
type Lib struct {
	// Albums and Asked are release groups the member has and has asked for.
	Albums map[string]bool
	Asked  map[string]bool
	// Artists maps an artist MBID to the member's affinity for it.
	Artists map[string]float64
	// Sim maps an artist MBID to its similarity to the library.
	Sim map[string]float64
	// Genres are the library's genre keys, strongest first.
	Genres []string
	// Decade is the decade key the library has most of.
	Decade string
	// Seeds are the library artists that seed rows.
	Seeds []Seed
}

// Seed is a library artist that a row is about.
type Seed struct {
	ID, Name string
	// Similar maps an artist MBID to its similarity to this seed.
	Similar map[string]float64
}

// Ctx holds everything one page build needs.
type Ctx struct {
	B *Bundle
	// Kind is genre, mood, season, decade, activity, explore or new.
	Kind string
	// Key is the page key such as "genre:hip-hop". It is empty on explore and new.
	Key    string
	Region string
	Now    time.Time
	Member int64
	Lib    *Lib
	// Blocks maps a list block name to item refs ("a:<rg>" or "r:<artist>").
	Blocks   map[string][]string
	Neighbor string
	// CountryName answers a display name for a country code, or empty if unknown.
	CountryName func(code string) string
	// Seen holds the item keys already on the page.
	Seen map[string]bool
	// HasArt reports whether an item's picture is stored. Nil means all are.
	HasArt func(i int) bool
}

// L is a list of item indices in a row's order.
type L []int

// ParseBundle reads a bundle and builds its indexes.
func ParseBundle(b []byte) (*Bundle, error) {
	var x Bundle
	if err := json.Unmarshal(b, &x); err != nil {
		return nil, err
	}
	x.index()
	return &x, nil
}

// UnmarshalJSON reads the compact [index, fit, own] form the bundle uses.
func (m *Member) UnmarshalJSON(b []byte) error {
	var a [3]int
	if err := json.Unmarshal(b, &a); err != nil {
		return err
	}
	m.I, m.T, m.Own = a[0], a[1], a[2] == 1
	return nil
}

// Day is the build's UTC day as YYYY-MM-DD.
func (c *Ctx) Day() string { return c.Now.UTC().Format("2006-01-02") }

// Lookup answers a catalog album or artist by its MusicBrainz id.
func (b *Bundle) Lookup(kind, id string) (Item, bool) {
	ix := b.albumIx
	if kind == "artist" {
		ix = b.artistIx
	}
	if i, ok := ix[id]; ok {
		return b.Items[i], true
	}
	return Item{}, false
}

// index builds the per-key, best-genre-fit and id lookups after a load.
func (b *Bundle) index() {
	b.byKey = map[string]map[int]Member{}
	for k, l := range b.Keys {
		m := make(map[int]Member, len(l))
		for _, x := range l {
			if x.I >= 0 && x.I < len(b.Items) {
				m[x.I] = x
			}
		}
		b.byKey[k] = m
	}
	b.mainT = map[int]int{}
	for k, l := range b.Keys {
		if strings.HasPrefix(k, "genre:") {
			for _, x := range l {
				if x.T > b.mainT[x.I] {
					b.mainT[x.I] = x.T
				}
			}
		}
	}
	b.artistIx, b.albumIx = map[string]int{}, map[string]int{}
	for i, it := range b.Items {
		if it.Kind == "r" {
			b.artistIx[it.ID] = i
		} else {
			b.albumIx[it.ID] = i
		}
		b.maxL = math.Max(b.maxL, float64(it.Listens))
	}
}

// empty reports a missing library or one with no artists and no albums.
func (l *Lib) empty() bool { return l == nil || len(l.Artists) == 0 && len(l.Albums) == 0 }

// week is the build's ISO year and week, used to rotate rows weekly.
func (c *Ctx) week() string {
	y, w := c.Now.UTC().ISOWeek()
	return strconv.Itoa(y) + "-" + strconv.Itoa(w)
}

func (c *Ctx) it(i int) *Item { return &c.B.Items[i] }

// pictured keeps the items with a stored picture, or all when HasArt is nil.
func (c *Ctx) pictured(l L) L {
	if c.HasArt == nil {
		return l
	}
	return c.where(l, c.HasArt)
}

// picturedFirst puts items with a stored picture first, each part keeping its order. It also reports whether the row has MinPictured of them.
func (c *Ctx) picturedFirst(l L) (L, bool) {
	if c.HasArt == nil {
		return l, true
	}
	with, without := make(L, 0, len(l)), L(nil)
	for _, i := range l {
		if c.HasArt(i) {
			with = append(with, i)
		} else {
			without = append(without, i)
		}
	}
	return append(with, without...), len(with) >= MinPictured
}

// region maps an empty or global region to "us", the chart we fall back on.
func (c *Ctx) region() string {
	if c.Region == "" || c.Region == "global" {
		return "us"
	}
	return c.Region
}

// pc is the item's popularity on one country's chart, from 0 to 1.
func (c *Ctx) pc(i int, country string) float64 {
	if r, ok := c.it(i).C[country]; ok {
		return float64(101-r) / 100
	}
	return 0
}

// p is chart popularity in the member's region. Without a regional rank it is 0.7 times the best rank anywhere, so foreign hits count for less.
func (c *Ctx) p(i int) float64 {
	if v := c.pc(i, c.region()); v > 0 {
		return v
	}
	return 0.7 * float64(c.it(i).P) / 1000
}

func (c *Ctx) m(i int) float64 { return float64(c.it(i).M) / 1000 }

// days is the item's age in days. A year-only date counts as July 7 and a year-month date as the 15th, so partial dates land mid-period.
func (c *Ctx) days(i int) (float64, bool) {
	d := c.it(i).Date
	if len(d) < 4 {
		return 0, false
	}
	for len(d) < 10 {
		if len(d) == 4 {
			d += "-07"
		} else {
			d += "-15"
		}
	}
	t, err := time.Parse("2006-01-02", d)
	if err != nil {
		return 0, false
	}
	return c.Now.Sub(t).Hours() / 24, true
}

// f is freshness, exp(-days/t). Future and unknown dates give 0.
func (c *Ctx) f(i int, t float64) float64 {
	d, ok := c.days(i)
	if !ok || d < 0 {
		return 0
	}
	return math.Exp(-d / t)
}

func (c *Ctx) year(i int) int {
	if d := c.it(i).Date; len(d) >= 4 {
		y, _ := strconv.Atoi(d[:4])
		return y
	}
	return 0
}

func (c *Ctx) age(i int) float64 {
	d, ok := c.days(i)
	if !ok {
		return -1
	}
	return d / 365.25
}

// lp is log listens over log of the most listens in the bundle.
func (c *Ctx) lp(i int) float64 {
	if c.B.maxL <= 1 {
		return 0
	}
	return math.Log1p(float64(c.it(i).Listens)) / math.Log1p(c.B.maxL)
}

// rank01 turns a 1-based rank among 1000 into a score from 0 to 1. No rank gives 0.
func rank01(r int) float64 {
	if r <= 0 {
		return 0
	}
	return 1 - float64(r-1)/1000
}

func (c *Ctx) wp(i int) float64 { return rank01(c.it(i).R[rWeek]) }

func (c *Ctx) mp(i int) float64 { return rank01(c.it(i).R[rMonth]) }

func (c *Ctx) yp(i int) float64 { return rank01(c.it(i).R[rThisYear]) }

// mainGenreT is the item's best fit to any genre key.
func (c *Ctx) mainGenreT(i int) float64 { return float64(c.B.mainT[i]) / 1000 }

// t is the item's fit to one key, from 0 to 1.
func (c *Ctx) t(i int, key string) float64 {
	if m, ok := c.B.byKey[key][i]; ok {
		return float64(m.T) / 1000
	}
	return 0
}

// artistOf answers the artist id of an item. Without an id the name stands in, so the per-artist cap still holds.
func (c *Ctx) artistOf(i int) string {
	it := c.it(i)
	if it.Kind == "r" {
		return it.ID
	}
	if it.ArtistID != "" {
		return it.ArtistID
	}
	return "name:" + strings.ToLower(strings.TrimSpace(it.Artist))
}

// had reports an album the member has or asked for, or an artist they have.
func (c *Ctx) had(i int) bool {
	if c.Lib == nil {
		return false
	}
	it := c.it(i)
	if it.Kind == "r" {
		_, ok := c.Lib.Artists[it.ID]
		return ok
	}
	return c.Lib.Albums[it.ID] || c.Lib.Asked[it.ID]
}

// l is library match: the larger of affinity and 0.6 times similarity.
func (c *Ctx) l(i int) float64 {
	if c.Lib == nil {
		return 0
	}
	a := c.artistOf(i)
	return math.Max(c.Lib.Artists[a], 0.6*c.Lib.Sim[a])
}

// n is novelty: 1 when new to the member, 0.5 when only the artist is known, 0 when had.
func (c *Ctx) n(i int) float64 {
	if c.had(i) {
		return 0
	}
	if c.Lib != nil {
		if _, ok := c.Lib.Artists[c.artistOf(i)]; ok {
			return 0.5
		}
	}
	return 1
}

func (c *Ctx) key(k string) L {
	ms := c.B.Keys[k]
	out := make(L, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.I)
	}
	return out
}

func (c *Ctx) all() L {
	out := make(L, len(c.B.Items))
	for i := range out {
		out[i] = i
	}
	return out
}

func (c *Ctx) where(l L, f func(i int) bool) L {
	var out L
	for _, i := range l {
		if f(i) {
			out = append(out, i)
		}
	}
	return out
}

func (c *Ctx) albums(l L) L {
	return c.where(l, func(i int) bool { return c.it(i).Kind == "a" })
}

func (c *Ctx) artists(l L) L {
	return c.where(l, func(i int) bool { return c.it(i).Kind == "r" })
}

func (c *Ctx) in(l L, key string) L {
	m := c.B.byKey[key]
	return c.where(l, func(i int) bool { _, ok := m[i]; return ok })
}

// by sorts a copy of l by f, best first. Ties go to more listens, then to the lower id, so the order is stable.
func (c *Ctx) by(l L, f func(i int) float64) L {
	out := append(L{}, l...)
	s := make(map[int]float64, len(out))
	for _, i := range out {
		s[i] = f(i)
	}
	sort.SliceStable(out, func(a, b int) bool {
		x, y := out[a], out[b]
		if s[x] != s[y] {
			return s[x] > s[y]
		}
		if c.it(x).Listens != c.it(y).Listens {
			return c.it(x).Listens > c.it(y).Listens
		}
		return c.it(x).ID < c.it(y).ID
	})
	return out
}

// block answers a list block as bundle items. Refs missing from the bundle are left out because they have no cover yet.
func (c *Ctx) block(name string) L {
	var out L
	for _, ref := range c.Blocks[name] {
		k, id, ok := strings.Cut(ref, ":")
		if !ok {
			continue
		}
		if k == "r" {
			if i, ok := c.B.artistIx[id]; ok {
				out = append(out, i)
			}
			continue
		}
		if i, ok := c.B.albumIx[id]; ok {
			out = append(out, i)
		}
	}
	return out
}

// charting answers the items on a country's chart, best rank first.
func (c *Ctx) charting(l L, country string) L {
	l = c.where(l, func(i int) bool { _, ok := c.it(i).C[country]; return ok })
	return c.by(l, func(i int) float64 { return c.pc(i, country) })
}

// pct answers the items of l whose listens lie between the lo and hi fractions of l's own listen order, where 0 is least played.
func (c *Ctx) pct(l L, lo, hi float64) L {
	s := c.by(l, func(i int) float64 { return float64(c.it(i).Listens) })
	n := len(s)
	a, b := int(float64(n)*(1-hi)), int(float64(n)*(1-lo))
	if a < 0 {
		a = 0
	}
	if b > n {
		b = n
	}
	if a >= b {
		return nil
	}
	return append(L{}, s[a:b]...)
}

// loyal keeps items whose listens per listener reach the median of l.
func (c *Ctx) loyal(l L) L {
	var r []float64
	for _, i := range l {
		if u := c.it(i).Users; u > 0 {
			r = append(r, float64(c.it(i).Listens)/float64(u))
		}
	}
	if len(r) == 0 {
		return nil
	}
	sort.Float64s(r)
	med := r[len(r)/2]
	return c.where(l, func(i int) bool {
		u := c.it(i).Users
		return u > 0 && float64(c.it(i).Listens)/float64(u) >= med
	})
}

func (c *Ctx) notHad(l L) L { return c.where(l, func(i int) bool { return !c.had(i) }) }

// hash makes a stable number from parts. The zero byte keeps neighbours from running together.
func hash(parts ...string) uint64 {
	h := fnv.New64a()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return h.Sum64()
}
