package main

import (
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/api"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// routes returns the handler that serves every route of the app. Each module
// that leaves the api package adds one registration line here.
func routes(a *api.API, c config) http.Handler {
	return webserver.UnderURLBase(c.URLBase, webserver.Compress(limitBodies(a.Handler())))
}
