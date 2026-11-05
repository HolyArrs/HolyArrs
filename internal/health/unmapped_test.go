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
