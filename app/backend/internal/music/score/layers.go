// Package musicscore scores the Music rows: the signals, the shared dedup,
// the row gate and order, and the three cache layers. It is pure, so
// callers pass in the data and the clock.
package musicscore

import (
	"encoding/binary"
	"hash/fnv"
	"math"
	"sort"
	"time"
)

// Acts holds one member's and the household's deliberate acts for an artist.
type Acts struct {
	Follows, Asked           bool
	OtherFollows, OtherAsked bool
	HaveAny                  bool
	AlbumsHad                int
}

// Lnorm returns log10(1+x)/log10(1+max), capped at 1. It returns 0 when
// the pool is empty or max is 0.
func Lnorm(x, max int64) float64 {
	if max <= 0 || x <= 0 {
		return 0
	}
	return math.Min(1, math.Log10(1+float64(x))/math.Log10(1+float64(max)))
}

// Popularity returns 1 - (rank-1)/length for an item on a chart of that
// length. It returns 0 for a rank or length under 1.
func Popularity(rank, length int) float64 {
	if length < 1 || rank < 1 || rank > length {
		return 0
	}
	return 1 - float64(rank-1)/float64(length)
}

// Uncharted returns the popularity of an item on no chart, capped at 0.3
// so it never beats a charted one.
func Uncharted(lnorm float64) float64 { return math.Min(0.3, 0.3*lnorm) }

// Freshness returns exp(-days/t) for the whole days since the release,
// counting a future date as day 0. A missing or bad date gives 0.
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

// Affinity returns min(raw, 6)/6, where raw adds 3 for a member follow, 2
// for an ask, 1.5 and 1 for the same acts by others, 1 for owning any
// album and 0.5*log2(1+albums owned).
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

// OwnOnly reports whether the member has any own follow or ask. Without
// one, the household acts carry the affinity.
func OwnOnly(all []Acts) bool {
	for _, x := range all {
		if x.Follows || x.Asked {
			return true
		}
	}
	return false
}

// GenreAffinity returns each genre's weight, the sum of artist affinity
// times vote share. A genre needs at least 2 contributing artists, only
// the top 12 stay, and weights are divided by the largest. aff maps artist
// to affinity and shares maps artist to genre to vote share.
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

// Norm divides each value by the largest. A largest of 0 gives all zeros.
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

// Tie returns a hash of member, date and id, so a tie breaks the same way
// all day.
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
