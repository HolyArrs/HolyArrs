package startup

import (
	"slices"
	"testing"
)

func TestRegisterFeatureKeepsOneCopyAndOrder(t *testing.T) {
	saved := heldFeatures
	heldFeatures = nil
	defer func() { heldFeatures = saved }()
	t.Setenv("HOLYARRS_FLAGS", "b")
	RegisterFeature("a")
	RegisterFeature("b")
	RegisterFeature("a")
	if got := HeldFeatures(); !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("HeldFeatures() = %v", got)
	}
	if got := FeaturesOff(); !slices.Equal(got, []string{"a"}) {
		t.Fatalf("FeaturesOff() = %v", got)
	}
}
