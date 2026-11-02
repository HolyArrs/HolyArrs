package api

import (
	"slices"
	"testing"
)

func TestSettingsStatusSourceWarnings(t *testing.T) {
	a := settingsStatus(statusInput{Sources: []namedState{{"Down", "unreachable"}, {"Slow", "rate_limited"}, {"Lan", "blocked"}, {"Off", "disabled"}, {"Ok", "working"}},
		SourcesNear: []string{"The Source Ok is close to its search limit (8 of 10)"}, Now: statusNow})
	want := []statusItem{
		{"copies", "/settings/sources", "The Source Down is not answering", "error"},
		{"copies", "/settings/sources", "The Source Slow asked HolyArrs to slow down, so it waits a while", "warning"},
		{"copies", "/settings/sources", "The Source Lan is blocked from the local network", "warning"},
		{"copies", "/settings/sources", "The Source Ok is close to its search limit (8 of 10)", "warning"},
	}
	if !slices.Equal(a.Items, want) {
		t.Fatalf("got  %+v\nwant %+v", a.Items, want)
	}
}
