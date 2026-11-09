package music

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/health"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/suggest"
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
	Auth         *accounts.Auth
	Version      string
}

// testAPI is a music Handlers over a test Server; s is kept so a test can
// swap Library or ImageBase.
type testAPI struct {
	*Handlers
	s *Server
	// extra mounts more routes a test needs (other modules' routes).
	extra func(m *http.ServeMux)
}

// newTestAPI builds the music Handlers the way api.New wires them, with the
// stand-ins api uses without sign-in.
func newTestAPI(s Server) *testAPI {
	sp := &s
	if sp.Library == nil {
		sp.Library = &library.Service{Store: sp.Store, Queue: sp.Queue, Metadata: sp.Metadata, Live: sp.Live}
	}
	if sp.Asks == nil {
		sp.Asks = &requests.Service{Store: sp.Store, Library: sp.Library, Metadata: sp.Metadata, Live: sp.Live}
	}
	t := &testAPI{s: sp}
	h := &Handlers{Deps: &sp.Deps, Services: Services{
		Store: sp.Store, Packages: sp.Packages, Queue: sp.Queue, Live: sp.Live, Parts: sp.Parts, Asks: sp.Asks,
		Library:      func() *library.Service { return sp.Library },
		ImageBase:    func() string { return sp.ImageBase },
		MusicControl: sp.MusicControl, ArtistVideos: sp.ArtistVideos,
		SlotOfTitle: func(ctx context.Context, itemID, titleID int64) bool {
			var n int
			_ = sp.Store.Read().QueryRowContext(ctx, "SELECT count(*) FROM items WHERE id = ? AND title_id = ?", itemID, titleID).Scan(&n)
			return n == 1
		},
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
	h.TitleView = func(ctx context.Context, t mediafiles.Title) (any, error) {
		v := struct {
			mediafiles.Title
			Slots  []any           `json:"slots"`
			Artist *ArtistInfo     `json:"artist,omitempty"`
			Asks   []requests.View `json:"asks,omitempty"`
		}{Title: t, Slots: []any{}}
		var err error
		if t.Kind == "artist" {
			if v.Artist, err = h.artistInfo(ctx, t); err == nil {
				v.Asks, err = sp.Asks.TitleAsks(ctx, t.ID)
			}
		}
		return v, err
	}
	h.PublishTitleView = func(any) {}
	h.LibraryError = rigLibraryError
	h.Limit = func(int64, time.Time) bool { return true }
	h.Trailers = &metadata.Trailers{Store: sp.Store, Version: sp.Version, Provider: func() metadata.Provider { return sp.Metadata },
		Pick:    func() func(ctx context.Context, kind, id, lang string) (string, error) { return nil },
		Explore: rigTrailerHost{t}, Settings: rigTrailerHost{t}, Ages: rigTrailerHost{t}, People: rigTrailerHost{t}, Wants: rigTrailerHost{t},
		KindsOn: func(kind string) []string { return []string{kind} }, Videos: h.VideoHooks()}
	h.TrackRoutes = func(m *http.ServeMux) {
		TrackHandlers[trackSlot]{Store: sp.Store, Slots: rigTrackSlots}.Routes(m)
	}
	t.Handlers = h
	return t
}

// Handler serves the music routes alone (no sign-in layer).
func (a *testAPI) Handler() http.Handler {
	rt := webserver.NewRouter()
	m := rt.Mux()
	a.RouteMux(m)
	// The search and release routes, over a stand-in catalog.
	search := &sources.Searcher{Store: a.s.Store, Queue: a.s.Queue}
	(&sources.Handlers{Store: a.s.Store, Queue: a.s.Queue, Search: func() *sources.Searcher { return search },
		Catalog: rigCatalog{a}, PartOn: a.PartIsOn}).Routes(rt)
	// The trailer routes (music videos play through their relay) and the
	// look settings.
	a.Trailers.Routes(m)
	(&health.UIHandlers{Store: a.s.Store, Actor: func(r *http.Request) domain.Actor { return webserver.ActorOr(r, domain.Actor{Kind: "user", ID: 1}) }}).Routes(m)
	// The title routes the music tests read through (api serves them in the app).
	m.HandleFunc("GET /api/v1/titles/{id}", func(w http.ResponseWriter, r *http.Request) {
		t, ok := a.TitleID(w, r)
		if !ok {
			return
		}
		v, err := a.TitleView(r.Context(), t)
		if err != nil {
			webserver.Internal(w, "music", err)
			return
		}
		webserver.WriteJSON(w, http.StatusOK, v)
	})
	(&sources.TitleSearchHandlers{Store: a.s.Store, Queue: a.s.Queue, Artist: a.SearchArtist}).Routes(m)
	(&requests.FollowHandlers{Svc: a.s.Asks, Person: a.Person, Title: a.TitleID, Changed: a.FollowsChanged}).Routes(m)
	// The Library-folder route the tests add their music folder with.
	m.HandleFunc("POST /api/v1/library-folders", func(w http.ResponseWriter, r *http.Request) {
		var in library.FolderInput
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			webserver.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		lf, err := library.AddFolder(r.Context(), a.s.Store, in, nil, webserver.ActorOr(r, domain.Actor{Kind: "user", ID: 1}))
		if err != nil {
			webserver.Fail(w, http.StatusConflict, err.Error())
			return
		}
		webserver.WriteJSON(w, http.StatusCreated, map[string]any{"id": lf.ID, "path": lf.Path, "kind": lf.Kind})
	})
	if a.extra != nil {
		a.extra(m)
	}
	if a.s.Auth == nil {
		return m
	}
	// With sign-in: the accounts and ask routes too, behind the sign-in layer.
	a.s.Auth.Routes(m)
	actor := func(r *http.Request) domain.Actor { return webserver.ActorOr(r, domain.Actor{Kind: "user", ID: 1}) }
	(&requests.AskHandlers{Svc: a.s.Asks, Store: a.s.Store, Person: a.Person, Actor: actor,
		PartOff: func(http.ResponseWriter, string) bool { return false },
		SeenArtists: func(vs []requests.View) {
			var names []NameKey
			for _, v := range vs {
				if v.Kind == "artist" && v.ArtistID != "" {
					names = append(names, NewNameKey("artist", strings.ToLower(v.ArtistID), v.Name, ""))
				}
			}
			a.RememberNames(names)
		},
		MusicError: a.musicError, ShowError: a.musicError}).Routes(m)
	return a.s.Auth.Middleware(accounts.Gate(m))
}

// rigTrackSlots is the test stand-in for api's track Slot views: each
// track's Slots with their state.
func rigTrackSlots(ctx context.Context, db *sql.DB, p mediafiles.Part) (map[int64][]trackSlot, error) {
	slots, err := database.Select[mediafiles.Slot](ctx, db, "slots", "item_id IN (SELECT id FROM items WHERE part_id = ? AND kind = 'track') ORDER BY id", p.ID)
	if err != nil {
		return nil, err
	}
	out := map[int64][]trackSlot{}
	for _, s := range slots {
		out[s.ItemID] = append(out[s.ItemID], trackSlot{State: string(s.State), StateLabel: mediafiles.StateLabels[s.State]})
	}
	return out, nil
}

// rigCatalog answers the sources routes' questions about Titles and Parts.
type rigCatalog struct{ a *testAPI }

func (c rigCatalog) TitleFromPath(w http.ResponseWriter, r *http.Request) (mediafiles.Title, bool) {
	return c.a.TitleID(w, r)
}

func (c rigCatalog) EpisodeLabels(context.Context, []int64) string { return "" }

func (c rigCatalog) AlbumLabels(ctx context.Context, slotIDs []int64) string {
	return c.a.AlbumLabels(ctx, slotIDs)
}

func (c rigCatalog) PartLabel(ctx context.Context, partID int64) (int64, string, string, string) {
	return c.a.PartLabel(ctx, partID)
}

// rigLibraryError answers Library refusals with the codes api gives them.
func rigLibraryError(w http.ResponseWriter, err error) {
	for _, bad := range []error{library.ErrBadGate, library.ErrBadDelay, library.ErrNotMovieFolder, library.ErrNotShowFolder, library.ErrNotMusicFolder,
		library.ErrNoPickRules, library.ErrNotMusicRules, library.ErrNotVideoRules, library.ErrFolderPath, library.ErrFolderKind,
		library.ErrFolderAbsent, library.ErrNoMetadataIDs, library.ErrVersionLabel, library.ErrTooManyVersions, library.ErrCopyChoice} {
		if errors.Is(err, bad) {
			webserver.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	var dup *library.AlreadyAddedError
	switch {
	case errors.As(err, &dup):
		webserver.WriteJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "title_id": dup.TitleID})
	case errors.Is(err, database.ErrNotFound), errors.Is(err, metadata.ErrNotFound):
		webserver.Fail(w, http.StatusNotFound, "not found")
	default:
		webserver.Fail(w, http.StatusConflict, err.Error())
	}
}

// rigTrailerHost answers the trailers' questions about the household:
// no movie or show titles, no age limit beyond the Kid's, the look
// settings from the store.
type rigTrailerHost struct{ a *testAPI }

func (h rigTrailerHost) Details(context.Context, string, string) (metadata.Details, error) {
	return metadata.Details{}, metadata.ErrNotFound
}

func (h rigTrailerHost) Caching(w http.ResponseWriter, r *http.Request, cache string) {
	webserver.CacheHeaders(w, cache, accounts.KidMax(r) != "")
}

func (h rigTrailerHost) AutoplayTrailers(ctx context.Context) bool {
	return health.AutoplayOn(ctx, h.a.s.Store)
}
func (h rigTrailerHost) Max(r *http.Request) string                      { return accounts.KidMax(r) }
func (h rigTrailerHost) Limited(http.ResponseWriter, *http.Request) bool { return false }
func (h rigTrailerHost) LibraryRating(context.Context, string, string) (string, bool) {
	return "", false
}
func (h rigTrailerHost) UserID(r *http.Request) int64           { return h.a.Person(r).UserID }
func (h rigTrailerHost) PassTitles(context.Context) [][2]string { return nil }
func (h rigTrailerHost) PassLanguages(context.Context) []string { return []string{"en"} }
func (h rigTrailerHost) Warm(string, [][2]string)               {}
