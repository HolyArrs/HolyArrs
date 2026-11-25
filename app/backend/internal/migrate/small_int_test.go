package migrate

import (
	"math"
	"testing"
)

func TestSmallInt(t *testing.T) {
	tests := []struct {
		name string
		in   int64
		want int
	}{
		{"zero", 0, 0},
		{"normal", 12, 12},
		{"negative", -3, -3},
		{"max int32", math.MaxInt32, math.MaxInt32},
		{"min int32", math.MinInt32, math.MinInt32},
		{"above max int32", math.MaxInt32 + 1, 0},
		{"far above max int32", 1 << 40, 0},
		{"below min int32", math.MinInt32 - 1, 0},
		{"max int64", math.MaxInt64, 0},
	}
	for _, tc := range tests {
		if got := smallInt(tc.in); got != tc.want {
			t.Errorf("%s: smallInt(%d) = %d, want %d", tc.name, tc.in, got, tc.want)
		}
	}
}

func TestEpisodesOffByTitleHugeNumbers(t *testing.T) {
	rows := []map[string]any{{"titleid": int64(1), "seasonnumber": int64(1) << 40, "episodenumber": int64(math.MaxInt32) + 5}}
	eps := episodesOffByTitle(rows)[1]
	if len(eps) != 1 || eps[0].Season != 0 || eps[0].Episode != 0 {
		t.Fatalf("out-of-range numbers should read as 0, got %+v", eps)
	}
}
