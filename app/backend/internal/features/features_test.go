package features

import (
	"slices"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/startup"
)

func TestHeldFeaturesAreOffByDefault(t *testing.T) {
	t.Setenv("HOLYARRS_FLAGS", "")
	for _, n := range startup.HeldFeatures() {
		if startup.FeatureOn(n) {
			t.Errorf("%s is on with no switches set", n)
		}
	}
	if got := startup.FeaturesOff(); !slices.Equal(got, startup.HeldFeatures()) {
		t.Errorf("FeaturesOff() = %v, want every held feature %v", got, startup.HeldFeatures())
	}
}

func TestSwitchTurnsOneFeatureOn(t *testing.T) {
	t.Setenv("HOLYARRS_FLAGS", "music_rows_v2, ai_apps ,go_back")
	if !startup.FeatureOn(AIApps) || !startup.FeatureOn(GoBack) {
		t.Fatal("listed switches are not on")
	}
	if startup.FeatureOn(TraktLists) {
		t.Fatal("an unlisted switch is on")
	}
	if got := startup.FeaturesOff(); slices.Contains(got, AIApps) || !slices.Contains(got, TraktLists) || len(got) != len(startup.HeldFeatures())-2 {
		t.Errorf("FeaturesOff() = %v", got)
	}
}

func TestProvenAlertKinds(t *testing.T) {
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
