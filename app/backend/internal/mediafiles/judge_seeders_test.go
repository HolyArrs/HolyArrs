package mediafiles

import "testing"

// The seeders reason says "seeder" for one and "seeders" for any other count.
func TestSeedersLine(t *testing.T) {
	for _, c := range []struct {
		n, min int
		failed bool
		want   string
	}{
		{1, 1, false, "1 seeder, at least 1"},
		{0, 1, true, "0 seeders, needs at least 1"},
		{1, 5, true, "1 seeder, needs at least 5"},
		{12, 5, false, "12 seeders, at least 5"},
	} {
		if got := seedersLine(c.n, c.min, c.failed); got != c.want {
			t.Errorf("seedersLine(%d, %d, %v) = %q, want %q", c.n, c.min, c.failed, got, c.want)
		}
	}
}
