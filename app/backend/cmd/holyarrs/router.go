package main

import (
	"context"
	"errors"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles/datapack"

	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/movies"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"

	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/updates"

	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/alerts"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/startup"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"

	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles"
)

// byHandRoutes registers the import-by-hand routes (internal/importer).
func (a *handlers) byHandRoutes(m *http.ServeMux) {
	h := &importer.Handlers{Deps: importer.Deps{Store: a.s.Store, SavePath: a.s.SavePath}}
	if h.SavePath == "" && a.s.Library != nil {
		h.SavePath = a.s.Library.SavePath
	}
	if a.s.Live != nil {
		h.Live = a.s.Live // a nil hub must stay a nil Publisher
	}
	h.Routes(m)
}

// Routes registers every route on rt. Modules that have left this package
// register themselves from cmd/holyarrs/wiring.go instead.
func (a *handlers) Routes(rt *webserver.Router) {
	m := rt.Mux()
	m.HandleFunc("GET /api/v1/titles", a.listTitles)
	if a.s.Store != nil { // some tests build the API without a store
		(&downloads.TagHandlers{DB: a.s.Store.Read()}).Routes(m)
	}
	a.ownedHandlers().Routes(m)
	a.suggest().Routes(m)
	th := a.titleHandlers()
	th.Routes(m)
	(&library.VersionHandlers{Store: a.s.Store, Lib: a.s.Library, TitleID: a.titleID, Publish: a.publishTitle, View: func(ctx context.Context, db database.Querier, v mediafiles.Version) any {
		return discover.VersionViewOf(ctx, db, v)
	}, Fail: library.WriteError, DownloadPaths: a.downloadPaths}).Routes(m)
	th.RefreshRoutes(m)
	m.HandleFunc("GET /api/v1/artwork/{name}", a.artwork)
	m.HandleFunc("GET /api/v1/artwork/tmdb/{size}/{file}", a.tmdbArtwork)
	m.HandleFunc("GET /api/v1/artwork/img/{kind}/{id}/{file}", a.serverArtwork)
	a.statusHandlers().Routes(m)
	(&requests.TitleDefaultsHandlers{Store: a.s.Store, Actor: actor}).Routes(m)
	a.dlh.Routes(m)
	(&library.FolderHandlers{Store: a.s.Store, DownloadPaths: a.downloadPaths, Actor: actor, Fail: library.WriteError}).Routes(m)
	lists := &requests.OutsideListHandlers{Store: a.s.Store, Queue: a.s.Queue, Secrets: a.s.Secrets, Actor: actor, LibraryError: library.WriteError}
	lists.Routes(m)
	(&sources.TitleSearchHandlers{Store: a.s.Store, Queue: a.s.Queue, Show: (&shows.SearchHandlers{Store: a.s.Store, Queue: a.s.Queue}).Search, Artist: a.mh().SearchArtist}).Routes(m)
	m.HandleFunc("GET /api/v1/downloads", downloads.ListDownloads(a.s.Store))
	m.HandleFunc("GET /api/v1/imports", mediafiles.ListImports(a.s.Store))
	m.HandleFunc("GET /api/v1/events", shows.ListEvents(a.s.Store))
	m.HandleFunc("GET /api/v1/jobs", a.listJobs)
	a.disc.PeopleRoutes(m)
	a.dlh.CopyRoutes(m)
	(&movies.Handlers{Lib: a.s.Library, KidMax: a.kidMax, AllowedTitleIDs: a.allowedTitleIDs}).Routes(m)
	a.mf.Routes(m)
	a.dl.Routes(m)
	(&library.OrganiseHandlers{OrganiseDeps: library.OrganiseDeps{Store: a.s.Store, Lib: a.s.Library, Queue: a.s.Queue, TitleID: a.titleID, Fail: library.WriteError}}).Routes(m)
	a.byHandRoutes(m)
	a.holding().Routes(m)
	a.healthHandlers().Routes(m)
	a.media.Routes(m)
	a.shows.Routes(m)
	a.mh().RouteMux(m)
	subtitles.NewHandlers(a.s.Store, a.s.Secrets, a.s.Subtitles).Routes(m)
	a.appHandlers().Routes(m)
	(&updates.Handlers{Checker: a.s.Updates}).Routes(m)
	(&datapack.Handlers{Deps: datapack.Deps{Store: a.s.Store}}).Routes(m)
	a.scheduleHandlers().Routes(m)
	m.HandleFunc("GET /api/v1/metadata/search", a.searchMetadata)
	(&metadata.Handlers{Up: func(ctx context.Context) bool { return a.s.MetadataUp == nil || a.s.MetadataUp(ctx) }}).Routes(m)
	a.disc.Routes(m)
	a.shows.AnimeRoutes(m)
	a.trailers.Routes(m)
	a.uiHandlers().Routes(m)
	a.signin.Routes(m)
	a.disc.StripRoutes(m)
	if startup.FeatureOn(startup.RowTitleStats) {
		m.HandleFunc("GET /api/v1/explore/title-stats", func(w http.ResponseWriter, r *http.Request) {
			discover.ExploreTitleStats(w, r, a.s.Store, a.explore.Now())
		})
	}
	a.titles.Routes(m)
	if a.s.Secrets != nil {
		if startup.FeatureOn(startup.TraktLists) {
			lists.TraktKeyRoutes(m)
			a.disc.TokenRoutes(m)
		}
	}
	a.askHandlers().Routes(m)
	(&webserver.URLBaseHandlers{Store: a.s.Store, InUse: a.s.URLBase}).Routes(m)
	(&alerts.Handlers{Store: a.s.Store, Secrets: a.s.Secrets, Sender: a.s.Alerts, Caller: func(r *http.Request) (int64, string) { p := a.person(r); return p.UserID, p.Name },
		OwnEmail: func(ctx context.Context, userID int64) (string, error) {
			t, err := requests.ReadTargets(ctx, a.s.Store.Read(), userID)
			return t.Email, err
		}}).Routes(m)
	(&requests.WatchlistHandlers{Store: a.s.Store, Queue: a.s.Queue, Watchlists: func() *requests.Watchlists { return a.s.Watchlists }, Person: a.person}).Routes(m)
	(&requests.ReportHandlers{Svc: a.s.Asks, Store: a.s.Store, Person: a.person, KidMax: a.kidMax, TitleGate: a.controls.ReportTitleGate}).Routes(m)
	(&requests.AskedByHandlers{Store: a.s.Store, KidMax: a.kidMax, AllowedTitleIDs: a.allowedTitleIDs}).Routes(m)
	(&requests.FollowHandlers{Svc: a.s.Asks, Person: a.person, Title: a.titleID, Changed: a.mh().FollowsChanged}).Routes(m)
	a.controls.Routes(m)
	m.HandleFunc("GET /api/v1/openapi.json", a.openapi)
	m.HandleFunc("GET /api/v1/health", a.health)
	m.HandleFunc("GET /healthz", a.healthz)
	// readyz answers 200 once the app has fully started (until then the
	// start page answers it with 503).
	m.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ready\n")) })
	a.s.Auth.LiveRoutes(m, a.s.Live, &a.interest, func() {
		if a.dl.DLSync != nil {
			a.dl.DLSync.Watched()
		}
	})
	m.Handle("GET /", webserver.Handler())
	if a.s.Backups != nil {
		a.s.Backups.Routes(m, a.s.Restart)
	}
	if a.s.Auth != nil {
		a.s.Auth.Routes(m)
	}
}

// Handler returns every route behind the sign-in, permission and gate layers.
func (a *handlers) Handler() http.Handler {
	rt := webserver.NewRouter()
	a.Routes(rt)
	a.ModuleRoutes(rt)
	return a.Chain(rt)
}

// Chain wraps a router that Routes filled with the layers every request
// passes: sign-in, the permission gate, kid limits, part switches and the
// security headers.
func (a *handlers) Chain(rt *webserver.Router) http.Handler {
	m := rt.Mux()
	if a.s.Auth != nil {
		a.ai.Inner = a.kidGate(m, a.controls.PartGate(m, accounts.Gate(m)))
		return webserver.SecureHeaders(a.arr().Gate(a.s.Auth.Middleware(wrapMux(accounts.LiveRecheck(a.s.Live, a.settingsLog().Wrap(a.status.ForgetOnWrite(a.kidGate(m, a.controls.PartGate(m, accounts.Gate(m))))))))))
	}
	a.ai.Inner = a.controls.PartGate(m, m)
	return webserver.SecureHeaders(a.arr().Gate(wrapMux(a.settingsLog().Wrap(a.status.ForgetOnWrite(a.controls.PartGate(m, m))))))
}

// wrapMux lets tests record which routes the mux served.
var wrapMux = func(h http.Handler) http.Handler { return h }

// healthz is open to everyone: it says only whether the database answers.
// It waits at most 2 s, so a busy database answers 503 well inside the
// health check's own 5 s limit instead of hanging it.
func (a *handlers) healthz(w http.ResponseWriter, _ *http.Request) {
	done := make(chan error, 1)
	go func() { _, err := a.s.Store.Version(); done <- err }()
	var err error
	select {
	case err = <-done:
	case <-time.After(2 * time.Second):
		err = errors.New("timed out")
	}
	if err != nil {
		http.Error(w, "database not answering\n", http.StatusServiceUnavailable)
		return
	}
	_, _ = w.Write([]byte("ok\n"))
}
