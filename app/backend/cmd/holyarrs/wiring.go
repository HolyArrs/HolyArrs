package main

import (
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/api"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/source"
)

// registerHooks hands core the functions that modules own. Each module that
// leaves a leftover package adds its registrations here.
func registerHooks() {
	secrets.RegisterRewriter("sources", source.EncryptPlainExtras)
}

// routes returns the handler that serves every route of the app. Each module
// that leaves the api package adds one registration line here.
func routes(a *api.API, c config) http.Handler {
	rt := webserver.NewRouter()
	a.Routes(rt)
	return webserver.UnderURLBase(c.URLBase, webserver.Compress(limitBodies(a.Chain(rt))))
}
