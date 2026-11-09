package health

import (
	"context"
	"strings"
	"testing"
)

// Forgot to map /config: the health banner shows an Error until it is
// mapped. Start-up never refuses for it.
func TestUnmappedConfigIsAnError(t *testing.T) {
	w := newWorld(t)
	for _, kind := range []string{"anonymous", "none"} {
		w.e.ConfigUnmapped = kind
		rs, err := w.e.Check(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		var found *Result
		for i := range rs {
			if rs[i].Kind == "config_folder" {
				found = &rs[i]
			}
		}
		if found == nil || found.State != Error || !strings.Contains(found.Message, "/config") {
			t.Fatalf("%s: want an error about /config, got %+v", kind, found)
		}
		if i, j := strings.Index(strings.ToLower(found.Message), "stop"), strings.Index(found.Message, "docker cp"); i < 0 || i > j {
			t.Errorf("%s: should say to stop the container before docker cp: %s", kind, found.Message)
		}
	}
	w.e.ConfigUnmapped = ""
	rs, err := w.e.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rs {
		if r.Kind == "config_folder" && r.State != OK {
			t.Fatalf("mapped /config: %+v", r)
		}
	}
}
