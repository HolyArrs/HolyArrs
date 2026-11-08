package api

import (
	"context"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/health"
	"github.com/HolyArrs/holyarrs/app/backend/internal/updates"
)

// PathInfo is one folder or file as the health module shows it.
type PathInfo = health.PathInfo

// AboutView is the About block as the health module shows it.
type AboutView = health.AboutView

// thisAppRoutes mounts the storage, About, log and support bundle routes
// (health) and the update routes (updates).
func (a *API) thisAppRoutes(m *http.ServeMux) {
	a.appHandlers().Routes(m)
	(&updates.Handlers{Checker: a.s.Updates}).Routes(m)
}

func (a *API) appHandlers() *health.AppHandlers {
	return &health.AppHandlers{Store: a.s.Store, Version: a.s.Version, Started: a.s.Started, Log: a.s.Log, Secrets: a.s.Secrets,
		ConfigDir: a.s.ConfigDir, SavePath: a.s.SavePath, HealthView: func(ctx context.Context) any { return a.healthView(ctx) }}
}

// updateView is the update state as the caller may see it.
func (a *API) updateView(r *http.Request, u updates.UpdateInfo) updates.UpdateInfo {
	return updates.ViewFor(r, u)
}

// redactBundle masks secrets in support bundle text.
func redactBundle(s string) string { return health.RedactBundle(s) }
