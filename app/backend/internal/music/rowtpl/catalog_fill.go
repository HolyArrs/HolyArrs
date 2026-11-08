package rowtpl

import (
	"math"
	"strconv"
)

// Tpl is one row template. Pages holds the page letters it runs on: E Explore, N New and Popular, G genre, M mood, S season, D decade, A activity. Slot names the value the template expands over, one row per value and at most Cap. Cross maps a page letter to the key kind a "cross" slot takes on that page. Need names data the row needs: "ls" for list blocks, "lib" for a library, "seed" for library seeds, "season" for a page season that is on, and "v7" or "cal" for rows the Explore builder supplies. Src, Sort and Filter say in words what Fill does.
type Tpl struct {
	ID, Fam           string
	Disc              bool
	Prior             float64
	Pages             string
	Title             string
	Vars              []string
	Reason            string
	Src, Sort, Filter string
	Ref               string
	Shape             string
	Slot              string
	Cap               int
	Cross             map[byte]string
	Chart             bool
	// Keep lets a chart row keep items the member has, shown with the Have chip.
	Keep bool
	Need string
	// Only limits the row to these page keys.
	Only map[string]bool
	// NotWith lists cross values the row never takes.
	NotWith map[string]bool
	Fill    func(c *Ctx, v string) L
}

// t builds a template with one row and no slot.
func t(id, fam string, prior float64, pages, title, reason, src, sort, filter, ref string) Tpl {
	return Tpl{ID: id, Fam: fam, Prior: prior, Pages: pages, Title: title, Reason: reason, Src: src, Sort: sort, Filter: filter, Ref: ref, Cap: 1}
}

// fill sets how the template picks its items.
func (x Tpl) fill(f func(c *Ctx, v string) L) Tpl { x.Fill = f; return x }

// slot makes the template expand over a slot, one row per value up to cap.
func (x Tpl) slot(s string, cap int) Tpl { x.Slot, x.Cap = s, cap; return x }

// cross makes the template expand over keys of another kind, by page letter.
func (x Tpl) cross(m map[byte]string, cap int) Tpl {
	x.Slot, x.Cross, x.Cap = "cross", m, cap
	return x
}

// round gives the template round covers, for artists.
func (x Tpl) round() Tpl { x.Shape = "round"; return x }

// wide gives the template wide covers.
func (x Tpl) wide() Tpl { x.Shape = "wide"; return x }

// chart marks a chart row. It keeps items the member has, shown with the Have chip.
func (x Tpl) chart() Tpl { x.Chart, x.Keep = true, true; return x }

// disc marks a discovery row.
func (x Tpl) disc() Tpl { x.Disc = true; return x }

// need names the data the row needs.
func (x Tpl) need(n string) Tpl { x.Need = n; return x }

// vars adds alternate title wordings.
func (x Tpl) vars(v ...string) Tpl { x.Vars = v; return x }

// notWith lists cross values the row never takes.
func (x Tpl) notWith(keys ...string) Tpl {
	x.NotWith = map[string]bool{}
	for _, k := range keys {
		x.NotWith[k] = true
	}
	return x
}

// only limits the row to some page keys.
func (x Tpl) only(keys ...string) Tpl {
	x.Only = map[string]bool{}
	for _, k := range keys {
		x.Only[k] = true
	}
	return x
}

// pool answers the page key's items. A genre page keeps only items whose main genre it is, or nearly, so an album tagged k-pop first and rap second stays on the K-pop page.
func pool(c *Ctx) L {
	l := c.key(c.Key)
	if kindOf(c.Key) == "genre" {
		l = c.where(l, func(i int) bool { return c.t(i, c.Key) >= 0.9*c.mainGenreT(i) })
	}
	return l
}

// palb answers the page pool's albums.
func palb(c *Ctx) L { return c.albums(pool(c)) }

// part answers the page pool's artists.
func part(c *Ctx) L { return c.artists(pool(c)) }

// kalb answers the albums of one key.
func kalb(c *Ctx, k string) L { return c.albums(c.key(k)) }

// newer keeps items released within the last days.
func (c *Ctx) newer(l L, days float64) L {
	return c.where(l, func(i int) bool { d, ok := c.days(i); return ok && d >= 0 && d <= days })
}

// older keeps items at least years old.
func (c *Ctx) older(l L, years float64) L {
	return c.where(l, func(i int) bool { return c.age(i) >= years })
}

// ageIn keeps items from lo up to but not including hi years old.
func (c *Ctx) ageIn(l L, lo, hi float64) L {
	return c.where(l, func(i int) bool { a := c.age(i); return a >= lo && a < hi })
}

// inYear keeps items released in year y.
func (c *Ctx) inYear(l L, y int) L { return c.where(l, func(i int) bool { return c.year(i) == y }) }

// ranked keeps items with a listen rank in range r.
func (c *Ctx) ranked(l L, r int) L {
	return c.where(l, func(i int) bool { return c.it(i).R[r] > 0 })
}

// onAnyChart keeps items on at least one country's chart.
func (c *Ctx) onAnyChart(l L) L { return c.where(l, func(i int) bool { return len(c.it(i).C) > 0 }) }

// typ keeps items of one release type, such as EP or Soundtrack.
func (c *Ctx) typ(l L, ty string) L {
	return c.where(l, func(i int) bool { return c.it(i).Type == ty })
}

// debuts keeps albums flagged as an artist's first.
func (c *Ctx) debuts(l L) L { return c.where(l, func(i int) bool { return c.it(i).Debut }) }

// byLP sorts by listens.
func (c *Ctx) byLP(l L) L { return c.by(l, c.lp) }

// byLPT sorts by listens, weighted by fit to key k.
func (c *Ctx) byLPT(l L, k string) L {
	return c.by(l, func(i int) float64 { return c.lp(i) * (0.5 + 0.5*c.t(i, k)) })
}

// byP sorts by chart popularity.
func (c *Ctx) byP(l L) L { return c.by(l, c.p) }

// cw ranks items on the many-country score, best first.
func (c *Ctx) cw(l L) L {
	l = c.where(l, func(i int) bool { return c.it(i).CW > 0 })
	return c.by(l, func(i int) float64 { return 1 / float64(c.it(i).CW) })
}

// gain is an item's 7-day rank gain on one country's chart.
func (c *Ctx) gain(i int, country string) float64 {
	return float64(c.it(i).G[country]) / 1000
}

// areaScore sums an item's chart popularity over an area's countries.
func (c *Ctx) areaScore(i int, area string) float64 {
	s := 0.0
	for _, cc := range areas[area] {
		s += c.pc(i, cc)
	}
	return s
}

// numberOnes counts the charts where the item is number one.
func (c *Ctx) numberOnes(i int) int {
	n := 0
	for _, r := range c.it(i).C {
		if r == 1 {
			n++
		}
	}
	return n
}

// blockIn answers a list block's items, kept to the members of l when l is not nil.
func (c *Ctx) blockIn(name string, l L) L {
	b := c.block(name)
	if l == nil {
		return b
	}
	in := map[int]bool{}
	for _, i := range l {
		in[i] = true
	}
	return c.where(b, func(i int) bool { return in[i] })
}

// union keeps the order of a, then adds the items of b not in a.
func union(a, b L) L {
	in := map[int]bool{}
	out := append(L{}, a...)
	for _, i := range a {
		in[i] = true
	}
	for _, i := range b {
		if !in[i] {
			in[i] = true
			out = append(out, i)
		}
	}
	return out
}

// libArtist reports an artist the member has.
func (c *Ctx) libArtist(i int) bool {
	if c.Lib == nil {
		return false
	}
	_, ok := c.Lib.Artists[c.artistOf(i)]
	return ok
}

// simArtist is the item's artist similarity to the library.
func (c *Ctx) simArtist(i int) float64 {
	if c.Lib == nil {
		return 0
	}
	return c.Lib.Sim[c.artistOf(i)]
}

// topArtists keeps albums whose artist is in the top frac of l's artists by listens.
func (c *Ctx) topArtists(l L, frac float64) L {
	sum := map[string]int64{}
	for _, i := range l {
		sum[c.artistOf(i)] += c.it(i).Listens
	}
	var v []int64
	for _, s := range sum {
		v = append(v, s)
	}
	if len(v) == 0 {
		return nil
	}
	sortInt64(v)
	cut := v[int(float64(len(v)-1)*(1-frac))]
	return c.where(l, func(i int) bool { return sum[c.artistOf(i)] >= cut && cut > 0 })
}

// artistYears answers, per artist, the release years of its bundle albums.
func (c *Ctx) artistYears() map[string][]int {
	out := map[string][]int{}
	for i, it := range c.B.Items {
		if it.Kind == "a" {
			if y := c.year(i); y > 0 {
				out[it.ArtistID] = append(out[it.ArtistID], y)
			}
		}
	}
	return out
}

// artistTopYear answers, per artist, the year of its most-listened album.
func (c *Ctx) artistTopYear() map[string]int {
	best := map[string]int64{}
	out := map[string]int{}
	for i, it := range c.B.Items {
		if it.Kind == "a" && it.Listens > best[it.ArtistID] {
			best[it.ArtistID] = it.Listens
			out[it.ArtistID] = c.year(i)
		}
	}
	return out
}

// sortInt64 sorts in place. It is an insertion sort because the lists are short.
func sortInt64(v []int64) {
	for i := 1; i < len(v); i++ {
		for j := i; j > 0 && v[j] < v[j-1]; j-- {
			v[j], v[j-1] = v[j-1], v[j]
		}
	}
}

// atoi reads a number and answers 0 when it is not one.
func atoi(s string) int { n, _ := strconv.Atoi(s); return n }

// tileBlock answers the list block name for a page key, or empty if it has none.
func tileBlock(key string) string {
	if t, ok := genreTile[key]; ok {
		return "tile:" + t
	}
	return ""
}

// keep lets the row keep items the member already has.
func (x Tpl) keep() Tpl { x.Keep = true; return x }

// crossFill answers the page key's albums that are also in the slot key.
func crossFill(c *Ctx, v string) L {
	return c.by(c.in(palb(c), v), func(i int) float64 { return 0.5*(c.t(i, v)+c.t(i, c.Key))/1 + 0.5*c.lp(i) })
}

// rightNow answers the page key's charting or week-ranked albums.
func rightNow(c *Ctx, _ string) L {
	l := c.where(palb(c), func(i int) bool { return len(c.it(i).C) > 0 || c.it(i).R[rWeek] > 0 || c.it(i).R[rThisWeek] > 0 })
	return c.by(l, func(i int) float64 {
		return 0.5*c.t(i, c.Key) + 0.5*math.Max(c.p(i), math.Max(c.wp(i), rank01(c.it(i).R[rThisWeek])))
	})
}

// gems answers the page key's mid-listened albums that have loyal listeners.
func gems(c *Ctx, _ string) L {
	return c.by(c.notHad(c.loyal(c.pct(palb(c), 0.4, 0.8))), func(i int) float64 { return 0.5*c.t(i, c.Key) + 0.5*c.n(i) })
}

// union2 is union as a method, for template chains.
func (c *Ctx) union2(a, b L) L { return union(a, b) }

// seasonOn reports whether the page is a season page and that season is on.
func (c *Ctx) seasonOn() bool {
	if len(c.Key) < 8 || c.Key[:7] != "season:" {
		return false
	}
	return SeasonActive(c.Key[7:], c.Now, c.region())
}

// decadeRange answers the first and last year of the page's decade, or zeros off a decade page.
func (c *Ctx) decadeRange() (int, int) {
	if len(c.Key) >= 11 {
		y := atoi(c.Key[7:11])
		return y, y + 9
	}
	return 0, 0
}

// seedSim answers a library seed's artist similarities.
func (c *Ctx) seedSim(id string) map[string]float64 {
	if c.Lib != nil {
		for _, s := range c.Lib.Seeds {
			if s.ID == id {
				return s.Similar
			}
		}
	}
	return nil
}

// artistsOfAlbums answers the artist items of albums' artists, once each.
func (c *Ctx) artistsOfAlbums(l L) L {
	var out L
	seen := map[string]bool{}
	for _, i := range l {
		a := c.it(i).ArtistID
		if j, ok := c.B.artistIx[a]; ok && !seen[a] {
			seen[a] = true
			out = append(out, j)
		}
	}
	return out
}

// weekPlayed answers the page's albums ranked this week.
func weekPlayed(c *Ctx, _ string) L { return c.by(c.ranked(palb(c), rWeek), c.wp) }

// modern answers the page's albums 5 to 15 years old, by listens.
func modern(c *Ctx, _ string) L { return c.byLP(c.ageIn(palb(c), 5, 15)) }

// rising answers artists ranked this week but not this year.
func rising(c *Ctx, _ string) L {
	return c.by(c.where(c.ranked(part(c), rWeek), func(i int) bool { return c.it(i).R[rThisYear] == 0 }), c.wp)
}

// acrossArea answers the page's albums that chart in an area, best first.
func acrossArea(c *Ctx, v string) L {
	l := c.where(palb(c), func(i int) bool { return c.areaScore(i, v) > 0 })
	return c.by(l, func(i int) float64 { return c.areaScore(i, v) })
}
