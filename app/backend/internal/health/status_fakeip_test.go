package health

import (
	"strings"
	"testing"
	"time"
)

func TestFakeIPHealthLine(t *testing.T) {
	down, up := false, true
	a := SettingsStatus(StatusInput{MetadataUp: &down, FakeIPHost: "api.holyarrs.com", MetadataFakeIP: "api.holyarrs.com", Now: time.Now()})
	if len(a.Items) != 1 || !strings.Contains(a.Items[0].Text, "198.18.x.x") || !strings.Contains(a.Items[0].Text, "api.holyarrs.com") || a.Items[0].Severity != "warning" {
		t.Fatalf("down through a fake address names the cause once: %+v", a.Items)
	}
	// Pictures or the update check can hit it while the metadata server answers.
	a = SettingsStatus(StatusInput{MetadataUp: &up, FakeIPHost: "image.tmdb.org", Now: time.Now()})
	if len(a.Items) != 1 || !strings.Contains(a.Items[0].Text, "image.tmdb.org") {
		t.Fatalf("fake address while up: %+v", a.Items)
	}
	// Down for another reason while a picture host hit a fake address: the
	// down line keeps its words and the picture host gets its own line.
	a = SettingsStatus(StatusInput{MetadataUp: &down, FakeIPHost: "image.tmdb.org", Now: time.Now()})
	if len(a.Items) != 2 || strings.Contains(a.Items[0].Text, "198.18") || !strings.Contains(a.Items[1].Text, "image.tmdb.org") {
		t.Fatalf("down for another reason: %+v", a.Items)
	}
	if a := SettingsStatus(StatusInput{MetadataUp: &up, Now: time.Now()}); len(a.Items) != 0 {
		t.Fatalf("no fake address, no line: %+v", a.Items)
	}
}
