package musictags

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Listen rank ranges, in the order of Album.Ranks and BItem.R.
const (
	RangeWeek     = iota // last full week
	RangeMonth           // last full month
	RangeThisYear        // this year so far
	RangeThisWeek        // this week so far
	RangeAllTime
	rangeCount
)

// RangeNames are the ListenBrainz range names, by index.
var RangeNames = [rangeCount]string{"week", "month", "this_year", "this_week", "all_time"}

// Album is one release group as collected.
type Album struct {
	ID, Title, Artist, ArtistID, Date, Type string
	Tags, ArtistTags                        []Tag
	Listens, Listeners                      int64
	Ranks                                   [rangeCount]int
	Art, Debut                              bool
	// Listed marks an album on a list shelf: kept whatever its listens.
	Listed      bool
	charts      map[string]int
	gains       map[string]float64
	chartGenres []string
}

// Artist is one artist as collected.
type Artist struct {
	ID, Name           string
	Tags               []Tag
	Listens, Listeners int64
	Ranks              [rangeCount]int
	Art                bool
	charts             map[string]int
}

// Source holds everything one build reads.
type Source struct {
	albums  map[string]*Album
	artists map[string]*Artist
}

// NewSource answers an empty source.
func NewSource() *Source {
	return &Source{albums: map[string]*Album{}, artists: map[string]*Artist{}}
}

// Album adds or merges an album: empty fields take the new value; counts
// and ranks keep the larger or better one.
func (s *Source) Album(a Album) {
	a.ID = strings.ToLower(a.ID)
	if a.ID == "" {
		return
	}
	old, ok := s.albums[a.ID]
	if !ok {
		c := a
		c.charts = map[string]int{}
		c.gains = map[string]float64{}
		s.albums[a.ID] = &c
		return
	}
	if old.Title == "" {
		old.Title = a.Title
	}
	if old.Artist == "" {
		old.Artist, old.ArtistID = a.Artist, a.ArtistID
	}
	if old.Date == "" || a.Date != "" && a.Date < old.Date {
		old.Date = a.Date
	}
	if old.Type == "" {
		old.Type = a.Type
	}
	if len(old.Tags) == 0 {
		old.Tags = a.Tags
	}
	if len(old.ArtistTags) == 0 {
		old.ArtistTags = a.ArtistTags
	}
	old.Listens, old.Listeners = max(old.Listens, a.Listens), max(old.Listeners, a.Listeners)
	for i, r := range a.Ranks {
		if r > 0 && (old.Ranks[i] == 0 || r < old.Ranks[i]) {
			old.Ranks[i] = r
		}
	}
	old.Art = old.Art || a.Art
	old.Debut = old.Debut || a.Debut
	old.Listed = old.Listed || a.Listed
}

// Artist adds or merges an artist.
func (s *Source) Artist(a Artist) {
	a.ID = strings.ToLower(a.ID)
	if a.ID == "" {
		return
	}
	old, ok := s.artists[a.ID]
	if !ok {
		c := a
		c.charts = map[string]int{}
		s.artists[a.ID] = &c
		return
	}
	if old.Name == "" {
		old.Name = a.Name
	}
	if len(old.Tags) == 0 {
		old.Tags = a.Tags
	}
	old.Listens, old.Listeners = max(old.Listens, a.Listens), max(old.Listeners, a.Listeners)
	for i, r := range a.Ranks {
		if r > 0 && (old.Ranks[i] == 0 || r < old.Ranks[i]) {
			old.Ranks[i] = r
		}
	}
	old.Art = old.Art || a.Art
}

// ArtistName answers a known artist's name.
func (s *Source) ArtistName(id string) string {
	if a, ok := s.artists[strings.ToLower(id)]; ok {
		return a.Name
	}
	return ""
}

// AlbumIDs answers every album id, sorted.
func (s *Source) AlbumIDs() []string {
	out := make([]string, 0, len(s.albums))
	for id := range s.albums {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// ChartGain records an album's 7-day rank gain (0..1) on one country's
// chart.
func (s *Source) ChartGain(country, id string, gain float64) {
	if a, ok := s.albums[strings.ToLower(id)]; ok && gain > 0 {
		a.gains[strings.ToLower(country)] = gain
	}
}

// HasAlbum reports whether the album is known.
func (s *Source) HasAlbum(id string) bool { _, ok := s.albums[strings.ToLower(id)]; return ok }

// Chart records an album's rank (1..100) on one country's chart with the
// chart's genre names; an unknown album is added bare.
func (s *Source) Chart(country, id string, rank int, genres ...string) {
	id = strings.ToLower(id)
	a, ok := s.albums[id]
	if !ok {
		s.Album(Album{ID: id})
		a = s.albums[id]
	}
	country = strings.ToLower(country)
	if r, ok := a.charts[country]; !ok || rank < r {
		a.charts[country] = rank
	}
	for _, g := range genres {
		dup := false
		for _, x := range a.chartGenres {
			dup = dup || strings.EqualFold(x, g)
		}
		if !dup && g != "" {
			a.chartGenres = append(a.chartGenres, g)
		}
	}
}

// ArtistChart records an artist's best song rank on one country's chart;
// an unknown artist is added bare.
func (s *Source) ArtistChart(country, id string, rank int) {
	id = strings.ToLower(id)
	if _, ok := s.artists[id]; !ok {
		s.Artist(Artist{ID: id})
	}
	a := s.artists[id]
	country = strings.ToLower(country)
	if r, ok := a.charts[country]; !ok || rank < r {
		a.charts[country] = rank
	}
}

// chartSignals answers the up to maxCountries best country ranks and P.
func chartSignals(charts map[string]int) (map[string]int, int) {
	if len(charts) == 0 {
		return nil, 0
	}
	type cr struct {
		c string
		r int
	}
	var l []cr
	best := 101
	for c, r := range charts {
		l = append(l, cr{c, r})
		best = min(best, r)
	}
	sort.Slice(l, func(i, j int) bool {
		if l[i].r != l[j].r {
			return l[i].r < l[j].r
		}
		return l[i].c < l[j].c
	})
	out := map[string]int{}
	for _, x := range l[:min(maxCountries, len(l))] {
		out[x.c] = x.r
	}
	return out, (101 - best) * 10
}

// BItem is one bundle item. K is "a" (album) or "r" (artist). P is the
// best chart popularity today ×1000 (rank 1 = 1000), CW the rank on the
// many-country score, C the ranks in up to 10 countries, R the listen
// ranks by range (0: not in that top 1,000).
type BItem struct {
	ID       string          `json:"id"`
	Kind     string          `json:"k"`
	Title    string          `json:"t"`
	Artist   string          `json:"ar,omitempty"`
	ArtistID string          `json:"ai,omitempty"`
	Date     string          `json:"d,omitempty"`
	Type     string          `json:"ty,omitempty"`
	Listens  int64           `json:"l,omitempty"`
	Users    int64           `json:"u,omitempty"`
	R        [rangeCount]int `json:"r"`
	C        map[string]int  `json:"c,omitempty"`
	CW       int             `json:"cw,omitempty"`
	P        int             `json:"p,omitempty"`
	M        int             `json:"m,omitempty"`
	G        map[string]int  `json:"g,omitempty"`
	Debut    bool            `json:"deb,omitempty"`
}

// KeyMember is one item in a key: its index in Items, its fit ×1000 and
// whether it is tagged with the key itself.
type KeyMember struct {
	I   int
	T   int
	Own bool
}

// MarshalJSON writes a member as [i, t, own].
func (m KeyMember) MarshalJSON() ([]byte, error) {
	o := 0
	if m.Own {
		o = 1
	}
	return json.Marshal([3]int{m.I, m.T, o})
}

// UnmarshalJSON reads [i, t, own].
func (m *KeyMember) UnmarshalJSON(b []byte) error {
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
	Parent string `json:"parent,omitempty"`
}

// Bundle is what every install reads.
type Bundle struct {
	SchemaVersion int                    `json:"schema_version"`
	Version       string                 `json:"version"`
	Built         string                 `json:"built"`
	Items         []BItem                `json:"items"`
	Keys          map[string][]KeyMember `json:"keys"`
	Names         map[string]KeyInfo     `json:"names"`
}

// maxCountries caps the country ranks kept per item.
const maxCountries = 10

// minListens is the fewest listens an album needs to be in the bundle
// when it is on no chart and in no listen ranking (fresh lists carry many
// albums almost nobody plays).
const minListens = 200

// Build makes the bundle: items with a cover only, chart signals, and each
// key's members by fit (best first). Version is a hash of the content, so
// it changes when any signal changes and never with the clock alone.
func Build(src *Source, rs []Recipe, now time.Time) *Bundle {
	b := &Bundle{SchemaVersion: 1, Keys: map[string][]KeyMember{}, Names: map[string]KeyInfo{}}
	for _, r := range rs {
		b.Names[r.Key] = KeyInfo{Name: r.Name, Kind: r.Kind, Parent: r.Parent}
	}
	// The many-country score: Σ (101 − rank)/100 over the album's charts.
	type cw struct {
		id string
		s  float64
	}
	var score []cw
	for id, a := range src.albums {
		s := 0.0
		for _, r := range a.charts {
			s += float64(101-r) / 100
		}
		if s > 0 {
			score = append(score, cw{id, s})
		}
	}
	sort.Slice(score, func(i, j int) bool {
		if score[i].s != score[j].s {
			return score[i].s > score[j].s
		}
		return score[i].id < score[j].id
	})
	cwRank := map[string]int{}
	for i, x := range score {
		cwRank[x.id] = i + 1
	}

	ids := make([]string, 0, len(src.albums))
	for id := range src.albums {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	keep := func(a *Album) bool {
		if !a.Art || a.Title == "" {
			return false
		}
		if len(a.charts) > 0 || a.Listed || a.Listens >= minListens {
			return true
		}
		for _, r := range a.Ranks {
			if r > 0 {
				return true
			}
		}
		return false
	}
	// Listens cut-offs for date-and-listens recipes, and artist tags
	// borrowed from albums for artists that have none of their own.
	var listens []int64
	artistTags := map[string][]Tag{}
	for _, id := range ids {
		a := src.albums[id]
		if !keep(a) {
			continue
		}
		listens = append(listens, a.Listens)
		if aid := strings.ToLower(a.ArtistID); len(a.ArtistTags) > 0 && len(artistTags[aid]) == 0 {
			artistTags[aid] = a.ArtistTags
		}
	}
	sort.Slice(listens, func(i, j int) bool { return listens[i] > listens[j] })
	cutoff := func(share float64) int64 {
		if len(listens) == 0 {
			return 0
		}
		return listens[min(len(listens)-1, int(float64(len(listens))*share))]
	}
	add := func(it BItem, m map[string]Member) {
		i := len(b.Items)
		b.Items = append(b.Items, it)
		for k, x := range m {
			b.Keys[k] = append(b.Keys[k], KeyMember{I: i, T: int(math.Round(x.T * 1000)), Own: x.Own})
		}
	}
	for _, id := range ids {
		a := src.albums[id]
		if !keep(a) {
			continue
		}
		it := BItem{ID: id, Kind: "a", Title: a.Title, Artist: a.Artist, ArtistID: strings.ToLower(a.ArtistID), Date: a.Date, Type: a.Type, Listens: a.Listens, Users: a.Listeners, R: a.Ranks, CW: cwRank[id], Debut: a.Debut}
		it.C, it.P = chartSignals(a.charts)
		for c := range it.C {
			if g := int(math.Round(a.gains[c] * 1000)); g > 0 {
				if it.G == nil {
					it.G = map[string]int{}
				}
				it.G[c] = g
				it.M = max(it.M, g)
			}
		}
		at := a.ArtistTags
		if len(at) == 0 {
			if ar, ok := src.artists[strings.ToLower(a.ArtistID)]; ok {
				at = ar.Tags
			}
		}
		m := Members(Item{ID: id, Kind: "album", Title: a.Title, Artist: a.Artist, Date: a.Date, Type: a.Type, AlbumTags: a.Tags, ArtistTags: at, ChartGenres: a.chartGenres}, rs)
		for _, r := range rs {
			if r.OlderThan > 0 && len(a.Date) >= 4 && a.Listens > 0 && a.Listens >= cutoff(r.TopShare) {
				if y, err := strconv.Atoi(a.Date[:4]); err == nil && now.Year()-y >= r.OlderThan {
					if _, ok := m[r.Key]; !ok {
						m[r.Key] = Member{T: 1}
					}
				}
			}
		}
		add(it, m)
	}
	ids = ids[:0]
	for id := range src.artists {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		ar := src.artists[id]
		if !ar.Art || ar.Name == "" {
			continue
		}
		tg := ar.Tags
		if len(tg) == 0 {
			tg = artistTags[id]
		}
		m := Members(Item{ID: id, Kind: "artist", Title: ar.Name, AlbumTags: tg}, artistRecipes(rs))
		it := BItem{ID: id, Kind: "r", Title: ar.Name, Listens: ar.Listens, Users: ar.Listeners, R: ar.Ranks}
		it.C, it.P = chartSignals(ar.charts)
		add(it, m)
	}
	for k, l := range b.Keys {
		sort.SliceStable(l, func(i, j int) bool {
			if l[i].T != l[j].T {
				return l[i].T > l[j].T
			}
			return b.Items[l[i].I].Listens > b.Items[l[j].I].Listens
		})
		b.Keys[k] = l
	}
	body, _ := json.Marshal(struct {
		I []BItem
		K map[string][]KeyMember
		N map[string]KeyInfo
	}{b.Items, b.Keys, b.Names})
	h := sha256.Sum256(body)
	b.Version = hex.EncodeToString(h[:8])
	b.Built = now.UTC().Format(time.RFC3339)
	return b
}

// artistRecipes are the recipes an artist can match: decades and title
// words are album facts, so they are left out.
func artistRecipes(rs []Recipe) []Recipe {
	var out []Recipe
	for _, r := range rs {
		if r.Kind == KindDecade || r.AlbumTagsOnly {
			continue
		}
		r.Words = nil
		out = append(out, r)
	}
	return out
}
