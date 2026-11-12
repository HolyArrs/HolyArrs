package health

import (
	"context"
	"slices"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// With the Sources part off, Health says nothing about Sources: a Source that
// is down raises no warning, one already shown goes quietly (no "fixed"
// Alert), and the Downloader checks carry on.
func TestSourcesOffDropsTheSourceChecks(t *testing.T) {
	w := newWorld(t)
	on := true
	w.e.SourcesOn = func() bool { return on }
	w.addDownloader(t, "SABnzbd", "unreachable")
	w.exec(t, `INSERT INTO sources (created_at, created_by, updated_at, updated_by, name, protocol, format, url, use_for, kinds, categories, limits, priority, state, pause_reason, extra)
		VALUES ('2026-10-01T00:00:00Z', '{}', '2026-10-01T00:00:00Z', '{}', 'Lab', 'torrent', 'torznab', 'http://x/api', '{}', '[]', '{}', '{}', 25, 'paused', 'down', '{}')`)
	w.run(t)
	if c := w.run(t)["source:1"]; c.Confirmed != Warning {
		t.Fatalf("Source down with the part on: %+v", c)
	}
	on = false
	cs := w.run(t)
	if c, ok := cs["source:1"]; ok {
		t.Fatalf("a Source check with the part off: %+v", c)
	}
	if c := cs["downloader:1"]; c.State != Warning {
		t.Fatalf("the Downloader check stopped too: %+v", c)
	}
	for _, e := range w.events(t) {
		if e.Kind == domain.EventHealthRecovered && e.Data["check"] == "source:1" {
			t.Fatalf("turning Sources off sent a fixed Alert: %+v", e)
		}
	}
	// The linked apps show the Source as off, which is not a problem.
	ctx := context.Background()
	links, err := w.e.Links(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range links {
		if l.Kind == "source" && (l.State != jobs.Off || l.Reason != SourcesOffReason) {
			t.Fatalf("Source link with the part off: %+v", l)
		}
	}
	// Settings asks for no Source fix, and none to be added.
	st := &StatusHandlers{Store: w.s, Engine: w.e}
	for _, drop := range []bool{false, true} {
		if drop {
			w.exec(t, "DELETE FROM sources")
		}
		in, err := st.statusInput(ctx, w.now)
		if err != nil {
			t.Fatal(err)
		}
		a := SettingsStatus(in)
		for _, i := range a.Items {
			if i.Page == "/settings/sources" {
				t.Fatalf("Settings asks to fix a Source with the part off: %+v", i)
			}
		}
		if slices.Contains(a.Unset["copies"], "/settings/sources") {
			t.Fatalf("Settings asks to add a Source with the part off: %+v", a.Unset)
		}
	}
}
