package health

import (
	"context"
	"testing"
)

// A Jellyfin or Emby user list that cannot be read is a warning among the
// health checks, and clears when it reads again.
func TestPeopleCheck(t *testing.T) {
	w := newWorld(t)
	lists := map[string]string{}
	w.e.UnreadUserLists = func(context.Context) map[string]string { return lists }
	if _, ok := w.run(t)["people_check:jellyfin"]; ok {
		t.Fatal("never read: no check")
	}
	lists["jellyfin"] = ""
	if c := w.run(t)["people_check:jellyfin"]; c.State != OK || c.Subject != "Jellyfin" {
		t.Fatalf("reads: %+v", c)
	}
	lists["jellyfin"] = "Can't check for people removed on the Jellyfin server: its user list could not be read 3 times in a row."
	if c := w.run(t)["people_check:jellyfin"]; c.State != Warning || c.Kind != "people_check" || c.Message != lists["jellyfin"] {
		t.Fatalf("unread: %+v", c)
	}
	lists["jellyfin"] = ""
	if c := w.run(t)["people_check:jellyfin"]; c.State != OK {
		t.Fatalf("reads again: %+v", c)
	}
}
