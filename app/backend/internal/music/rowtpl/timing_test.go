package rowtpl

import (
	"os"
	"testing"
	"time"
)

// TestPageCounts_Timing reports how long counting every Browse page takes on a real bundle (ROWTPL_BUNDLE). It skips without one.
func TestPageCounts_Timing(t *testing.T) {
	path := os.Getenv("ROWTPL_BUNDLE")
	if path == "" {
		t.Skip("set ROWTPL_BUNDLE")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	b, err := ParseBundle(raw)
	if err != nil {
		t.Fatal(err)
	}
	parsed := time.Since(start)
	start = time.Now()
	PageCounts(b, time.Now(), "us", nil, BrowseKeys())
	counted := time.Since(start)
	start = time.Now()
	c := &Ctx{B: b, Kind: "genre", Key: "genre:pop", Region: "us", Now: time.Now(), Member: 1}
	Spotlight(c)
	Build(c)
	one := time.Since(start)
	t.Logf("parse %v, count 61 pages %v, one genre page %v", parsed, counted, one)
	if one > 2*time.Second {
		t.Fatalf("one page took %v", one)
	}
}
