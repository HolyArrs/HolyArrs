package accounts

import (
	"slices"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// 4.2 review: the Downloads, Needs a decision, naming and live routes
// the show downloads work touched stay closed to members.
func TestShowDownloadRoutesClosedToMembers(t *testing.T) {
	member := webserver.DefaultPerms(webserver.RoleMember)
	for _, p := range []string{
		"GET /api/v1/downloads/view", "POST /api/v1/downloads/{id}/remove", "POST /api/v1/downloads/{id}/mark-failed",
		"GET /api/v1/needs-decision", "POST /api/v1/needs-decision/{id}/decide",
		"GET /api/v1/settings/naming", "PUT /api/v1/settings/naming", "POST /api/v1/settings/naming/example",
	} {
		need, ok := RoutePerms[p]
		if !ok || slices.Contains(member, need) {
			t.Errorf("%s: need %q (in table %v), member may use it", p, need, ok)
		}
	}
}
