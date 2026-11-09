package suggest

import "math"

// Outcomes of the Best new releases overlap rule.
const (
	BestKeep    = "keep"
	BestReframe = "reframe"
)

// TopOverlap returns the share of b's top n that is also in a's top n,
// rounded to two decimals.
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

// BestDecision returns BestReframe when half or more of either list overlaps,
// and BestKeep when both are under half.
func BestDecision(movie, show float64) string {
	if movie >= 0.5 || show >= 0.5 {
		return BestReframe
	}
	return BestKeep
}
