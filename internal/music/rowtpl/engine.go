// Package rowtpl is the music row template engine (flag music_lists_v1,
// plan 2026-10-24-music-row-templates): a catalog of row templates filled
// from the server's music catalog bundle, the region's list blocks and the
// install's own library, ranked and deduped by the v7 row plan.
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

// Listen rank ranges (the bundle's R array).
const (
	rWeek = iota
	rMonth
	rThisYear
	rThisWeek
	rAllTime
)

// Item is one bundle item: an album ("a") or an artist ("r").
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

// Member is an item in a key: index, fit ×1000, tagged with the key.
type Member struct {
	I, T int
	Own  bool
}

// UnmarshalJSON reads [i, t, own].
func (m *Member) UnmarshalJSON(b []byte) error {
	var a [3]int
	if err := json.Unmarshal(b, &a); err != nil {
		return err
	}
	m.I, m.T, m.Own = a[0], a[1], a[2] == 1
	return nil
}

// KeyInfo names a key.
type KeyInfo struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Parent string `json:"parent"`
}

// Bundle is the server's music catalog bundle.
type Bundle struct {
	Version string              `json:"version"`
	Built   string              `json:"built"`
	Items   []Item              `json:"items"`
	Keys    map[string][]Member `json:"keys"`
	Names   map[string]KeyInfo  `json:"names"`

	byKey    map[string]map[int]Member
	artistIx map[string]int
	albumIx  map[string]int
	maxL     float64
}

// ParseBundle reads a bundle and indexes it.
func ParseBundle(b []byte) (*Bundle, error) {
	var x Bundle
	if err := json.Unmarshal(b, &x); err != nil {
		return nil, err
	}
	x.index()
	return &x, nil
}

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

// Lib is what the install knows about the member (never sent anywhere).
type Lib struct {
	Albums  map[string]bool    // release groups had
	Asked   map[string]bool    // release groups asked for
	Artists map[string]float64 // artist MBID → affinity A
	Sim     map[string]float64 // artist MBID → similarity to the library
	Genres  []string           // library genre keys, strongest first
	Decade  string             // decade key the library has most of
	Seeds   []Seed             // library artists that seed rows
}

// Seed is a library artist a row is about.
type Seed struct {
	ID, Name string
	Similar  map[string]float64 // artist MBID → S
}

func (l *Lib) empty() bool { return l == nil || len(l.Artists) == 0 && len(l.Albums) == 0 }

// Ctx is one page build.
type Ctx struct {
	B        *Bundle
	Kind     string // genre, mood, season, decade, activity, explore, new
	Key      string // page key ("genre:hip-hop"), empty on explore and new
	Region   string // "us", or "global"
	Now      time.Time
	Member   int64
	Lib      *Lib
	Blocks   map[string][]string // list blocks: name → item refs ("a:<rg>", "r:<artist>")
	Neighbor string
	// CountryName answers "Brazil" for "br" (empty: unknown).
	CountryName func(code string) string
	// Seen holds item keys already on the page (the Spotlight).
	Seen map[string]bool
}

// Day is the build's UTC day.
func (c *Ctx) Day() string { return c.Now.UTC().Format("2006-01-02") }

func (c *Ctx) week() string {
	y, w := c.Now.UTC().ISOWeek()
	return strconv.Itoa(y) + "-" + strconv.Itoa(w)
}

func (c *Ctx) it(i int) *Item { return &c.B.Items[i] }

func (c *Ctx) region() string {
	if c.Region == "" || c.Region == "global" {
		return "us"
	}
	return c.Region
}

// ---- signals (all 0..1) ----

// pc is the item's popularity on one country's chart.
func (c *Ctx) pc(i int, country string) float64 {
	if r, ok := c.it(i).C[country]; ok {
		return float64(101-r) / 100
	}
	return 0
}

// p is chart popularity in the member's region, else 0.7 × the best.
func (c *Ctx) p(i int) float64 {
	if v := c.pc(i, c.region()); v > 0 {
		return v
	}
	return 0.7 * float64(c.it(i).P) / 1000
}

func (c *Ctx) m(i int) float64 { return float64(c.it(i).M) / 1000 }

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

// f is freshness exp(−days/t); future or unknown dates give 0.
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

// lp is log(listens)/log(most listens).
func (c *Ctx) lp(i int) float64 {
	if c.B.maxL <= 1 {
		return 0
	}
	return math.Log1p(float64(c.it(i).Listens)) / math.Log1p(c.B.maxL)
}

func rank01(r int) float64 {
	if r <= 0 {
		return 0
	}
	return 1 - float64(r-1)/1000
}

func (c *Ctx) wp(i int) float64 { return rank01(c.it(i).R[rWeek]) }
func (c *Ctx) mp(i int) float64 { return rank01(c.it(i).R[rMonth]) }
func (c *Ctx) yp(i int) float64 { return rank01(c.it(i).R[rThisYear]) }

// t is the item's fit to a key.
func (c *Ctx) t(i int, key string) float64 {
	if m, ok := c.B.byKey[key][i]; ok {
		return float64(m.T) / 1000
	}
	return 0
}

func (c *Ctx) artistOf(i int) string {
	it := c.it(i)
	if it.Kind == "r" {
		return it.ID
	}
	return it.ArtistID
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

// l is library match: max(A, 0.6·S).
func (c *Ctx) l(i int) float64 {
	if c.Lib == nil {
		return 0
	}
	a := c.artistOf(i)
	return math.Max(c.Lib.Artists[a], 0.6*c.Lib.Sim[a])
}

// n is novelty: 1 new to the member, 0.5 artist known, 0 had.
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

// ---- lists ----

// L is a list of item indices in a row's order.
type L []int

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

// by sorts a copy of l by f, best first (ties: listens, then id).
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

// block answers a list block as bundle items (refs not in the bundle are
// left out: they have no cover yet).
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

// charting answers items on a country's chart, best rank first.
func (c *Ctx) charting(l L, country string) L {
	l = c.where(l, func(i int) bool { _, ok := c.it(i).C[country]; return ok })
	return c.by(l, func(i int) float64 { return c.pc(i, country) })
}

// pct answers the items of l whose listens lie between the lo and hi
// fractions of l's own listen order (0 = least played).
func (c *Ctx) pct(l L, lo, hi float64) L {
	s := c.by(l, func(i int) float64 { return float64(c.it(i).Listens) })
	n := len(s)
	// s is most-played first: fraction f from the bottom is index n−1−f·n.
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

func hash(parts ...string) uint64 {
	h := fnv.New64a()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return h.Sum64()
}
