package rowtpl

import (
	"math"
	"sort"
	"strconv"
	"strings"

	musicscore "github.com/HolyArrs/holyarrs/app/backend/internal/music/score"
)

// RowMin is the fewest items any template row shows with.
const RowMin = 8

// MinArtists is the fewest different artists an album row needs.
const MinArtists = 5

// rawMax caps a candidate row's raw items before dedup.
const rawMax = 60

// SpotlightMax is how many albums the Spotlight shows at most. With fewer than SpotlightMin the page has none.
const (
	SpotlightMax = 10
	SpotlightMin = 3
)

// pageLetter maps a page kind to the letter templates use to name the pages they run on.
var pageLetter = map[string]byte{"genre": 'G', "mood": 'M', "season": 'S', "decade": 'D', "activity": 'A', "explore": 'E', "new": 'N'}

// heads are the head templates per page letter, in order. They fill the Spotlight and lead the page.
var heads = map[byte][]string{
	'G': {"D01", "E01"},
	'M': {"H01"},
	'A': {"K01"},
	'S': {"B10", "I02"},
	'D': {"J01"},
	'N': {"B01", "A01"},
	'E': {"B06", "A01"},
}

// shortName is a key's short name in titles, where the catalog name is too long.
var shortName = map[string]string{"decade:2000s": "2000s", "decade:2010s": "2010s", "decade:2020s": "2020s", "genre:dance": "Dance", "genre:folk": "Folk", "genre:reggae": "Reggae", "genre:christian": "Christian", "genre:kids": "Kids", "genre:soundtracks": "Soundtrack"}

// forOf is an activity's "for ..." phrase.
var forOf = map[string]string{"activity:driving": "for driving", "activity:cooking": "for cooking", "activity:gaming": "for gaming", "activity:studying": "for studying", "activity:gym": "for the gym", "activity:running": "for running", "activity:home": "for a night in", "activity:party": "for a party"}

// areas are fixed country groups.
var areas = map[string][]string{
	"north-america": {"us", "ca"},
	"latin-america": {"mx", "br", "ar", "co", "cl", "pe", "ec", "ve", "do", "gt", "cr", "pa", "py", "uy", "bo", "hn", "sv", "ni", "jm"},
	"europe":        {"gb", "de", "fr", "es", "it", "nl", "se", "no", "dk", "fi", "be", "at", "ch", "ie", "pt", "pl", "cz", "sk", "hu", "ro", "bg", "gr", "hr", "si", "rs", "ua", "lt", "lv", "ee", "ru", "cy"},
	"africa":        {"za", "ng", "ke", "ci", "sn", "ma", "dz", "tn", "eg"},
	"middle-east":   {"ae", "sa", "il", "jo", "lb", "tr"},
	"asia-pacific":  {"jp", "kr", "in", "id", "ph", "my", "sg", "th", "vn", "tw", "hk", "au", "nz", "pk"},
}

// areaName is an area's name as titles write it.
var areaName = map[string]string{"north-america": "North America", "latin-america": "Latin America", "europe": "Europe", "africa": "Africa", "middle-east": "the Middle East", "asia-pacific": "Asia-Pacific"}

// bigMarkets are the largest music markets, in order.
var bigMarkets = []string{"us", "gb", "de", "jp", "fr", "br", "kr", "ca", "au", "mx", "it", "es", "nl", "se", "in"}

// areaOrder is the order areas are offered in.
var areaOrder = []string{"north-america", "latin-america", "europe", "africa", "middle-east", "asia-pacific"}

// countryNames name the chart countries for titles.
var countryNames = map[string]string{
	"us": "the US", "gb": "the UK", "ca": "Canada", "au": "Australia", "nz": "New Zealand", "ie": "Ireland", "de": "Germany", "fr": "France", "es": "Spain", "it": "Italy",
	"nl": "the Netherlands", "be": "Belgium", "at": "Austria", "ch": "Switzerland", "se": "Sweden", "no": "Norway", "dk": "Denmark", "fi": "Finland", "pt": "Portugal", "pl": "Poland",
	"cz": "Czechia", "sk": "Slovakia", "hu": "Hungary", "ro": "Romania", "bg": "Bulgaria", "gr": "Greece", "hr": "Croatia", "si": "Slovenia", "rs": "Serbia", "ua": "Ukraine",
	"lt": "Lithuania", "lv": "Latvia", "ee": "Estonia", "ru": "Russia", "cy": "Cyprus", "tr": "Turkey", "il": "Israel", "ae": "the UAE", "sa": "Saudi Arabia", "jo": "Jordan",
	"lb": "Lebanon", "eg": "Egypt", "ma": "Morocco", "dz": "Algeria", "tn": "Tunisia", "ng": "Nigeria", "ke": "Kenya", "za": "South Africa", "ci": "Ivory Coast", "sn": "Senegal",
	"cm": "Cameroon", "gh": "Ghana", "mx": "Mexico", "br": "Brazil", "ar": "Argentina", "co": "Colombia", "cl": "Chile", "pe": "Peru", "ec": "Ecuador", "ve": "Venezuela",
	"do": "the Dominican Republic", "gt": "Guatemala", "cr": "Costa Rica", "pa": "Panama", "py": "Paraguay", "uy": "Uruguay", "bo": "Bolivia", "hn": "Honduras", "sv": "El Salvador",
	"ni": "Nicaragua", "jm": "Jamaica", "jp": "Japan", "kr": "South Korea", "in": "India", "id": "Indonesia", "ph": "the Philippines", "my": "Malaysia", "sg": "Singapore",
	"th": "Thailand", "vn": "Vietnam", "tw": "Taiwan", "hk": "Hong Kong", "pk": "Pakistan", "kz": "Kazakhstan", "lu": "Luxembourg", "is": "Iceland",
}

// commonSeasons are season names written in lower case in mid-title.
var commonSeasons = map[string]bool{"season:fall": true, "season:spring": true, "season:summer": true, "season:back-to-school": true}

// Row is one filled page row. Items are bundle indices in row order.
type Row struct {
	// ID is the template id, plus ":" and the slot value.
	ID       string
	Template string
	Title    string
	Reason   string
	Shape    string
	Family   string
	Chart    bool
	Keep     bool
	Items    []int
}

// Cand is one candidate row before the plan orders and dedups it: its raw items, best first, with their scores.
type Cand struct {
	ID, Template  string
	Title, Reason string
	Shape, Family string
	Chart, Disc   bool
	Keep          bool
	Prior         float64
	Items         []int
	Scores        []float64
}

// LibraryKeys answers the library's genre keys, strongest first, by the affinity of its artists and the albums it has. It also answers the decade key the library has most albums in.
func LibraryKeys(b *Bundle, l *Lib) ([]string, string) {
	w := map[string]float64{}
	dec := map[string]int{}
	for id, a := range l.Artists {
		if i, ok := b.artistIx[id]; ok {
			for k, m := range b.byKey {
				if kindOf(k) == "genre" {
					if x, in := m[i]; in {
						w[k] += a * float64(x.T) / 1000
					}
				}
			}
		}
	}
	for id := range l.Albums {
		i, ok := b.albumIx[id]
		if !ok {
			continue
		}
		for k, m := range b.byKey {
			if _, in := m[i]; in {
				switch kindOf(k) {
				case "genre":
					w[k] += 0.5
				case "decade":
					dec[k]++
				}
			}
		}
	}
	var g []string
	for k := range w {
		g = append(g, k)
	}
	sort.Slice(g, func(i, j int) bool {
		if w[g[i]] != w[g[j]] {
			return w[g[i]] > w[g[j]]
		}
		return g[i] < g[j]
	})
	best, d := 0, ""
	for k, n := range dec {
		if n > best || n == best && k < d {
			best, d = n, k
		}
	}
	return g, d
}

// Spotlight answers the page's hero albums: up to SpotlightMax albums by different artists, not had, taken from the head templates in order. The most-listened albums come last as a fallback. They are marked seen, so no row repeats them.
func Spotlight(c *Ctx) []int {
	letter := pageLetter[c.Kind]
	type src struct {
		tp Tpl
		v  string
	}
	var srcs []src
	for _, h := range heads[letter] {
		tp := Catalog[indexOf(h)]
		if !c.applies(tp) {
			continue
		}
		for _, v := range c.slotValues(tp) {
			srcs = append(srcs, src{tp, v})
		}
	}
	// Last resort: the most-listened albums of the page, or of the whole catalog.
	srcs = append(srcs, src{Tpl{Fill: func(c *Ctx, _ string) L {
		if c.Key == "" {
			return c.byLP(c.albums(c.all()))
		}
		return c.byLP(palb(c))
	}}, ""})
	var out []int
	artists := map[string]bool{}
	for _, sv := range srcs {
		for _, i := range c.pictured(c.notHad(c.albums(sv.tp.Fill(c, sv.v)))) {
			if len(out) == SpotlightMax {
				break
			}
			if a := c.artistOf(i); !artists[a] {
				artists[a] = true
				out = append(out, i)
			}
		}
		if len(out) == SpotlightMax {
			break
		}
	}
	if len(out) < SpotlightMin {
		return nil
	}
	if c.Seen == nil {
		c.Seen = map[string]bool{}
	}
	for _, i := range out {
		it := c.it(i)
		c.Seen["a:"+it.ID] = true
		c.Seen["rg:"+it.ID] = true
		c.Seen["fold:"+foldKey(it)] = true
	}
	return out
}

// Candidates answers every template row that applies to the page with at least RowMin raw items. Builders that run their own plan, like Explore's, use it.
func Candidates(c *Ctx) []Cand {
	var out []Cand
	for _, tp := range Catalog {
		if !c.applies(tp) {
			continue
		}
		for _, v := range c.slotValues(tp) {
			l := tp.Fill(c, v)
			if !tp.Keep {
				l = c.notHad(l)
			}
			l, pictured := c.picturedFirst(l)
			if !pictured {
				continue
			}
			if len(l) > rawMax {
				l = l[:rawMax]
			}
			if len(l) < RowMin {
				continue
			}
			id := tp.ID
			if v != "" {
				id += ":" + v
			}
			cd := Cand{ID: id, Template: tp.ID, Shape: shapeOr(tp.Shape), Family: tp.Fam, Chart: tp.Chart, Disc: tp.Disc, Keep: tp.Keep, Prior: tp.Prior, Items: l}
			for _, i := range l {
				cd.Scores = append(cd.Scores, c.itemScore(i, tp, v))
			}
			cd.Title, cd.Reason = c.render(tp, v, l[:min(20, len(l))], id)
			out = append(out, cd)
		}
	}
	return out
}

// PageItems answers every item the page can show to anyone with an empty library, best first. Each member's rows are drawn from these lists, whatever the day's order and dedup.
func PageItems(c *Ctx) []int {
	seen := map[int]bool{}
	var out []int
	add := func(l []int) {
		for _, i := range l {
			if !seen[i] {
				seen[i] = true
				out = append(out, i)
			}
		}
	}
	x := *c
	x.Seen = nil
	add(Spotlight(&x))
	for _, tp := range Catalog {
		if !c.applies(tp) {
			continue
		}
		for _, v := range c.slotVals(tp, true) {
			l := tp.Fill(c, v)
			if !tp.Keep {
				l = c.notHad(l)
			}
			if len(l) < RowMin {
				continue
			}
			add(l[:min(rawMax, len(l))])
		}
	}
	return out
}

// Build fills the page: every template that applies, expanded over its slot values, then ordered and deduped by the row plan. It gives at most 30 rows, each with at least RowMin items and two per artist, and at least five artists for album rows.
func Build(c *Ctx) []Row {
	letter := pageLetter[c.Kind]
	type meta struct {
		tp Tpl
		v  string
	}
	metas := map[string]meta{}
	var rows []musicscore.Row
	withLib := !c.Lib.empty()
	for _, tp := range Catalog {
		if !c.applies(tp) {
			continue
		}
		for _, v := range c.slotValues(tp) {
			l := tp.Fill(c, v)
			if !tp.Keep {
				l = c.notHad(l)
			}
			l, pictured := c.picturedFirst(l)
			if !pictured {
				continue
			}
			if len(l) > rawMax {
				l = l[:rawMax]
			}
			if len(l) < RowMin {
				continue
			}
			id := tp.ID
			if v != "" {
				id += ":" + v
			}
			r := musicscore.Row{ID: id, Family: tp.Fam, Disc: tp.Disc, Prior: musicscore.PriorMixed, ChartSet: true, Chart: tp.Chart, Shape: tp.Shape, Min: RowMin}
			if tp.Shape != "round" && !(tp.ID == "G13") {
				r.MinArtists = MinArtists
			}
			r.ArtistRow = tp.ID == "G13" || tp.ID == "C13"
			for _, i := range l {
				it := c.it(i)
				x := musicscore.Item{Key: it.Kind + ":" + it.ID, Artist: c.artistOf(i), Score: c.itemScore(i, tp, v)}
				if it.Kind == "a" {
					x.Album, x.FoldKey = it.ID, foldKey(it)
				}
				r.Items = append(r.Items, x)
			}
			prior, jit := tp.Prior, 0.95+0.1*float64(hash(strconv.FormatInt(c.Member, 10), c.Day(), id)%1000)/1000
			wPr := 0.15
			if !withLib {
				wPr = 0.15 / 0.80
			}
			r.ScoreFn = func(unseen []musicscore.Item) float64 {
				s := make([]float64, len(unseen))
				for k, it := range unseen {
					s[k] = it.Score
				}
				sort.Sort(sort.Reverse(sort.Float64Slice(s)))
				n := min(8, len(s))
				sum := 0.0
				for _, x := range s[:n] {
					sum += x
				}
				return (sum/float64(max(n, 1)) + wPr*prior) * jit
			}
			rows = append(rows, r)
			metas[id] = meta{tp, v}
		}
	}
	var head []string
	for _, h := range heads[letter] {
		for _, r := range rows {
			if metas[r.ID].tp.ID == h {
				head = append(head, r.ID)
				break
			}
		}
	}
	maxOf := map[string]int{}
	for k, v := range familyMax {
		maxOf[k] = v
	}
	if letter == 'E' {
		maxOf["pe"] = 10
	}
	plan := musicscore.NewPlan(rows, head, maxOf, c.Member, c.Day(), c.Seen)
	served := plan.All(nil)
	var out []Row
	for _, r := range served {
		m := metas[r.ID]
		var items []int
		for _, it := range r.Items {
			k, id, _ := strings.Cut(it.Key, ":")
			if k == "r" {
				items = append(items, c.B.artistIx[id])
			} else {
				items = append(items, c.B.albumIx[id])
			}
		}
		title, reason := c.render(m.tp, m.v, items, r.ID)
		out = append(out, Row{ID: r.ID, Template: m.tp.ID, Title: title, Reason: reason, Shape: shapeOr(m.tp.Shape), Family: m.tp.Fam, Chart: m.tp.Chart, Keep: m.tp.Keep, Items: items})
	}
	return out
}

// areaOf answers the area a country belongs to, or North America for an unknown one.
func areaOf(country string) string {
	for _, a := range areaOrder {
		for _, c := range areas[a] {
			if c == country {
				return a
			}
		}
	}
	return "north-america"
}

// name is a key's name for titles.
func (c *Ctx) name(key string) string {
	if n, ok := shortName[key]; ok {
		return n
	}
	if n := c.B.Names[key].Name; n != "" {
		return n
	}
	return key
}

// kindOf is the part of a key before the colon.
func kindOf(key string) string { k, _, _ := strings.Cut(key, ":"); return k }

// keysOf answers the bundle keys of one kind, in a stable order.
func (c *Ctx) keysOf(kind string) []string {
	var out []string
	for k := range c.B.Keys {
		if kindOf(k) == kind {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// regionWeight orders keys by how many of their albums chart in the region.
func (c *Ctx) regionWeight(keys []string) []string {
	r := c.region()
	n := map[string]int{}
	for _, k := range keys {
		for _, m := range c.B.Keys[k] {
			if _, ok := c.it(m.I).C[r]; ok {
				n[k]++
			}
		}
	}
	out := append([]string{}, keys...)
	sort.SliceStable(out, func(i, j int) bool {
		if n[out[i]] != n[out[j]] {
			return n[out[i]] > n[out[j]]
		}
		return len(c.B.Keys[out[i]]) > len(c.B.Keys[out[j]])
	})
	return out
}

// enough keeps candidate values whose pool has at least RowMin albums.
func (c *Ctx) enough(vals []string, f func(v string) L) []string {
	var out []string
	for _, v := range vals {
		if len(f(v)) >= RowMin {
			out = append(out, v)
		}
	}
	return out
}

// slotValues answers the values a template expands over on this page, in the week's rotation, at most Cap.
func (c *Ctx) slotValues(tp Tpl) []string { return c.slotVals(tp, false) }

// slotVals answers a template's slot values. With all set it answers every value any member may get, with no weekly rotation and no cap.
func (c *Ctx) slotVals(tp Tpl, all bool) []string {
	letter := pageLetter[c.Kind]
	var vals []string
	switch tp.Slot {
	case "":
		return []string{""}
	case "cross":
		kind := tp.Cross[letter]
		cand := c.keysOf(kind)
		if kind == "season" && letter != 'S' {
			var on []string
			for _, k := range cand {
				if SeasonActive(strings.TrimPrefix(k, "season:"), c.Now, c.region()) {
					on = append(on, k)
				}
			}
			cand = on
		}
		var keep []string
		for _, k := range cand {
			if !tp.NotWith[k] {
				keep = append(keep, k)
			}
		}
		vals = c.enough(keep, func(v string) L { return c.in(palb(c), v) })
	case "decade":
		vals = c.enough(c.keysOf("decade"), func(v string) L { return c.in(palb(c), v) })
	case "sub":
		var subs []string
		for _, k := range c.keysOf("sub") {
			if c.B.Names[k].Parent == c.Key {
				subs = append(subs, k)
			}
		}
		vals = c.enough(subs, func(v string) L { return kalb(c, v) })
	case "genre2":
		type kv struct {
			k string
			n int
		}
		var l []kv
		for _, g := range c.keysOf("genre") {
			if g != c.Key {
				if n := len(c.in(palb(c), g)); n >= RowMin {
					l = append(l, kv{g, n})
				}
			}
		}
		sort.SliceStable(l, func(i, j int) bool { return l[i].n > l[j].n })
		for _, x := range l[:min(3, len(l))] {
			vals = append(vals, x.k)
		}
	case "country":
		// Countries near the viewer come first, then the big markets, so an obscure chart never leads.
		r := c.region()
		count := map[string]int{}
		src := palb(c)
		if c.Key == "" {
			src = c.albums(c.all())
		}
		for _, i := range src {
			for cc := range c.it(i).C {
				if cc != r {
					count[cc]++
				}
			}
		}
		near := map[string]bool{}
		for _, cc := range areas[areaOf(r)] {
			near[cc] = true
		}
		rank := map[string]int{}
		for i, cc := range bigMarkets {
			rank[cc] = i + 1
		}
		for cc, n := range count {
			if n >= RowMin && (near[cc] || rank[cc] > 0) {
				vals = append(vals, cc)
			}
		}
		sort.SliceStable(vals, func(i, j int) bool {
			a, b := vals[i], vals[j]
			if near[a] != near[b] {
				return near[a]
			}
			if rank[a] != rank[b] {
				if rank[a] == 0 || rank[b] == 0 {
					return rank[a] != 0
				}
				return rank[a] < rank[b]
			}
			return a < b
		})
		vals = vals[:min(3, len(vals))]
	case "area":
		mine := areaOf(c.region())
		vals = append([]string{mine}, rotate(without(areaOrder, mine), hash(c.week(), "area"))...)
		return vals[:min(tp.Cap, len(vals))]
	case "year":
		lo, hi := c.decadeRange()
		if lo == 0 {
			lo, hi = c.Now.Year()-30, c.Now.Year()-1
		}
		for y := hi; y >= lo; y-- {
			vals = append(vals, strconv.Itoa(y))
		}
		vals = c.enough(vals, func(v string) L { return c.inYear(palb(c), atoi(v)) })
	case "n":
		for _, n := range []int{10, 20, 25, 30, 40, 50} {
			vals = append(vals, strconv.Itoa(n))
		}
		vals = c.enough(vals, func(v string) L { return c.inYear(palb(c), c.Now.Year()-atoi(v)) })
	case "genre":
		if c.Kind == "genre" {
			return []string{c.Key}
		}
		g := c.regionWeight(c.keysOf("genre"))
		if c.Kind == "explore" && c.Lib != nil && len(c.Lib.Genres) > 0 {
			g = c.Lib.Genres
		}
		vals = g
	case "libgenre":
		if c.Lib == nil {
			return nil
		}
		vals = c.Lib.Genres
	case "libdecade":
		if c.Lib == nil || c.Lib.Decade == "" {
			return nil
		}
		return []string{c.Lib.Decade}
	case "decade_page":
		if c.Kind == "decade" {
			return []string{c.Key}
		}
		if c.Lib != nil && c.Lib.Decade != "" {
			return []string{c.Lib.Decade}
		}
		return nil
	case "mood":
		vals = []string{"mood:chill", "mood:party", "mood:workout", "mood:love", "mood:focus", "mood:sleep"}
		if c.Kind == "mood" {
			return []string{c.Key}
		}
	case "season_on":
		if c.Kind == "season" {
			return []string{c.Key}
		}
		for _, s := range SeasonOrder(c.Now, c.region()) {
			if SeasonActive(s.Key, c.Now, c.region()) && len(kalb(c, "season:"+s.Key)) >= RowMin && s.Key != "fall" && s.Key != "spring" {
				return []string{"season:" + s.Key}
			}
		}
		return nil
	case "season_up":
		for _, s := range SeasonOrder(c.Now, c.region()) {
			if !SeasonActive(s.Key, c.Now, c.region()) && s.Start.Sub(dayOf(c.Now)).Hours() <= 21*24 && len(kalb(c, "season:"+s.Key)) >= RowMin {
				return []string{"season:" + s.Key}
			}
		}
		return nil
	case "seed":
		if c.Lib == nil {
			return nil
		}
		for _, s := range c.Lib.Seeds {
			// On a Browse page the seed must belong to the page's key.
			if c.Key != "" {
				i, ok := c.B.artistIx[s.ID]
				if !ok {
					continue
				}
				if _, in := c.B.byKey[c.Key][i]; !in {
					continue
				}
			}
			vals = append(vals, s.ID)
		}
	case "thisyear":
		return []string{strconv.Itoa(c.Now.Year())}
	}
	if len(vals) == 0 {
		return nil
	}
	if all {
		return vals
	}
	if tp.Slot != "genre" && tp.Slot != "libgenre" && tp.Slot != "mood" {
		vals = rotate(vals, hash(strconv.FormatInt(c.Member, 10), c.week(), c.Key, tp.ID))
	}
	return vals[:min(tp.Cap, len(vals))]
}

// rotate moves the start of v by h, so each member and week sees a different first value.
func rotate(v []string, h uint64) []string {
	if len(v) == 0 {
		return v
	}
	k := int(h % uint64(len(v)))
	return append(append([]string{}, v[k:]...), v[:k]...)
}

// without answers v with every x removed.
func without(v []string, x string) []string {
	var out []string
	for _, s := range v {
		if s != x {
			out = append(out, s)
		}
	}
	return out
}

// applies reports whether a template runs on this page with this data.
func (c *Ctx) applies(tp Tpl) bool {
	if !strings.ContainsRune(tp.Pages, rune(pageLetter[c.Kind])) || tp.Fill == nil {
		return false
	}
	if tp.Only != nil && !tp.Only[c.Key] {
		return false
	}
	switch tp.Need {
	case "lib":
		return !c.Lib.empty()
	case "seed":
		return c.Lib != nil && len(c.Lib.Seeds) > 0
	case "ls":
		return len(c.Blocks) > 0
	case "season":
		return c.seasonOn()
	case "v7", "cal":
		return false
	}
	return true
}

// itemScore is one item's share of the row score for a family. Without a library its weight is spread over the other terms.
func (c *Ctx) itemScore(i int, tp Tpl, v string) float64 {
	var pm float64
	switch tp.Fam {
	case "ch", "co", "nw":
		pm = math.Max(c.p(i), c.m(i))
		if tp.Fam == "nw" {
			pm = math.Max(pm, 0.6*c.lp(i))
		}
	case "po":
		pm = math.Max(c.wp(i), c.lp(i)*0.8)
	case "cl", "de", "ar":
		pm = c.lp(i)
	case "cr", "su", "se":
		k := c.Key
		if strings.Contains(v, ":") {
			k = v
		}
		pm = (0.5 + 0.5*c.t(i, k)) * c.lp(i)
	default:
		pm = 0.5*c.lp(i) + 0.5*c.n(i)
	}
	f := c.f(i, 30)
	if c.Lib.empty() {
		return (0.30*pm + 0.20*f + 0.15*c.n(i)) / 0.80
	}
	return 0.30*pm + 0.20*f + 0.20*c.l(i) + 0.15*c.n(i)
}

// foldKey is an album's title and artist in lower case, so reissues dedupe as one.
func foldKey(it *Item) string {
	return strings.ToLower(strings.TrimSpace(it.Title)) + "|" + strings.ToLower(strings.TrimSpace(it.Artist))
}

// indexOf answers a template's place in the catalog, or -1.
func indexOf(id string) int {
	for i, tp := range Catalog {
		if tp.ID == id {
			return i
		}
	}
	return -1
}
