package api

import (
	"net/http"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/arrapi"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/migrate"
	"github.com/HolyArrs/holyarrs/app/backend/internal/parts"
)

// ModuleRoutes registers the routes of the modules that have left this
// package. They need services this package builds, so it hands them over.
func (a *API) ModuleRoutes(rt *webserver.Router) {
	a.migrateHandlers().Routes(rt)
	a.arr().Routes(rt)
}

// arr is the old-app API: the Sonarr and Radarr prefixes and their settings.
func (a *API) arr() *arrapi.Handlers {
	return &arrapi.Handlers{
		Deps: arrapi.Deps{Store: a.s.Store, Library: a.s.Library, Metadata: a.s.Metadata, Queue: a.s.Queue,
			Picture: a.oldAppPicture, Secrets: a.s.Secrets, SourceSaved: a.publishSource, SourceRemoved: a.forgetSource},
		Auth: a.s.Auth,
	}
}

// forgetSource drops what the searcher keeps of a removed Source, as the
// Sources page's delete does.
func (a *API) forgetSource(id int64) {
	if a.s.Search != nil {
		a.s.Search.ForgetSource(id)
	}
}

// oldAppPicture sends a cover for those apps (Bazarr shows them): a
// stored poster ("poster:<cache name>"), made smaller once when px asks,
// or a wide picture's address, through the same cache and miss budget as
// the TMDB pictures of the web app.
func (a *API) oldAppPicture(w http.ResponseWriter, r *http.Request, src string, px int) {
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
			serveArtworkCached(w, r, n, sp, "private, max-age=86400")
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
		serveArtwork(w, r, name, p) // already a day, named by the file's time
		return
	}
	serveArtworkCached(w, r, name, p, "private, max-age=86400")
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
