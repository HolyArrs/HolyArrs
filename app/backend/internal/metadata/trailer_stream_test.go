package metadata

import (
	"testing"
	"time"
)

// A person may start trailerPerMinute music video resolves a minute.
func TestTrailerBudget(t *testing.T) {
	var ts Trailers
	now := time.Now()
	for i := 0; i < trailerPerMinute; i++ {
		if !ts.Allow(7, now) {
			t.Fatalf("refused at %d", i)
		}
	}
	if ts.Allow(7, now) || !ts.Allow(8, now) || !ts.Allow(7, now.Add(61*time.Second)) {
		t.Fatal("budget is per person per minute")
	}
}
