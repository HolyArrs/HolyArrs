package accounts

import (
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// KidMax is the signed-in person's age-rating limit: "" for no limit, for
// owners and admins, and without sign-in.
func KidMax(r *http.Request) string {
	who, ok := webserver.From(r.Context())
	if !ok || who.Role != webserver.RoleMember {
		return ""
	}
	return who.MaxRating
}
