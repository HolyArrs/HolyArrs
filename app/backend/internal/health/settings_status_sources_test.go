package health

import (
	"slices"
	"testing"
	"time"
)

func TestSettingsStatusSourceWarnings(t *testing.T) {
	t.Parallel()
	a := SettingsStatus(StatusInput{Sources: []NamedState{{"Down", "unreachable"}, {"Slow", "rate_limited"}, {"Lan", "blocked"}, {"Guarded", "bot_check"}, {"Banned", "site_ban"}, {"Off", "disabled"}, {"Ok", "working"}},
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

// Titles no Source would search for because of tags get one warning on the Sources page.
func TestSettingsStatusStrandedTitles(t *testing.T) {
	t.Parallel()
	line := "2 titles have no Source that searches for them: every Source they could use has a tag they do not share, or their Pick rules leave the Source out"
	a := SettingsStatus(StatusInput{Sources: []NamedState{{"Toloka", "working"}}, SourcesStranded: line, Now: time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)})
	want := []StatusItem{{"copies", "/settings/sources", line, "warning"}}
	if !slices.Equal(a.Items, want) {
		t.Fatalf("got  %+v\nwant %+v", a.Items, want)
	}
	// With the Sources part off there are no Sources to read, and nothing is said.
	if a := SettingsStatus(StatusInput{SourcesStranded: line, Now: time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)}); slices.ContainsFunc(a.Items, func(i StatusItem) bool { return i.Text == line }) {
		t.Fatalf("said with no Sources: %+v", a.Items)
	}
}
