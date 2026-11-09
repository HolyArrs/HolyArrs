package musicscore

import (
	"sort"
	"time"
)

// RankDays is how many days of chart ranks the rank gain needs.
const RankDays = 7

// RisingYears is how recent an artist's first release must be to count as
// rising.
const RisingYears = 3

// Ranked holds one chart item's rank today and RankDays ago. Known is
// false when the chart keeps fewer than RankDays of ranks.
type Ranked struct {
	Today, Ago int
	Known      bool
}

// RankGain returns clamp((ago - today) / length, 0, 1). An item that was
// not charted then (ago <= 0) counts as a new entry just below the chart.
// It returns 0 when length < 1 or there is no rank today.
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

// RanksReady reports whether the rank gain is defined for a whole chart:
// it has items and each one has a rank from RankDays ago. Rows that need
// the gain stay hidden otherwise.
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

// ScoreTrending scores the Fresh & Trending Spotlight and Out this week
// rows as 0.6*p + 0.4*f7.
func ScoreTrending(p, f7 float64) float64 { return 0.6*p + 0.4*f7 }

// ScoreRising scores the Rising row as 0.4*g + 0.4*dr + 0.2*f60.
func ScoreRising(g, dr, f60 float64) float64 { return 0.4*g + 0.4*dr + 0.2*f60 }

// ScoreComingTrending scores the Fresh & Trending Coming soon row as
// 0.7 for a liked artist plus 0.3*p.
func ScoreComingTrending(liked bool, p float64) float64 { return 0.7*Bool(liked) + 0.3*p }

// Median returns the middle value, or the mean of the two middle values.
// It returns 0 for an empty list.
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

// NewArtist reports whether the earliest of dates (YYYY-MM-DD or YYYY) is
// within RisingYears of now. With no usable date it is false, since an
// unknown artist is never counted as new.
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
