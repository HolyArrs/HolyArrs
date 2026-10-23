package musicscore

import (
	"sort"
)

// Row priors (§2.3).
const (
	PriorPersonal = 1.2
	PriorMixed    = 1.0
	PriorChart    = 0.9
	// RowMax caps a filled row.
	RowMax = 20
	// PerArtist is the most items one artist has in a row (artist-centred
	// rows are exempt).
	PerArtist = 2
)

// Item is one candidate. Album is the release group MBID (empty for
// artists and songs), FoldKey the fullAlbumKey (title + artist) that folds
// editions, Artist the primary artist's key.
type Item struct {
	Key     string
	Album   string
	FoldKey string
	Artist  string
	Score   float64
}

// Row is one row's raw candidates (layer 1), already in the row's own
// order (its sort key).
type Row struct {
	ID     string
	Anchor bool
	Prior  float64
	// Min is the gate: the fewest items the row shows (default 6).
	Min int
	// ArtistRow exempts the row from the per-artist cap.
	ArtistRow bool
	// Seed is the artist a seeded row is about (at most one row each).
	Seed  string
	Items []Item
	// Family groups rows for the v7 plan's variety rules (music_rows_v7);
	// empty means the row's ID. Disc marks a discovery row.
	Family string
	Disc   bool
}

// family is the row's family, its ID when none is set.
func (r Row) family() string {
	if r.Family != "" {
		return r.Family
	}
	return r.ID
}

// Gate is the fewest items a row needs; 0 means the default 6.
func (r Row) Gate() int {
	if r.Min > 0 {
		return r.Min
	}
	return 6
}

// Passes reports whether n items pass the row's gate.
func (r Row) Passes(n int) bool { return n >= r.Gate() }

// Strength is R = mean of the top min(6, n) item scores × prior; an empty
// row is 0.
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

func (r Row) strength() float64 {
	s := make([]float64, len(r.Items))
	for i, it := range r.Items {
		s[i] = it.Score
	}
	return Strength(s, r.Prior)
}

// Order fixes the row order from raw candidates (step 2): anchors first in
// the order given, then the rest by R, ties by Tie(member, date, row id).
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

// Dedup fills rows in the fixed order (step 3) with one shared seen set
// over release-group MBIDs, fold keys and item keys, a per-row artist cap
// and one row per seed artist; then drops rows under their gate without
// giving their items back (step 4). seen may hold keys already placed
// (the hero). rows must already be in Order.
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
