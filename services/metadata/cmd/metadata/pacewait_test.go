package main

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// The pace sums each host's waits; Waits hands them over and starts again.
func TestPaceSumsWaitsPerHost(t *testing.T) {
	p := &hostPace{gap: func(string) time.Duration { return 30 * time.Millisecond }}
	for range 3 {
		r, _ := http.NewRequest("GET", "https://slow.example/x", nil)
		if err := p.wait(r); err != nil {
			t.Fatal(err)
		}
	}
	line := p.waitsLine()
	if !strings.Contains(line, "slow.example=3/") {
		t.Fatalf("line %q", line)
	}
	if p.waitsLine() != "" {
		t.Fatal("waits not reset")
	}
}
