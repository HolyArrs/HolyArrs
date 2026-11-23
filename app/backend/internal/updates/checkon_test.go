package updates

import "testing"

// TestCheckOnEnv: HOLYARRS_UPDATE_CHECK=off starts the check off; a saved
// setting still wins.
func TestCheckOnEnv(t *testing.T) {
	t.Setenv("HOLYARRS_UPDATE_CHECK", "")
	if !CheckOn("") || CheckOn("off") || !CheckOn("on") {
		t.Fatal("default must be on, saved values must win")
	}
	for _, v := range []string{"off", "OFF", " false ", "0", "no"} {
		t.Setenv("HOLYARRS_UPDATE_CHECK", v)
		if CheckOn("") {
			t.Fatalf("%q: unsaved check should be off", v)
		}
		if !CheckOn("on") {
			t.Fatalf("%q: saved on must win", v)
		}
	}
	t.Setenv("HOLYARRS_UPDATE_CHECK", "on")
	if !CheckOn("") {
		t.Fatal("on keeps it on")
	}
}
