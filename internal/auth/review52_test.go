package auth

import (
	"slices"
	"testing"
)

// 5.2 review: the routes 5.2 added or changed, and the live stream that
// carries their Events, stay closed to members.
func TestShowOrganiseRoutesClosedToMembers(t *testing.T) {
	member := DefaultPerms(RoleMember)
	for _, p := range []string{
		"GET /api/v1/settings/alerts", "PUT /api/v1/settings/alerts", "POST /api/v1/titles/{id}/delete",
		"POST /api/v1/library-folders/{id}/scan", "POST /api/v1/library-folders/{id}/adopt", "GET /api/v1/events",
		"GET /live",
	} {
		need, ok := RoutePerms[p]
		if !ok || slices.Contains(member, need) || need == NeedPublic {
			t.Errorf("%s: need %q (in table %v), member may use it", p, need, ok)
		}
	}
	if RoutePerms["GET /live"] != PermManageLibrary {
		t.Errorf("live stream needs %q, want manage library", RoutePerms["GET /live"])
	}
}

// The Calendar is "what's coming for me": every signed-in person may read it
// (Kid limits still narrow what it returns); its feed key stays closed.
func TestCalendarOpenToSignedIn(t *testing.T) {
	if RoutePerms["GET /api/v1/calendar"] != needSignedIn {
		t.Errorf("calendar needs %q, want signed in", RoutePerms["GET /api/v1/calendar"])
	}
}
