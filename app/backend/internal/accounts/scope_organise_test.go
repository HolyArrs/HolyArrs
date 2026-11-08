package accounts

import (
	"slices"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// 5.2 review: the routes 5.2 added or changed stay closed to members. The
// live stream is open to everyone signed in since the hub filters each
// event by audience (members receive only their own items).
func TestShowOrganiseRoutesClosedToMembers(t *testing.T) {
	member := webserver.DefaultPerms(webserver.RoleMember)
	for _, p := range []string{
		"GET /api/v1/settings/alerts", "PUT /api/v1/settings/alerts", "POST /api/v1/titles/{id}/delete",
		"POST /api/v1/library-folders/{id}/scan", "POST /api/v1/library-folders/{id}/adopt", "GET /api/v1/events",
	} {
		need, ok := RoutePerms[p]
		if !ok || slices.Contains(member, need) || need == webserver.NeedPublic {
			t.Errorf("%s: need %q (in table %v), member may use it", p, need, ok)
		}
	}
	if RoutePerms["GET /live"] != webserver.NeedSignedIn || RoutePerms["POST /api/v1/live/interest"] != webserver.NeedSignedIn {
		t.Errorf("live stream needs %q, interest %q, want signed in", RoutePerms["GET /live"], RoutePerms["POST /api/v1/live/interest"])
	}
}

// The Calendar is "what's coming for me": every signed-in person may read it
// (Kid limits still narrow what it returns); its feed key stays closed.
func TestCalendarOpenToSignedIn(t *testing.T) {
	if RoutePerms["GET /api/v1/calendar"] != webserver.NeedSignedIn {
		t.Errorf("calendar needs %q, want signed in", RoutePerms["GET /api/v1/calendar"])
	}
}
