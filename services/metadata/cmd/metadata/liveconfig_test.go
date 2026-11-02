package main

import (
	"path/filepath"
	"testing"
)

// The config the VM runs reads cleanly: TMDB 6 at once 50 ms apart, TVmaze
// at 1000 ms, the raised bursts.
func TestLiveConfigReads(t *testing.T) {
	c, err := readConfig(filepath.Join("..", "..", "..", "..", "deploy", "metadata", "live", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if p := c.Providers["tmdb"]; p.IntervalMS != 50 || p.Concurrency != 6 {
		t.Fatalf("tmdb %+v", p)
	}
	if p, ok := c.Providers["tvmaze"]; !ok || p.IntervalMS != 1000 {
		t.Fatalf("tvmaze %+v", p)
	}
	if c.Burst != 200 || c.ClientAuthBurst != 300 || c.ClientAuthPerMinute != 480 {
		t.Fatalf("limits %d %d %v", c.Burst, c.ClientAuthBurst, c.ClientAuthPerMinute)
	}
}
