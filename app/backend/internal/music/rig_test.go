package music

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/discover/rows"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"
)

// Server is the test stand-in for the old api.Server: the services the
// moved music tests built the API with.
type Server struct {
	Deps
	Store        *database.Store
	Queue        *jobs.Queue
	Live         *webserver.LiveHub
	Library      *library.Service
	Parts        *parts.Control
	Asks         *requests.Service
	Packages     *PackageSync
	ImageBase    string
	MusicControl func(ctx context.Context) (metadata.Control, error)
	ArtistVideos func(ctx context.Context, mbid string) ([]metadata.ArtistVideo, error)
	Metadata     metadata.Provider
	Version      string
}

// testAPI is a music Handlers over a test Server; s is kept so a test can
// swap Library or ImageBase as it did on the api.
type testAPI struct {
	*Handlers
	s *Server
}

// newTestAPI builds the music Handlers the way api.New wires them, with the
// stand-ins api uses without sign-in.
func newTestAPI(s Server) *testAPI {
	sp := &s
	if sp.Library == nil {
		sp.Library = &library.Service{Store: sp.Store, Queue: sp.Queue, Metadata: sp.Metadata, Live: sp.Live}
	}
	t := &testAPI{s: sp}
	h := &Handlers{Deps: &sp.Deps, Services: Services{
		Store: sp.Store, Packages: sp.Packages, Queue: sp.Queue, Live: sp.Live, Parts: sp.Parts, Asks: sp.Asks,
		Library:      func() *library.Service { return sp.Library },
		ImageBase:    func() string { return sp.ImageBase },
		MusicControl: sp.MusicControl, ArtistVideos: sp.ArtistVideos,
		SlotOfTitle: func(context.Context, int64, int64) bool { return false },
		SeasonLabel: func(context.Context, int64) (int64, string, string) { return 0, "", "" },
	}}
	h.PartIsOn = func(p parts.Part) bool { return parts.PartOn(sp.Parts, p) }
	h.KidMax = accounts.KidMax
	h.Person = func(r *http.Request) requests.Person {
		return requests.PersonOf(r, accounts.KidMax, func(context.Context) string { return "" })
	}
	h.OpenArtistAsks = func(ctx context.Context, mbid string, me int64) ([]requests.AskBrief, error) {
		return requests.OpenArtistAsks(ctx, sp.Store.Read(), mbid, me)
	}
	h.ArtistVideosOn = func() bool {
		p, _ := parts.ForTitleKind("artist")
		return h.PartIsOn(p) && sp.Library != nil && sp.Library.Artists != nil && sp.ArtistVideos != nil
	}
	h.MusicFresh = func(string, MusicCard) *engine.Fresh { return nil }
	h.ExploreCaching = func(w http.ResponseWriter, r *http.Request, cache string) {
		webserver.CacheHeaders(w, cache, accounts.KidMax(r) != "")
	}
	h.TitleID = func(w http.ResponseWriter, r *http.Request) (mediafiles.Title, bool) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		var t mediafiles.Title
		if err == nil {
			err = database.Get(r.Context(), sp.Store.Read(), "titles", id, &t)
		}
		if err != nil {
			webserver.Fail(w, http.StatusNotFound, "no such Title")
			return t, false
		}
		return t, true
	}
	h.TitleView = func(_ context.Context, t mediafiles.Title) (any, error) { return t, nil }
	h.PublishTitleView = func(any) {}
	h.LibraryError = func(w http.ResponseWriter, err error) { webserver.Fail(w, http.StatusConflict, err.Error()) }
	h.Limit = func(int64, time.Time) bool { return true }
	t.Handlers = h
	return t
}

// Handler serves the music routes alone (no sign-in layer).
func (a *testAPI) Handler() http.Handler {
	m := http.NewServeMux()
	a.RouteMux(m)
	return m
}
