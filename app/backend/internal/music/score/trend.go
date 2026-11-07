package musicscore

import (
	"sort"
	"time"
)

// Phase 6 (plan 2026-10-22 §2.1 ΔR, §2.6 row 13, §2.7): rank gain over the
// kept chart days, and the Fresh & Trending and Rising formulas.

// RankDays is how many days of chart ranks ΔR needs (the metadata server
// keeps exactly this many; Luis, 2026-10-22).
const RankDays = 7

// RisingYears is how recent an artist's first release must be for Rising.
const RisingYears = 3

// RankGain is ΔR = clamp((rank_7d_ago − rank_today) / L, 0, 1). An item
// not charted 7 days ago (ago ≤ 0) counts as rank L + 1 (a new entry).
// L < 1 or no rank today gives 0.
func RankGain(ago, today, length int) float64 {
	if length < 1 || today < 1 {
		return 0
	}
	if ago <= 0 {
		ago = length + 1
	}
	g := float64(ago-today) / float64(length)
	if g < 0 {
		return 0
	}
	if g > 1 {
		return 1
	}
	return g
}

// Ranked is one chart item's rank today and RankDays ago; Known is false
// when the chart has fewer than RankDays of kept ranks (ΔR undefined).
type Ranked struct {
	Today, Ago int
	Known      bool
}

// RanksReady reports whether ΔR is defined for a whole chart: it has items
// and every item carries a rank from RankDays ago. Otherwise the rows that
// need ΔR stay hidden.
func RanksReady(list []Ranked) bool {
	if len(list) == 0 {
		return false
	}
	for _, r := range list {
		if !r.Known {
			return false
		}
	}
	return true
}

// ScoreTrending is the Fresh & Trending Spotlight and Out this week score:
// 0.6·P + 0.4·F7.
func ScoreTrending(p, f7 float64) float64 { return 0.6*p + 0.4*f7 }

// ScoreRising is row 13: 0.4·G + 0.4·ΔR + 0.2·F60.
func ScoreRising(g, dr, f60 float64) float64 { return 0.4*g + 0.4*dr + 0.2*f60 }

// ScoreComingTrending is the Fresh & Trending Coming soon score:
// 0.7·[A > 0] + 0.3·P.
func ScoreComingTrending(liked bool, p float64) float64 { return 0.7*Bool(liked) + 0.3*p }

// Median is the middle value (mean of the two middle ones); empty → 0.
func Median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]float64{}, v...)
	sort.Float64s(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

// NewArtist reports whether an artist's first release (the earliest of
// the dates given, YYYY-MM-DD or YYYY) is within RisingYears of now. No
// usable date → false (unknown is never "new").
func NewArtist(dates []string, now time.Time) bool {
	first := ""
	for _, d := range dates {
		if len(d) < 4 {
			continue
		}
		if first == "" || d < first {
			first = d
		}
	}
	if first == "" {
		return false
	}
	cut := now.UTC().AddDate(-RisingYears, 0, 0).Format("2006-01-02")
	return first[:min(len(first), 10)] >= cut[:min(len(first), 10)]
}
