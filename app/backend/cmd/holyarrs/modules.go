package main

import (
	"context"
	"net/http"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/arrapi"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/startup"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/migrate"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

// ModuleRoutes registers the routes of the modules that have left this
// package. They need services this package builds, so it hands them over.
func (a *handlers) ModuleRoutes(rt *webserver.Router) {
	a.sourceHandlers().Routes(rt)
	a.migrateHandlers().Routes(rt)
	a.arr().Routes(rt)
	if startup.FeatureOn(startup.AIApps) {
		a.ai.Routes(rt)
	}
}

// sourceHandlers is the Source, search and bridge routes; one value serves
// both the routes and the callers here that publish a changed Source.
func (a *handlers) sourceHandlers() *sources.Handlers {
	a.sourceOnce.Do(func() {
		h := &sources.Handlers{Store: a.s.Store, Vault: a.s.Secrets, Search: a.searcher, Bridges: a.s.Bridges, Queue: a.s.Queue,
			Catalog: libraryCatalog{a}, OldApps: oldApps{a}, PartOn: a.partIsOn}
		if a.s.Live != nil {
			h.Live = a.s.Live
		}
		a.sourceH = h
	})
	return a.sourceH
}

func (a *handlers) searcher() *sources.Searcher { return a.s.Search }

// publishSource tells open screens that a Source changed.
func (a *handlers) publishSource(x domain.Source) { a.sourceHandlers().PublishSource(x) }

// libraryCatalog answers the sources routes' questions about Titles, Slots
// and Parts.
type libraryCatalog struct{ a *handlers }

func (c libraryCatalog) TitleFromPath(w http.ResponseWriter, r *http.Request) (mediafiles.Title, bool) {
	return c.a.titleID(w, r)
}

func (c libraryCatalog) EpisodeLabels(ctx context.Context, slotIDs []int64) string {
	return c.a.shows.EpisodeLabels(ctx, slotIDs)
}

func (c libraryCatalog) AlbumLabels(ctx context.Context, slotIDs []int64) string {
	return c.a.albumLabels(ctx, slotIDs)
}

func (c libraryCatalog) PartLabel(ctx context.Context, partID int64) (int64, string, string, string) {
	return c.a.partLabel(ctx, partID)
}

// oldApps is the arr key and switch side of connecting Prowlarr.
type oldApps struct{ a *handlers }

func (o oldApps) CanMakeKeys() bool { return o.a.s.Auth != nil }

func (o oldApps) KeySides(ctx context.Context) (map[string]bool, error) {
	out := map[string]bool{}
	if o.a.s.Auth == nil {
		return out, nil
	}
	ks, err := o.a.s.Auth.ArrKeys(ctx)
	if err != nil {
		return nil, err
	}
	for _, k := range ks {
		out[k.Side] = true
	}
	return out, nil
}

func (o oldApps) MakeKey(r *http.Request, side string) (string, error) {
	return o.a.s.Auth.MakeArrKey(r, side)
}

func (o oldApps) DeleteKey(r *http.Request, side string) error {
	_, err := o.a.s.Auth.DeleteArrKey(r, side)
	return err
}

func (o oldApps) On(ctx context.Context) bool { return o.a.arr().On(ctx) }

func (o oldApps) SetOn(ctx context.Context, by domain.Actor, on bool) error {
	return o.a.arr().SaveSwitches(ctx, by, &on, nil)
}

// arr is the arr API: the Sonarr and Radarr prefixes and their settings.
func (a *handlers) arr() *arrapi.Handlers {
	return &arrapi.Handlers{
		Deps: arrapi.Deps{Store: a.s.Store, Library: a.s.Library, Metadata: a.s.Metadata, Queue: a.s.Queue,
			Picture: a.oldAppPicture, Secrets: a.s.Secrets, SourceSaved: a.publishSource, SourceRemoved: a.forgetSource},
		Auth: a.s.Auth,
	}
}

// forgetSource drops what the searcher keeps of a removed Source, as the
// Sources page's delete does.
func (a *handlers) forgetSource(id int64) {
	if a.s.Search != nil {
		a.s.Search.ForgetSource(id)
	}
}

// oldAppPicture sends a cover for those apps (Bazarr shows them): a
// stored poster ("poster:<cache name>"), made smaller once when px asks,
// or a wide picture's address, through the same cache and miss budget as
// the TMDB pictures of the web app.
func (a *handlers) oldAppPicture(w http.ResponseWriter, r *http.Request, src string, px int) {
	if a.s.Library == nil || a.s.Library.Artwork == nil {
		http.NotFound(w, r)
		return
	}
	c := a.s.Library.Artwork
	name, ok := strings.CutPrefix(src, "poster:")
	if !ok {
		a.serveRemoteArtwork(w, r, src, serveCover)
		return
	}
	p, ok := c.Path(name)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if px > 0 {
		if n, sp, err := c.Smaller(name, px); err == nil && n != name {
			library.ServeArtworkCached(w, r, n, sp, "private, max-age=86400")
			return
		}
	}
	serveCover(w, r, name, p)
}

// serveCover sends a cover. Its address stays the same when the title's
// picture changes, so it is kept a day and then revalidated by name,
// never kept a year as the web app's own picture addresses are.
func serveCover(w http.ResponseWriter, r *http.Request, name, p string) {
	if library.StableFile(p) {
		library.ServeArtwork(w, r, name, p) // already a day, named by the file's time
		return
	}
	library.ServeArtworkCached(w, r, name, p, "private, max-age=86400")
}

func (a *handlers) migrateHandlers() *migrate.Handlers {
	return &migrate.Handlers{Store: a.s.Store, Secrets: a.s.Secrets, Mover: a.mover, SafeMode: a.safeMode}
}

// mover is the move runner (made on first use in tests).
func (a *handlers) mover() *migrate.Mover {
	if a.s.Mover == nil {
		a.s.Mover = &migrate.Mover{Store: a.s.Store, Library: a.s.Library, Queue: a.s.Queue, Secrets: a.s.Secrets}
	}
	if a.s.Mover.PartOn == nil {
		a.s.Mover.PartOn = func(p string) bool { return !a.safeMode() && a.partIsOn(parts.Part(p)) }
	}
	return a.s.Mover
}

func (a *handlers) safeMode() bool { return a.s.SafeMode || (a.s.Parts != nil && a.s.Parts.SafeMode) }

// holding is the holding-folder routes of mediafiles on the library service.
func (a *handlers) holding() *mediafiles.HoldingHandlers {
	return &mediafiles.HoldingHandlers{
		HeldFiles: func(ctx context.Context) (any, error) { return a.s.Library.HeldFiles(ctx) },
		RestoreHeld: func(ctx context.Context, ids []int64, by domain.Actor) (any, []int64, error) {
			return a.s.Library.RestoreHeld(ctx, ids, by)
		},
		QueueRescan: func(ctx context.Context, by domain.Actor) (int64, error) {
			id, _, err := a.s.Queue.Enqueue(ctx, library.KindRescan, nil, nil, library.KindRescan, by)
			return id, err
		},
		Actor: actor,
	}
}
