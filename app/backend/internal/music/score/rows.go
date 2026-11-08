package musicscore

import (
	"sort"
)

const (
	// PriorPersonal weighs rows built from the member's own taste.
	PriorPersonal = 1.2
	// PriorMixed weighs rows built from taste and charts together.
	PriorMixed = 1.0
	// PriorChart weighs rows built from charts alone.
	PriorChart = 0.9
	// RowMax caps a filled row.
	RowMax = 20
	// PerArtist is the most items one artist has in a row. Artist rows are
	// exempt.
	PerArtist = 2
)

// Item is one candidate. Album is the release group id (empty for artists
// and songs). FoldKey is the title and artist key that folds editions of
// one album together. Artist is the primary artist's key.
type Item struct {
	Key     string
	Album   string
	FoldKey string
	Artist  string
	Score   float64
}

// Row is one row's raw candidates, already in the row's own order.
type Row struct {
	ID     string
	Anchor bool
	Prior  float64
	// Min is the fewest items the row shows. Zero means the default of 6.
	Min int
	// ArtistRow exempts the row from the per-artist cap.
	ArtistRow bool
	// Seed is the artist a seeded row is about. Only one row per seed stays.
	Seed  string
	Items []Item
	// Family groups rows for the variety rules. Empty means the row's ID.
	// Disc marks a discovery row.
	Family string
	Disc   bool
	// ChartSet makes Chart alone decide whether this is a chart row. When
	// false, a Prior equal to PriorChart decides.
	ChartSet bool
	Chart    bool
	// Shape is "round" or "wide" for rows that count toward the shape rule.
	Shape string
	// ScoreFn, when set, scores the row over its items not yet placed.
	ScoreFn func(unseen []Item) float64 `json:"-"`
	// MinArtists is the fewest artists a filled row needs.
	MinArtists int
}

// Gate returns the fewest items the row needs, 6 unless Min is set.
func (r Row) Gate() int {
	if r.Min > 0 {
		return r.Min
	}
	return 6
}

// Passes reports whether n items pass the row's gate.
func (r Row) Passes(n int) bool { return n >= r.Gate() }

// Strength returns the mean of the top 6 scores times prior, or 0 for an
// empty row.
func Strength(scores []float64, prior float64) float64 {
	if len(scores) == 0 {
		return 0
	}
	s := append([]float64{}, scores...)
	sort.Sort(sort.Reverse(sort.Float64Slice(s)))
	n := min(6, len(s))
	sum := 0.0
	for _, x := range s[:n] {
		sum += x
	}
	return sum / float64(n) * prior
}

// Order fixes the row order from raw candidates: anchors first in the order
// given, then the rest by strength, with ties broken by a daily hash.
func Order(rows []Row, member int64, date string) []Row {
	var anchors, rest []Row
	for _, r := range rows {
		if r.Anchor {
			anchors = append(anchors, r)
		} else {
			rest = append(rest, r)
		}
	}
	sort.SliceStable(rest, func(i, j int) bool {
		a, b := rest[i].strength(), rest[j].strength()
		if a != b {
			return a > b
		}
		return Tie(member, date, rest[i].ID) < Tie(member, date, rest[j].ID)
	})
	return append(anchors, rest...)
}

// Dedup fills rows in order with one shared seen set over release groups,
// fold keys and item keys, so no item shows twice. It also applies the
// per-artist cap and keeps one row per seed, then drops rows under their
// gate without giving their items back. seen may already hold placed keys,
// such as the hero. rows must already be in Order.
func Dedup(rows []Row, seen map[string]bool) []Row {
	if seen == nil {
		seen = map[string]bool{}
	}
	seeds := map[string]bool{}
	var out []Row
	for _, r := range rows {
		if r.Seed != "" {
			if seeds[r.Seed] {
				continue
			}
			seeds[r.Seed] = true
		}
		per := map[string]int{}
		var kept []Item
		for _, it := range r.Items {
			if len(kept) >= RowMax {
				break
			}
			if seen[it.Key] || it.Album != "" && seen["rg:"+it.Album] || it.FoldKey != "" && seen["fold:"+it.FoldKey] {
				continue
			}
			if !r.ArtistRow && it.Artist != "" && per[it.Artist] >= PerArtist {
				continue
			}
			per[it.Artist]++
			kept = append(kept, it)
			seen[it.Key] = true
			if it.Album != "" {
				seen["rg:"+it.Album] = true
			}
			if it.FoldKey != "" {
				seen["fold:"+it.FoldKey] = true
			}
		}
		r.Items = kept
		if r.Passes(len(kept)) {
			out = append(out, r)
		}
	}
	return out
}

// chartLike reports whether the row counts as a chart row for variety.
func (r Row) chartLike() bool {
	if r.ChartSet {
		return r.Chart
	}
	return r.Prior == PriorChart
}

// shaped reports whether the row is a round or wide row.
func (r Row) shaped() bool { return r.Shape == "round" || r.Shape == "wide" }

// family returns the row's family, or its ID when none is set.
func (r Row) family() string {
	if r.Family != "" {
		return r.Family
	}
	return r.ID
}

// strength returns the row's strength from its item scores.
func (r Row) strength() float64 {
	s := make([]float64, len(r.Items))
	for i, it := range r.Items {
		s[i] = it.Score
	}
	return Strength(s, r.Prior)
}
