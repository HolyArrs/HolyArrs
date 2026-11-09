package mediafiles

import (
	"fmt"
	"testing"
	"time"
)

// TestRatingCacheEvictsOldest: a full rating cache drops its oldest
// entries, not everything.
func TestRatingCacheEvictsOldest(t *testing.T) {
	var s ratingState
	now := time.Now()
	for i := range maxRatingCache + 10 {
		s.put(fmt.Sprint(i), "G", now)
	}
	if len(s.cache) != maxRatingCache {
		t.Fatalf("cache holds %d, want %d", len(s.cache), maxRatingCache)
	}
	if _, ok := s.get("9", now); ok {
		t.Fatal("oldest entry kept")
	}
	if _, ok := s.get("10", now); !ok {
		t.Fatal("newer entry dropped")
	}
	if _, ok := s.get(fmt.Sprint(maxRatingCache+9), now.Add(ratingTTL)); ok {
		t.Fatal("entry kept past a day")
	}
}
