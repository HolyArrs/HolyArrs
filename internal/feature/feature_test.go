package feature

import (
	"slices"
	"testing"
)

func TestHeldFeaturesAreOffByDefault(t *testing.T) {
	t.Setenv("HOLYARRS_FLAGS", "")
	for _, n := range Held {
		if On(n) {
			t.Errorf("%s is on with no switches set", n)
		}
	}
	if got := Off(); !slices.Equal(got, Held) {
		t.Errorf("Off() = %v, want every held feature %v", got, Held)
	}
}

func TestSwitchTurnsOneFeatureOn(t *testing.T) {
	t.Setenv("HOLYARRS_FLAGS", "music_rows_v2, ai_apps ,go_back")
	if !On(AIApps) || !On(GoBack) {
		t.Fatal("listed switches are not on")
	}
	if On(TraktLists) {
		t.Fatal("an unlisted switch is on")
	}
	if got := Off(); slices.Contains(got, AIApps) || !slices.Contains(got, TraktLists) || len(got) != len(Held)-2 {
		t.Errorf("Off() = %v", got)
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
