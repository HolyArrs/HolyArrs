package mediafiles

import (
	"testing"
	"time"
)

func TestStallWaitGrowsToSevenDays(t *testing.T) {
	day := 24 * time.Hour
	for n, want := range map[int]time.Duration{0: 0, 1: day, 2: 2 * day, 3: 4 * day, 4: 7 * day, 9: 7 * day} {
		if got := StallWait(n); got != want {
			t.Fatalf("StallWait(%d) = %v, want %v", n, got, want)
		}
	}
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if got := (Stall{Count: 2, Last: at}).Until(); !got.Equal(at.Add(2 * day)) {
		t.Fatalf("until %v", got)
	}
	if !(Stall{}).Until().IsZero() {
		t.Fatal("no stall waits")
	}
}
