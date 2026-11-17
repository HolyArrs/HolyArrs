package health

import (
	"strings"
	"testing"
	"time"
)

func TestMetadataModeHealthLines(t *testing.T) {
	down := false
	cases := []struct {
		mode, bad, want, severity string
	}{
		{"off", "ful", `"ful"`, "error"},
		{"own", "", "your own keys", "info"},
	}
	for _, c := range cases {
		a := SettingsStatus(StatusInput{MetadataMode: c.mode, MetadataBad: c.bad, MetadataUp: &down, Now: time.Now()})
		var found bool
		for _, it := range a.Items {
			if strings.Contains(it.Text, "Metadata server is down") {
				t.Errorf("%s: no down warning outside server mode: %+v", c.mode, it)
			}
			if strings.Contains(it.Text, c.want) {
				found = it.Severity == c.severity
			}
		}
		if !found {
			t.Errorf("%s %q: want %q as %s, got %+v", c.mode, c.bad, c.want, c.severity, a.Items)
		}
	}
	off := SettingsStatus(StatusInput{MetadataMode: "off", MetadataUp: &down, Now: time.Now()})
	if len(off.Items) != 0 {
		t.Fatalf("off is a choice, nothing needs attention: %+v", off.Items)
	}
	a := SettingsStatus(StatusInput{MetadataMode: "full", MetadataUp: &down, Now: time.Now()})
	if len(a.Items) != 1 || !strings.Contains(a.Items[0].Text, "down") {
		t.Fatalf("server mode keeps the down warning: %+v", a.Items)
	}
}
