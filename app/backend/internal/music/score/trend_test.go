package musicscore

import (
	"math"
	"testing"
	"time"
)

func TestRankGain_Clamps(t *testing.T) {
	for _, c := range []struct {
		ago, today, l int
		want          float64
	}{
		{10, 4, 20, 0.3},
		{4, 10, 20, 0},
		{5, 5, 20, 0},
		{0, 1, 20, 1},
		{0, 11, 20, 0.5},
		{-1, 20, 20, 0.05},
		{3, 1, 0, 0},
		{3, 0, 10, 0},
	} {
		if got := RankGain(c.ago, c.today, c.l); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("RankGain(%d, %d, %d) = %v, want %v", c.ago, c.today, c.l, got, c.want)
		}
	}
}

func TestRanksReady_Chart(t *testing.T) {
	if RanksReady(nil) {
		t.Fatal("an empty chart has no ΔR")
	}
	if RanksReady([]Ranked{{1, 2, true}, {2, 0, false}}) {
		t.Fatal("one item without a 7-day-old rank hides the row")
	}
	if !RanksReady([]Ranked{{1, 2, true}, {2, 0, true}}) {
		t.Fatal("a full 7-day history (new entries included) is ready")
	}
}

func TestTrendScores_ExactValues(t *testing.T) {
	near := func(a, b float64) bool { return math.Abs(a-b) < 1e-9 }
	if !near(ScoreTrending(0.5, 1), 0.7) {
		t.Fatal("trending")
	}
	if !near(ScoreRising(0.5, 0.25, 1), 0.5) {
		t.Fatal("rising")
	}
	if !near(ScoreComingTrending(true, 0.5), 0.85) || !near(ScoreComingTrending(false, 0.5), 0.15) {
		t.Fatal("coming")
	}
	if Median(nil) != 0 || Median([]float64{3, 1, 2}) != 2 || Median([]float64{4, 1, 2, 3}) != 2.5 {
		t.Fatal("median")
	}
}

func TestNewArtist_Window(t *testing.T) {
	now := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	if !NewArtist([]string{"2025-01-01", "2024-03-01"}, now) {
		t.Fatal("first release 2024 is within 3 years")
	}
	if NewArtist([]string{"2025-01-01", "2019-03-01"}, now) {
		t.Fatal("an older first release is not new")
	}
	if NewArtist([]string{"", "x"}, now) || NewArtist(nil, now) {
		t.Fatal("unknown dates are never new")
	}
	if !NewArtist([]string{"2024"}, now) || NewArtist([]string{"2022"}, now) {
		t.Fatal("year-only dates compare by year")
	}
}
