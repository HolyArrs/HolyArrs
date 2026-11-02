package api

import (
	"slices"
	"strings"
	"testing"
)

func TestEdgeRuleMatchesPolicies(t *testing.T) {
	r, err := CommittedEdgeRule()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(r.Paths, EdgePrefixes()) {
		t.Fatalf("edgerule.json paths drifted from edgePolicies:\n rule %v\n code %v", r.Paths, EdgePrefixes())
	}
	if strings.Contains(string(r.ActionParameters), "cache_key") {
		t.Fatal("custom cache key is rejected on the free plan")
	}
	if !strings.HasPrefix(r.Expression(), `(http.host eq "holyarrs-api.coderluii.dev" and (starts_with(`) {
		t.Fatal(r.Expression())
	}
}
