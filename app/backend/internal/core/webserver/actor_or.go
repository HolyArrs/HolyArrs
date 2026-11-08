package webserver

import (
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// ActorOr is the signed-in person; without sign-in (tests) it is def.
func ActorOr(r *http.Request, def database.Actor) database.Actor {
	if _, ok := From(r.Context()); ok {
		return Actor(r.Context())
	}
	return def
}
