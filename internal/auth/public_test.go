package auth

import "testing"

// 5.1.7 minor 10: the iCal feed needs no sign-in (it checks its own feed
// key), so the table marks it public instead of signed in.
func TestFeedRouteIsMarkedPublic(t *testing.T) {
	for _, r := range []string{"GET /api/v1/calendar/feed.ics"} {
		if got := RoutePerms[r]; got != NeedPublic {
			t.Fatalf("%s needs %q, want %q", r, got, NeedPublic)
		}
	}
	if !(Who{Role: RoleMember}).Has(NeedPublic) {
		t.Fatal("a public route refused a signed-in person")
	}
}
