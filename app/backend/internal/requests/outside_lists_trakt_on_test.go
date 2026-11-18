package requests

import (
	"testing"
)

// Trakt lists are offered with no switch set.
func TestNormalListRefTraktWithoutSwitch(t *testing.T) {
	t.Setenv("HOLYARRS_FLAGS", "")
	got, err := NormalListRef(ListTrakt, "https://trakt.tv/users/sean/lists/good-films")
	if err != nil || got != "sean/good-films" {
		t.Fatalf("NormalListRef = %q, %v", got, err)
	}
}
