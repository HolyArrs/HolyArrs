package database

import (
	"context"
	"testing"
)

func TestEventKeepsViaPlugin(t *testing.T) {
	s, err := Open(t.TempDir(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, c := range []struct{ plugin string }{{""}, {"trakt"}} {
		ctx := WithViaPlugin(context.Background(), c.plugin)
		e := &Event{Kind: "title_added", Summary: "x"}
		if err := s.AddEvent(ctx, e); err != nil {
			t.Fatal(err)
		}
		var got *string
		if err := s.ReadEvents().QueryRow("SELECT via_plugin FROM events WHERE id = ?", e.ID).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if (got == nil) != (c.plugin == "") || got != nil && *got != c.plugin {
			t.Errorf("%q: stored %v", c.plugin, got)
		}
	}
}
