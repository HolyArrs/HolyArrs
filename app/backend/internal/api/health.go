package api

import (
	"context"
	"net/http"
	"time"

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

// statusHandlers serve the Settings status (health); backups, the update
// check and sign-in reach it as funcs.
func (a *API) statusHandlers() *health.StatusHandlers {
	h := &health.StatusHandlers{Store: a.s.Store, Engine: a.s.Health, States: a.s.States,
		MetadataUp: func(ctx context.Context) (bool, bool) {
			if a.s.MetadataUp == nil {
				return false, false
			}
			return a.s.MetadataUp(ctx), true
		},
		UpdateNewer: func(ctx context.Context) bool { return a.s.Updates.Update(ctx).Newer }, Cache: &a.status}
	if a.s.Parts != nil {
		h.PartStatuses = func() []jobs.Status { return a.s.Parts.Statuses() }
	}
	if a.s.Backups != nil {
		h.NewestBackup = func() (string, bool) {
			bs, err := a.s.Backups.List()
			if err != nil {
				return "", false
			}
			newest := ""
			var at time.Time
			for _, b := range bs {
				if b.CreatedAt.After(at) {
					at = b.CreatedAt
					newest = b.CreatedAt.UTC().Format(time.RFC3339)
				}
			}
			return newest, true
		}
	}
	if a.s.Auth != nil {
		h.SkipLocal, h.OutsideJoin = a.s.Auth.SkipLocal, a.s.Auth.OutsideJoinOpen
	}
	return h
}

// settingsLog writes the settings_changed Events (health).
func (a *API) settingsLog() *health.SettingsLog {
	return &health.SettingsLog{Store: a.s.Store, Who: whoName, Actor: actor}
}

// EventSettingsChanged is one change to settings (health).
const EventSettingsChanged = health.EventSettingsChanged
