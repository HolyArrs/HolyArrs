package metadata

import (
	"testing"
	"time"
)

// TrailerPassTiming sets the first wait and the gap between passes; zero
// or negative keeps the one in use.
func TestTrailerPassTiming(t *testing.T) {
	first, every := trailerPassFirst, trailerPassEvery
	t.Cleanup(func() { trailerPassFirst, trailerPassEvery = first, every })
	if first != 3*time.Minute || every != 5*time.Minute {
		t.Fatalf("defaults = %v and %v, want 3m and 5m", first, every)
	}

	TrailerPassTiming(2*time.Second, 4*time.Second)
	if trailerPassFirst != 2*time.Second || trailerPassEvery != 4*time.Second {
		t.Fatalf("after set: %v and %v", trailerPassFirst, trailerPassEvery)
	}
	TrailerPassTiming(0, -time.Second)
	if trailerPassFirst != 2*time.Second || trailerPassEvery != 4*time.Second {
		t.Fatalf("zero and negative changed them: %v and %v", trailerPassFirst, trailerPassEvery)
	}
	TrailerPassTiming(0, 6*time.Second)
	if trailerPassFirst != 2*time.Second || trailerPassEvery != 6*time.Second {
		t.Fatalf("every alone: %v and %v", trailerPassFirst, trailerPassEvery)
	}
}
