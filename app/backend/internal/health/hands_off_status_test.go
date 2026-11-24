package health

import (
	"testing"
	"time"
)

// With no Library folder the Library settings count as not set up, unless
// hands-off mode is on: then none is needed.
func TestHandsOffNeedsNoLibraryFolder(t *testing.T) {
	zero := 0
	for _, tc := range []struct {
		handsOff bool
		unset    bool
	}{{false, true}, {true, false}} {
		a := SettingsStatus(StatusInput{Folders: &zero, HandsOff: tc.handsOff, Now: time.Now()})
		if _, got := a.Unset["library"]; got != tc.unset {
			t.Fatalf("hands-off %v: library unset %v", tc.handsOff, got)
		}
	}
}
