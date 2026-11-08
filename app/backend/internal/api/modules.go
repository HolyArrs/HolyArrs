package api

import (
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/migrate"
	"github.com/HolyArrs/holyarrs/app/backend/internal/parts"
)

// ModuleRoutes registers the routes of the modules that have left this
// package. They need services this package builds, so it hands them over.
func (a *API) ModuleRoutes(rt *webserver.Router) {
	a.migrateHandlers().Routes(rt)
}

func (a *API) migrateHandlers() *migrate.Handlers {
	return &migrate.Handlers{Store: a.s.Store, Secrets: a.s.Secrets, Mover: a.mover, SafeMode: a.safeMode}
}

// mover is the move runner (made on first use in tests).
func (a *API) mover() *migrate.Mover {
	if a.s.Mover == nil {
		a.s.Mover = &migrate.Mover{Store: a.s.Store, Library: a.s.Library, Queue: a.s.Queue}
	}
	if a.s.Mover.PartOn == nil {
		a.s.Mover.PartOn = func(p string) bool { return !a.safeMode() && a.partIsOn(parts.Part(p)) }
	}
	return a.s.Mover
}

func (a *API) safeMode() bool { return a.s.SafeMode || (a.s.Parts != nil && a.s.Parts.SafeMode) }
