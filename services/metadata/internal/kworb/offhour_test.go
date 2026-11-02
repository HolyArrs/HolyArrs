package kworb

import (
	"testing"
	"time"
)

func TestOffHourStepsJustPast(t *testing.T) {
	s := &Service{o: Options{rnd: func() float64 { return 0.99 }}}
	at := time.Date(2026, 10, 31, 5, 0, 30, 0, time.UTC)
	got := s.offHour(at)
	if got.Hour() != 5 || got.Minute() != 1 {
		t.Fatalf("got %v, want within 05:01", got)
	}
	if other := time.Date(2026, 10, 31, 5, 7, 0, 0, time.UTC); !s.offHour(other).Equal(other) {
		t.Fatal("a time off minute 0 moved")
	}
}
