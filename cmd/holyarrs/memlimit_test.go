package main

import "testing"

// The default memory target applies only when GOMEMLIMIT is not set.
func TestApplyMemoryTarget(t *testing.T) {
	var got int64
	set := func(n int64) int64 { got = n; return 0 }
	if !applyMemoryTarget(func(string) string { return "" }, set) || got != 384<<20 {
		t.Fatalf("unset GOMEMLIMIT: applied %d", got)
	}
	got = 0
	if applyMemoryTarget(func(k string) string {
		if k == "GOMEMLIMIT" {
			return "1GiB"
		}
		return ""
	}, set) || got != 0 {
		t.Fatalf("GOMEMLIMIT set: still applied %d", got)
	}
}
