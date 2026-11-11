package startup

import (
	"slices"
	"testing"
)

func TestHeldFeaturesAreOffByDefault(t *testing.T) {
	RegisterHeldFeatures()
	t.Setenv("HOLYARRS_FLAGS", "")
	for _, n := range HeldFeatures() {
		if FeatureOn(n) {
			t.Errorf("%s is on with no switches set", n)
		}
	}
	if got := FeaturesOff(); !slices.Equal(got, HeldFeatures()) {
		t.Errorf("FeaturesOff() = %v, want every held feature %v", got, HeldFeatures())
	}
}

func TestSwitchTurnsOneFeatureOn(t *testing.T) {
	RegisterHeldFeatures()
	t.Setenv("HOLYARRS_FLAGS", "music_rows_v2, ai_apps ,go_back")
	if !FeatureOn(AIApps) || !FeatureOn(GoBack) {
		t.Fatal("listed switches are not on")
	}
	if FeatureOn(TraktLists) {
		t.Fatal("an unlisted switch is on")
	}
	if got := FeaturesOff(); slices.Contains(got, AIApps) || !slices.Contains(got, TraktLists) || len(got) != len(HeldFeatures())-2 {
		t.Errorf("FeaturesOff() = %v", got)
	}
}

// Every Alert channel is proven end to end, so none is held back: the web
// app shows them all with no switch set.
func TestAlertChannelsAreNotHeld(t *testing.T) {
	RegisterHeldFeatures()
	t.Setenv("HOLYARRS_FLAGS", "")
	if slices.Contains(HeldFeatures(), "alert_channels_more") || slices.Contains(FeaturesOff(), "alert_channels_more") {
		t.Fatalf("alert channels are still held back: %v", HeldFeatures())
	}
}
