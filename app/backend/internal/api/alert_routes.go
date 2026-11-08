package api

import (
	"context"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/alerts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"
)

// AlertView is an Alert as the API shows it.
type AlertView = alerts.View

// alertRoutes mounts the alerts module's routes. Requests owns a person's
// own email, so the module reads it through a func.
func (a *API) alertRoutes(m *http.ServeMux) {
	h := &alerts.Handlers{Store: a.s.Store, Secrets: a.s.Secrets, Sender: a.s.Alerts,
		Caller: func(r *http.Request) (int64, string) {
			p := a.person(r)
			return p.UserID, p.Name
		},
		OwnEmail: func(ctx context.Context, userID int64) (string, error) {
			t, err := requests.ReadTargets(ctx, a.s.Store.Read(), userID)
			return t.Email, err
		}}
	h.Routes(m)
}
