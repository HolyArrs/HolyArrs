package accounts

import (
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/startup"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// Routes adds the sign-in routes and the sign-in page.
func (a *Auth) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /login", webserver.Page)
	m.HandleFunc("GET /api/v1/auth/state", a.state)
	m.HandleFunc("POST /api/v1/auth/setup", a.setup)
	m.HandleFunc("POST /api/v1/auth/login", a.login)
	m.HandleFunc("POST /api/v1/auth/logout", a.logout)
	m.HandleFunc("POST /api/v1/auth/reauth", a.reauth)
	m.HandleFunc("POST /api/v1/auth/setup/done", a.setupDone)
	m.HandleFunc("GET /api/v1/auth/me", a.me)
	m.HandleFunc("GET /api/v1/auth/keys", a.listKeys)
	m.HandleFunc("POST /api/v1/auth/keys", a.makeKey)
	m.HandleFunc("DELETE /api/v1/auth/keys/{id}", a.deleteKey)
	m.HandleFunc("GET /api/v1/auth/settings", a.getSettings)
	m.HandleFunc("PUT /api/v1/auth/settings", a.putSettings)
	a.twoFactorRoutes(m)
	a.userRoutes(m)
	a.avatarRoutes(m)
	a.profileRoutes(m)
	a.savedViewRoutes(m)
	a.sessionRoutes(m)
	a.outsideRoutes(m)
	a.mediaServerPeopleRoutes(m)
	a.plexMineRoutes(m)
	a.ssoRoutes(m)
	if startup.FeatureOn(startup.AIApps) {
		a.aiOAuthRoutes(m)
	}
}
