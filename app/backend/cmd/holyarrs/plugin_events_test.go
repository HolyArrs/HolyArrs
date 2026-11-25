package main

import (
	"slices"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/alerts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/plugins"
)

// Every staff Library moment has a plugin event, every plugin event comes
// from a moment or a plugin-only Event, and no Alerts moment about people
// who ask is passed on (requests reach plugins without names instead). A
// plugin's own message to the owner never goes to plugins, so plugins can't
// set each other off.
func TestPluginEventsMatchMoments(t *testing.T) {
	people := []string{alerts.MomentWaiting, alerts.MomentApproved, alerts.MomentTurnedDown, alerts.MomentReady, alerts.MomentNewEpisodes,
		alerts.MomentNewAlbums, alerts.MomentReported, alerts.MomentFixed, alerts.MomentTest, alerts.MomentPlugin}
	var got []string
	for _, m := range alerts.Moments {
		ev := alerts.PluginMomentEvents[m]
		if slices.Contains(people, m) {
			if ev != "" {
				t.Fatalf("%s is passed to plugins", m)
			}
			continue
		}
		if ev == "" {
			t.Fatalf("staff moment %s has no plugin event", m)
		}
		got = append(got, ev)
	}
	for _, ev := range alerts.PluginOnlyEvents {
		if !slices.Contains(got, ev) {
			got = append(got, ev)
		}
	}
	slices.Sort(got)
	want := slices.Sorted(slices.Values(plugins.EventNames))
	if !slices.Equal(got, want) {
		t.Fatalf("events %v, want %v", got, want)
	}
	tellPlugins(nil, alerts.Message{Moment: alerts.MomentAdd}) // before the plugins start: nothing
}
