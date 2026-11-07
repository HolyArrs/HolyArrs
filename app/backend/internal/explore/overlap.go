// Package explore builds the Explore page per member: seeds from the
// Library and asks, affinity, scored candidate rows, the fixed head, a
// generated tail whose length grows with signal, and the dedup ledger.
// Everything here is pure; fetching and storage live in the api package.
package explore

import "math"

// Outcomes of the Best new releases overlap rule.
const (
	BestKeep    = "keep"
	BestReframe = "reframe"
)

// TopOverlap is the share of b's top n that is also in a's top n.
func TopOverlap(a, b []string, n int) float64 {
	a, b = a[:min(n, len(a))], b[:min(n, len(b))]
	if len(b) == 0 {
		return 0
	}
	in := map[string]bool{}
	for _, id := range a {
		in[id] = true
	}
	shared := 0
	for _, id := range b {
		if in[id] {
			shared++
		}
	}
	return math.Round(100*float64(shared)/float64(len(b))) / 100
}

// BestDecision applies the plan's rule: 50% or more for movies or shows
// reframes (or drops) the row; under 50% for both keeps it.
func BestDecision(movie, show float64) string {
	if movie >= 0.5 || show >= 0.5 {
		return BestReframe
	}
	return BestKeep
}
