package providers

import (
	"context"
	"testing"
	"time"
)

func TestPaceSharesTheInterval(t *testing.T) {
	c, err := New(Options{Providers: map[string]Config{"musicbrainz": {Interval: 100 * time.Millisecond}}, Timeout: time.Second, Attempts: 1, UserAgent: "test"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	start := time.Now()
	for range 3 {
		if err := c.Pace(ctx, "musicbrainz"); err != nil {
			t.Fatal(err)
		}
	}
	if d := time.Since(start); d < 190*time.Millisecond {
		t.Fatalf("three paced calls in %s", d)
	}
	if err := c.Pace(ctx, "nope"); err != nil {
		t.Fatal(err)
	}
}
