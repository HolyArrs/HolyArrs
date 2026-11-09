package startup

import "testing"

// The memory target applies only when GOMEMLIMIT is not set.
func TestApplyMemoryTarget(t *testing.T) {
	var got int64
	set := func(n int64) int64 { got = n; return 0 }
	if !applyMemoryTarget(func(string) string { return "" }, set, 384<<20) || got != 384<<20 {
		t.Fatalf("unset GOMEMLIMIT: applied %d", got)
	}
	got = 0
	if applyMemoryTarget(func(k string) string {
		if k == "GOMEMLIMIT" {
			return "1GiB"
		}
		return ""
	}, set, 256<<20) || got != 0 {
		t.Fatalf("GOMEMLIMIT set: still applied %d", got)
	}
}

// Low-memory mode aims at a quarter of the machine, between 192 and 384 MiB.
func TestMemoryTarget(t *testing.T) {
	const mib = 1 << 20
	for _, c := range []struct {
		low   bool
		total uint64
		want  int64
	}{
		{false, 0, 384 * mib},
		{false, 1 << 30, 384 * mib},
		{true, 0, 256 * mib},
		{true, 512 * mib, 192 * mib},
		{true, 1 << 30, 256 * mib},
		{true, 2 << 30, 384 * mib},
		{true, 8 << 30, 384 * mib},
	} {
		if got := memoryTarget(c.low, c.total); got != c.want {
			t.Errorf("memoryTarget(%v, %d) = %d MiB, want %d MiB", c.low, c.total, got/mib, c.want/mib)
		}
	}
}
