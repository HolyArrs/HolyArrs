package sources

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
)

// Prowlarr only gets the arr sides whose part is switched on.
func TestProwlarrSidesFollowParts(t *testing.T) {
	h := &Handlers{PartOn: func(p parts.Part) bool { return p == parts.Movies }}
	s := h.prowlarrSides("http://h:6262")
	if len(s) != 1 || s[0].Kind != "Radarr" || s[0].Base != "http://h:6262/radarr" {
		t.Fatal(s)
	}
	h.PartOn = func(p parts.Part) bool { return p == parts.Shows }
	s = h.prowlarrSides("http://h:6262")
	if len(s) != 1 || s[0].Kind != "Sonarr" || s[0].Base != "http://h:6262/sonarr" {
		t.Fatal(s)
	}
	h.PartOn = func(parts.Part) bool { return true }
	if len(h.prowlarrSides("x")) != 2 {
		t.Fatal("want both sides")
	}
	h.PartOn = func(parts.Part) bool { return false }
	if len(h.prowlarrSides("x")) != 0 {
		t.Fatal("want no sides")
	}
}
