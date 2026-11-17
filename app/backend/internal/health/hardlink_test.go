package health

import (
	"strings"
	"testing"
)

// A Library folder on another mount than the downloads is a warning that
// says what it costs and what to do, not an error.
func TestNoHardlinkSaysWhatToDo(t *testing.T) {
	w := newWorld(t)
	w.exec(t, "UPDATE library_folders SET health = 'no_hardlink'")
	c := w.run(t)["library_folder:1"]
	if c.State != Warning {
		t.Fatalf("state %s, want warning", c.State)
	}
	for _, want := range []string{"copied, not hardlinked", "one disk and one mount"} {
		if !strings.Contains(c.Message, want) {
			t.Fatalf("message %q does not say %q", c.Message, want)
		}
	}
}

// A folder where copying is marked as expected counts as fine.
func TestNoHardlinkExpectedIsFine(t *testing.T) {
	w := newWorld(t)
	w.exec(t, "UPDATE library_folders SET health = 'no_hardlink', copies_expected = 1")
	if c := w.run(t)["library_folder:1"]; c.State != OK {
		t.Fatalf("state %s (%q), want ok", c.State, c.Message)
	}
}
