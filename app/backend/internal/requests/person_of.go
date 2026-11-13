package requests

import (
	"context"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// PersonOf is the signed-in person as Requests sees them. Without sign-in
// (tests) it is user 1 with every right, named by ownerName (nil: no name).
func PersonOf(r *http.Request, kidMax func(*http.Request) string, ownerName func(context.Context) string) Person {
	who, ok := webserver.From(r.Context())
	if !ok {
		var name string
		if ownerName != nil {
			name = ownerName(r.Context())
		}
		return Person{UserID: 1, Name: name, AutoApprove: true, AutoApproveFourK: true, NoLimit: true}
	}
	p := Person{UserID: who.UserID, Name: who.Name, AutoApprove: who.Has(webserver.PermAutoApprove), AutoApproveFourK: who.Has(webserver.PermAutoApproveFourK), NoLimit: who.Has(webserver.PermApprove)}
	if kidMax != nil {
		p.MaxRating = kidMax(r)
	}
	return p
}
