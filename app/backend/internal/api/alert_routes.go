package api

import (
	"context"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"
)

// callerID names the signed-in person for the alerts module.
func (a *API) callerID(r *http.Request) (int64, string) {
	p := a.person(r)
	return p.UserID, p.Name
}

// ownEmail reads a person's own email; requests owns it.
func (a *API) ownEmail(ctx context.Context, userID int64) (string, error) {
	t, err := requests.ReadTargets(ctx, a.s.Store.Read(), userID)
	return t.Email, err
}
