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

func TestProvenAlertKinds(t *testing.T) {
	RegisterHeldFeatures()
	for _, k := range []string{"discord", "email"} {
		if !ProvenAlertKinds[k] {
			t.Errorf("%s should stay on", k)
		}
	}
	for _, k := range []string{"telegram", "ntfy", "gotify", "pushover", "webhook"} {
		if ProvenAlertKinds[k] {
			t.Errorf("%s should be held back", k)
		}
	}
}
