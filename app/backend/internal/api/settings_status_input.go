package api

import (
	"context"
	"net/http"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/health"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

// settingsStatusRoutes mounts health's settings status route; api gathers
// the other modules' states it looks at.
func (a *API) settingsStatusRoutes(m *http.ServeMux) {
	a.status.Input = a.statusInput
	a.status.Routes(m)
}

// statusInput gathers what the rules look at, including whether the
// metadata server answers.
func (a *API) statusInput(ctx context.Context, now time.Time) (health.StatusInput, error) {
	in := health.StatusInput{Now: now}
	db := a.s.Store.Read()
	checks, err := health.List(ctx, a.s.Store)
	if err != nil {
		return in, err
	}
	in.Checks = checks
	var states []jobs.Status
	if a.s.States != nil {
		states = a.s.States.List()
	}
	if a.s.Parts != nil {
		states = append(states, a.s.Parts.Statuses()...)
	}
	links, err := a.s.Health.Links(ctx)
	if err != nil {
		return in, err
	}
	in.States = append(states, links...)
	var folders int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM library_folders").Scan(&folders); err != nil {
		return in, err
	}
	in.Folders = &folders
	srcs, err := database.Select[domain.Source](ctx, db, "sources", "1=1 ORDER BY priority, name")
	if err != nil {
		return in, err
	}
	in.Sources = []health.NamedState{}
	for _, s := range srcs {
		state := sources.HealthOf(s).State
		// Its own backoff for slowing down or a blocked address is a warning,
		// not a Source that stopped answering.
		if s.State == "paused" && s.PauseReason != nil && (*s.PauseReason == sources.KindRateLimited || *s.PauseReason == sources.KindBlocked) {
			state = *s.PauseReason
		}
		if b := sources.BlockOf(s); b != "" {
			state = b // a site's bot check or ban (#106), not the local network
		}
		in.Sources = append(in.Sources, health.NamedState{Name: s.Name, State: state})
		if s.State != "disabled" {
			if near, text := sources.NearCap(ctx, a.s.Store, s, now); near {
				in.SourcesNear = append(in.SourcesNear, text)
			}
		}
	}
	dls, err := database.Select[domain.Downloader](ctx, db, "downloaders", "removed = 0 ORDER BY priority, name")
	if err != nil {
		return in, err
	}
	in.Downloaders = []health.NamedState{}
	for _, d := range dls {
		in.Downloaders = append(in.Downloaders, health.NamedState{Name: d.Name, State: downloads.HealthOf(d, now).State})
	}
	if a.s.Backups != nil {
		if bs, err := a.s.Backups.List(); err == nil {
			newest := ""
			var at time.Time
			for _, b := range bs {
				if b.CreatedAt.After(at) {
					at = b.CreatedAt
					newest = b.CreatedAt.UTC().Format(time.RFC3339)
				}
			}
			in.NewestBackup = &newest
		}
	}
	if a.s.MetadataUp != nil {
		up := a.s.MetadataUp(ctx)
		in.MetadataUp = &up
	}
	newer := a.s.Updates.Update(ctx).Newer
	in.UpdateNewer = &newer
	gone, err := goneDefaults(ctx, a.s.Store)
	if err != nil {
		return in, err
	}
	in.GoneDefaults, in.DefaultsKnown = gone, true
	var kids int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM users WHERE max_rating != ''").Scan(&kids); err != nil {
		return in, err
	}
	in.Kids = &kids
	if a.s.Auth != nil {
		skip := a.s.Auth.SkipLocal()
		in.SkipLocal = &skip
		join := a.s.Auth.OutsideJoinOpen(ctx)
		in.OutsideJoin = &join
	}
	return in, nil
}

// goneDefaults words each saved Movies and TV default whose folder or Pick
// rules no longer exist (adds then use the built-in choice).
func goneDefaults(ctx context.Context, st *database.Store) ([]string, error) {
	d, saved, err := library.ReadTitleDefaults(ctx, st.Read())
	if err != nil {
		return nil, err
	}
	var out []string
	for _, k := range []struct {
		kind, many, folder string
		kd                 library.KindDefaults
	}{{"movie", "movies", "movie", d.Movie}, {"show", "shows", "show", d.Show}} {
		if saved[k.kind+"_library_folder_id"] && k.kd.LibraryFolderID != 0 && !library.UsableDefault(ctx, st.Read(), "library_folders", k.kind, k.kd.LibraryFolderID) {
			out = append(out, "The Library folder saved for new "+k.many+" is gone, so new "+k.many+" go to the first "+k.folder+" folder")
		}
		if saved[k.kind+"_pick_rules_id"] && k.kd.PickRulesID != 0 && !library.UsableDefault(ctx, st.Read(), "pick_rules", k.kind, k.kd.PickRulesID) {
			out = append(out, "The Pick rules saved for new "+k.many+" are gone, so new "+k.many+" use their folder's own Pick rules")
		}
	}
	return out, nil
}
