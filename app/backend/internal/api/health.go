package api

import (
	"context"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/health"
	"github.com/HolyArrs/holyarrs/app/backend/internal/updates"
)

// DiskView is one library folder's disk as the health module shows it.
type DiskView = health.DiskView

// keyKitForOthers is the key kit warning for people who are not the owner.
const keyKitForOthers = health.KeyKitForOthers

// notOwner says whether the caller is signed in and is not the owner.
func notOwner(r *http.Request) bool { return health.NotOwner(r) }

// healthRoutes mounts the health module's routes; downloads, the parts and
// the update check reach it as funcs.
func (a *API) healthRoutes(m *http.ServeMux) {
	h := &health.Handlers{Store: a.s.Store, Engine: a.s.Health, States: a.s.States, SafeMode: a.s.SafeMode,
		SyncTrouble: func(ctx context.Context) any { return a.syncTrouble(ctx) },
		DefaultFeed: updates.DefaultReleaseFeed, ResetUpdate: a.s.Updates.ResetUpdate}
	if a.s.Parts != nil {
		h.PartStatuses = func() []jobs.Status { return a.s.Parts.Statuses() }
	}
	h.Routes(m)
}
