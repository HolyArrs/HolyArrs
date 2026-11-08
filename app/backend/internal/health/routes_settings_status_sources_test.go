package health

import (
	"slices"
	"testing"
	"time"
)

func TestSettingsStatusSourceWarnings(t *testing.T) {
	a := SettingsStatus(StatusInput{Sources: []NamedState{{Name: "Down", State: "unreachable"}, {Name: "Slow", State: "rate_limited"}, {Name: "Lan", State: "blocked"}, {Name: "Guarded", State: "bot_check"}, {Name: "Banned", State: "site_ban"}, {Name: "Off", State: "disabled"}, {Name: "Ok", State: "working"}},
		SourcesNear: []string{"The Source Ok is close to its search limit (8 of 10)"}, Now: time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)})
	want := []StatusItem{
		{"copies", "/settings/sources", "The Source Down is not answering", "error"},
		{"copies", "/settings/sources", "The Source Slow asked HolyArrs to slow down, so it waits a while", "warning"},
		{"copies", "/settings/sources", "The Source Lan is blocked from the local network", "warning"},
		{"copies", "/settings/sources", "The Source Guarded is behind a bot check HolyArrs cannot pass", "warning"},
		{"copies", "/settings/sources", "The site behind the Source Banned refuses HolyArrs's address", "warning"},
		{"copies", "/settings/sources", "The Source Ok is close to its search limit (8 of 10)", "warning"},
	}
	if !slices.Equal(a.Items, want) {
		t.Fatalf("got  %+v\nwant %+v", a.Items, want)
	}
}
