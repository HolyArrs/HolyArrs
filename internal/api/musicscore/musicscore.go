// Package musicscore holds the Music rows' signals, the shared dedup, the
// row gate and order, and the three cache layers (plan 2026-10-22 §2).
// Everything here is pure: callers pass the data and the clock.
package musicscore

import (
	"encoding/binary"
	"hash/fnv"
	"math"
	"sort"
	"time"
)

// Lnorm is log10(1+x)/log10(1+max) over a candidate pool; an empty pool
// or a max of 0 gives 0.
func Lnorm(x, max int64) float64 {
	if max <= 0 || x <= 0 {
		return 0
	}
	return math.Min(1, math.Log10(1+float64(x))/math.Log10(1+float64(max)))
}

// Popularity is P for a charted item: 1 − (rank − 1)/L, L the length of
// the chart the item came from. Rank or L under 1 gives 0.
func Popularity(rank, length int) float64 {
	if length < 1 || rank < 1 || rank > length {
		return 0
	}
	return 1 - float64(rank-1)/float64(length)
}

// Uncharted is P for an item on no chart: min(0.3, 0.3·Lnorm).
func Uncharted(lnorm float64) float64 { return math.Min(0.3, 0.3*lnorm) }

// Freshness is F_t = exp(−d/t), d whole days since the release (0 for a
// future date). An unknown or bad date gives 0.
func Freshness(date string, t float64, now time.Time) float64 {
	if len(date) < 10 || t <= 0 {
		return 0
	}
	d, err := time.Parse("2006-01-02", date[:10])
	if err != nil {
		return 0
	}
	days := math.Floor(now.UTC().Sub(d).Hours() / 24)
	if days < 0 {
		days = 0
	}
	return math.Exp(-days / t)
}

// Acts are one member's and the household's deliberate acts for an artist.
type Acts struct {
	Follows, Asked           bool // the member
	OtherFollows, OtherAsked bool // any other household member
	HaveAny                  bool // the household has any album by the artist
	AlbumsHad                int
}

// Affinity is A = min(raw, 6)/6, raw = 3·f_m + 2·k_m + 1.5·f_o + 1·k_o +
// 1·h + 0.5·log2(1+n_had).
func Affinity(x Acts) float64 {
	b := func(v bool) float64 {
		if v {
			return 1
		}
		return 0
	}
	raw := 3*b(x.Follows) + 2*b(x.Asked) + 1.5*b(x.OtherFollows) + b(x.OtherAsked) + b(x.HaveAny) + 0.5*math.Log2(1+float64(max(0, x.AlbumsHad)))
	return math.Min(raw, 6) / 6
}

// OwnOnly reports whether the member has any own act (f_m or k_m) on any
// artist; without one the household terms carry A (cold A).
func OwnOnly(all []Acts) bool {
	for _, x := range all {
		if x.Follows || x.Asked {
			return true
		}
	}
	return false
}

// GenreAffinity is G per genre: w(g) = Σ A(a)·share(a,g) over artists with
// tags, kept when at least 2 artists contribute, top 12, divided by the
// max. A max of 0 (or no tags at all) gives every G = 0 (an empty map).
// shares maps artist → genre → vote share; aff maps artist → A.
func GenreAffinity(aff map[string]float64, shares map[string]map[string]float64) map[string]float64 {
	w := map[string]float64{}
	n := map[string]int{}
	for a, gs := range shares {
		A := aff[a]
		for g, sh := range gs {
			if sh <= 0 || g == "" {
				continue
			}
			w[g] += A * sh
			if A > 0 {
				n[g]++
			}
		}
	}
	type gw struct {
		g string
		w float64
	}
	var list []gw
	for g, v := range w {
		if n[g] >= 2 && v > 0 {
			list = append(list, gw{g, v})
		}
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].w != list[j].w {
			return list[i].w > list[j].w
		}
		return list[i].g < list[j].g
	})
	out := map[string]float64{}
	if len(list) == 0 {
		return out
	}
	list = list[:min(12, len(list))]
	top := list[0].w
	for _, x := range list {
		out[x.g] = x.w / top
	}
	return out
}

// Norm divides each value by the list's max; a max of 0 gives all 0.
func Norm(v []float64) []float64 {
	m := 0.0
	for _, x := range v {
		m = math.Max(m, x)
	}
	out := make([]float64, len(v))
	if m <= 0 {
		return out
	}
	for i, x := range v {
		out[i] = x / m
	}
	return out
}

// Tie is the day's tie-break: hash(member, date, id), the same all day.
func Tie(member int64, date, id string) uint64 {
	h := fnv.New64a()
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], uint64(member))
	_, _ = h.Write(b[:])
	_, _ = h.Write([]byte(date))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(id))
	return h.Sum64()
}
