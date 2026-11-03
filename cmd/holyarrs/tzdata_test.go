package main

import (
	"testing"
	"time"
)

// Zones load from the built-in database even without a system one
// (ZONEINFO pointed at an empty folder hides the system's).
func TestTimeZonesBuiltIn(t *testing.T) {
	t.Setenv("ZONEINFO", t.TempDir())
	for _, name := range []string{"America/New_York", "Europe/Berlin", "Asia/Tokyo"} {
		if _, err := time.LoadLocation(name); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}
