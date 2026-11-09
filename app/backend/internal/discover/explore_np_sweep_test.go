package discover

import (
	"testing"
	"time"
)

// A New & Popular page's key carries its day: past days' pages are dropped
// when a new one is kept, not left until 500 have piled up.
func TestNewPagesDropPastDays(t *testing.T) {
	t.Parallel()
	c := &newPageCache{pages: map[string]newPageEntry{
		"old":   {at: time.Now().Add(-newPageTTL - time.Minute)},
		"fresh": {at: time.Now().Add(-time.Minute)},
	}}
	c.put("new", newPage{})
	if _, ok := c.pages["old"]; ok {
		t.Fatal("a past day's page was kept")
	}
	if _, ok := c.pages["fresh"]; !ok {
		t.Fatal("a fresh page was dropped")
	}
	if _, ok := c.get("new"); !ok {
		t.Fatal("the new page is not served")
	}
}
