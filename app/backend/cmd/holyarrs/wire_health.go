package main

import (
	"context"
	"errors"

	"github.com/HolyArrs/holyarrs/app/backend/internal/updates"

	"net/http"
	"strconv"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/health"
	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
)

func (a *handlers) listJobs(w http.ResponseWriter, r *http.Request) {
	js, err := database.Select[domain.Job](r.Context(), a.s.Store.Read(), "jobs", "1=1 ORDER BY id DESC LIMIT ?", webserver.QueryLimit(r, 50))
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	if js == nil {
		js = []domain.Job{}
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"jobs": js})
}

func (a *handlers) health(w http.ResponseWriter, r *http.Request) {
	webserver.WriteJSON(w, http.StatusOK, a.healthView(r.Context()))
}

// healthView is what GET /api/v1/health answers.
func (a *handlers) healthView(ctx context.Context) map[string]any {
	db := a.s.Store.Read()
	ver, err := a.s.Store.Version()
	ok := err == nil
	type part struct {
		Name  string `json:"name"`
		State string `json:"state"`
	}
	var srcs, dls []part
	rows, err := db.QueryContext(ctx, "SELECT name, state FROM sources ORDER BY id")
	if err == nil {
		for rows.Next() {
			var p part
			_ = rows.Scan(&p.Name, &p.State)
			srcs = append(srcs, p)
		}
		rows.Close()
	}
	rows, err = db.QueryContext(ctx, "SELECT name, state FROM downloaders ORDER BY id")
	if err == nil {
		for rows.Next() {
			var p part
			_ = rows.Scan(&p.Name, &p.State)
			dls = append(dls, p)
		}
		rows.Close()
	}
	p95, n := a.s.Store.WriteWaitP95()
	clients := 0
	if a.s.Live != nil {
		clients = a.s.Live.Clients()
	}
	return map[string]any{
		"ok": ok, "version": a.s.Version, "schema": ver, "uptime_s": int(time.Since(a.s.Started).Seconds()),
		"write_wait_p95_ms": float64(p95.Microseconds()) / 1000, "write_wait_samples": n, "writes": a.s.Store.WriteCount(),
		"sources": srcs, "downloaders": dls, "live_clients": clients, "url_base": a.s.URLBase,
		"music_covers": a.mh().PreloadStatus(),
	}
}

func (a *handlers) searchMetadata(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" || len(q) > 200 {
		webserver.Fail(w, http.StatusBadRequest, "send ?q=<name>, and &year=<year> if you know it")
		return
	}
	if a.disc.KidLimited(w, r) {
		return
	}
	year, _ := strconv.Atoi(r.URL.Query().Get("year"))
	ms, err := a.s.Metadata.Search(r.Context(), q, year)
	if library.ServiceError(w, err) {
		return
	}
	if errors.Is(err, metadata.ErrNoAccess) {
		webserver.Fail(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	if ms == nil {
		ms = []metadata.Movie{}
	}
	ms = a.kidMovies(r.Context(), a.kidMax(r), ms)
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"movies": ms, "attribution": metadata.Attribution})
}

// healthHandlers build the health module's routes; downloads, the parts
// and the update check reach it as funcs.
func (a *handlers) healthHandlers() *health.Handlers {
	h := &health.Handlers{Store: a.s.Store, Engine: a.s.Health, States: a.s.States, SafeMode: a.s.SafeMode,
		SyncTrouble: func(ctx context.Context) any { return a.dl.SyncTrouble(ctx) },
		DefaultFeed: updates.DefaultReleaseFeed, ResetUpdate: a.s.Updates.ResetUpdate}
	if a.s.Parts != nil {
		h.PartStatuses = func() []jobs.Status { return a.s.Parts.Statuses() }
	}
	return h
}

// statusHandlers serve the Settings status (health); backups, the update
// check and sign-in reach it as funcs.
func (a *handlers) statusHandlers() *health.StatusHandlers {
	h := &health.StatusHandlers{Store: a.s.Store, Engine: a.s.Health, States: a.s.States,
		MetadataUp: func(ctx context.Context) (bool, bool) {
			if a.s.MetadataUp == nil {
				return false, false
			}
			return a.s.MetadataUp(ctx), true
		},
		MetadataMode: func() (string, string) {
			m := a.s.MetadataModes
			return string(m.Current()), m.BadValue()
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
		h.BackupsOff = a.s.Backups.Off
	}
	if a.s.Auth != nil {
		h.SkipLocal, h.OutsideJoin = a.s.Auth.SkipLocal, a.s.Auth.OutsideJoinOpen
	}
	return h
}

// settingsLog writes the settings_changed Events (health).
func (a *handlers) settingsLog() *health.SettingsLog {
	return &health.SettingsLog{Store: a.s.Store, Who: requests.WhoName, Actor: actor}
}

// scheduleHandlers build the health module's Schedules routes.
func (a *handlers) scheduleHandlers() *health.ScheduleHandlers {
	return &health.ScheduleHandlers{Store: a.s.Store, Queue: a.s.Queue, States: a.s.States, Parts: a.s.Parts, SafeMode: a.s.SafeMode}
}

// appHandlers build the storage, About, log and support bundle routes (health).
func (a *handlers) appHandlers() *health.AppHandlers {
	return &health.AppHandlers{Store: a.s.Store, Version: a.s.Version, Started: a.s.Started, Log: a.s.Log, Secrets: a.s.Secrets,
		ConfigDir: a.s.ConfigDir, SavePath: a.s.SavePath, HealthView: func(ctx context.Context) any { return a.healthView(ctx) }}
}

// uiHandlers build the UI settings routes (health).
func (a *handlers) uiHandlers() *health.UIHandlers {
	h := &health.UIHandlers{Store: a.s.Store, Actor: actor, CountryCount: a.s.CountryCount}
	if a.s.Library != nil && a.s.Library.Artwork != nil {
		h.SetArtworkCap = a.s.Library.Artwork.SetCap
	}
	return h
}

// ApplyCountryCount passes the stored "Count this install" on to the
// requests to the metadata server (at start and after each change).
func (a *handlers) ApplyCountryCount(ctx context.Context) { a.uiHandlers().ApplyCountryCount(ctx) }
