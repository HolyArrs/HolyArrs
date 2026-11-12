package shows

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// TVDBKeyGroup and TVDBKeyName are where the person's own TVDB key is kept
// in the vault. HolyArrs ships no TVDB key.
const TVDBKeyGroup, TVDBKeyName = "links", "tvdb_key"

// actor is the signed-in person, or the owner when no session is attached.
func actor(r *http.Request) domain.Actor {
	if _, ok := webserver.From(r.Context()); ok {
		return webserver.Actor(r.Context())
	}
	return domain.Actor{Kind: "user", ID: 1}
}

// Deps is what the Shows routes need: services and core types, and funcs
// for anything a higher module owns.
type Deps struct {
	Store    *database.Store
	Library  *library.Service
	Metadata metadata.Provider
	// AnimeMap is the metadata server's anime ID mapping, nil when unset.
	AnimeMap func(ctx context.Context, by, id string) (metadata.AnimeMap, error)
	// KidMax is the signed-in person's age-rating limit ("" for none).
	KidMax func(*http.Request) string
	// KidLimited counts one Kid lookup against the Explore minute limit;
	// true when it already answered 429.
	KidLimited func(w http.ResponseWriter, r *http.Request) bool
	// KidShows keeps the shows under max.
	KidShows func(ctx context.Context, max string, shows []metadata.Show) []metadata.Show
	// KindsOn are the asked kinds whose part is on.
	KindsOn func(kind string) []string
	// LibraryRating is a Library title's rating and whether it is there.
	LibraryRating func(ctx context.Context, kind, tmdbID string) (string, bool)
	// Caching sets the Explore cache headers.
	Caching func(w http.ResponseWriter, r *http.Request, cache string)
	// CachedDetails is the details page already loaded for kind/id, nil if none.
	CachedDetails func(kind, id string) *metadata.Details
	// Details is the details page for kind/id, from the cache when fresh.
	Details func(ctx context.Context, d metadata.Discoverer, kind, id string) (*metadata.Details, error)
	// OpenAsk is the person's open ask for a show, nil when none.
	OpenAsk func(r *http.Request, ids map[string]string) (any, error)
	// Follow puts an added Title on the adder's calendar.
	Follow func(r *http.Request, titleID int64) error
	// Added is the title view of a just added Title, also sent to live pages.
	Added func(ctx context.Context, t mediafiles.Title) (any, error)
	// Hidden marks a looked-up show the person's hides cover (nil: none).
	Hidden func(ctx context.Context, kind, tmdbID string, genres []string) (hidden, drop bool)
	// KindDefaultsFor fills an add's left-out fields from the saved defaults.
	KindDefaultsFor func(ctx context.Context, kind string, sent map[string]json.RawMessage,
		folder, rules *int64, keep **bool, search *bool) error
	// TitleID reads the {id} Title, answering when it is missing.
	TitleID func(w http.ResponseWriter, r *http.Request) (mediafiles.Title, bool)
	// StateLabels names the Slot states.
	StateLabels map[mediafiles.SlotState]string
	// Trim trims the Slots shown to a member.
	Trim func(ctx context.Context, slots map[int64][]EpisodeSlot)
	// LibraryError answers Library refusals Error does not know.
	LibraryError func(w http.ResponseWriter, err error)
	// PackNext switches on the next episodes of a season pack fetched a few at
	// a time, returning the Download and how many were switched on.
	PackNext func(ctx context.Context, titleID int64, season int, by domain.Actor) (int64, int, error)
}

// Handlers serves the Shows routes. Its state fields are private and
// zero-value ready, so &Handlers{Deps: d} works.
type Handlers struct {
	Deps
}

// NewHandlers is the Shows routes over d, with a default Kid limit reader.
func NewHandlers(d Deps) *Handlers {
	if d.KidMax == nil {
		d.KidMax = accounts.KidMax
	}
	return &Handlers{Deps: d}
}

// Routes registers the show add, episode and season routes.
func (h *Handlers) Routes(m *http.ServeMux) {
	(&AddHandlers{Store: h.Store, Lib: h.Library, Fail: h.Error, KidLimited: h.KidLimited,
		KidShows: func(r *http.Request, list []metadata.Show) []metadata.Show {
			return h.KidShows(r.Context(), h.KidMax(r), list)
		},
		OpenAsk: h.OpenAsk, Defaults: h.showDefaults, Follow: h.Follow, Added: h.Added, Hidden: h.Hidden}).Routes(m)
	(&EpisodeHandlers{Store: h.Store, Lib: h.Library, TitleID: h.TitleID, Fail: h.Error, StateLabels: h.StateLabels, Trim: h.Trim}).Routes(m)
	(&SeasonHandlers{Store: h.Store, Lib: h.Library, TitleID: h.TitleID, Fail: h.Error, PackNext: h.PackNext}).Routes(m)
}

// SeasonHandlers serves a show's seasons and the show settings.
type SeasonHandlers struct {
	Store *database.Store
	Lib   *library.Service
	// TitleID reads the {id} Title, answering 404 itself.
	TitleID func(w http.ResponseWriter, r *http.Request) (mediafiles.Title, bool)
	// Fail answers a show or library error.
	Fail func(w http.ResponseWriter, err error)
	// PackNext is Deps.PackNext; nil answers that no pack is fetched that way.
	PackNext func(ctx context.Context, titleID int64, season int, by domain.Actor) (int64, int, error)
}

// Routes registers the seasons and show settings routes.
func (h *SeasonHandlers) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/titles/{id}/seasons", h.listSeasons)
	m.HandleFunc("GET /api/v1/titles/{id}/episode-groups", h.listEpisodeGroups)
	m.HandleFunc("PUT /api/v1/titles/{id}/seasons/{number}", h.setSeason)
	m.HandleFunc("POST /api/v1/titles/{id}/seasons/{number}/pack-next", h.packNext)
	m.HandleFunc("GET /api/v1/settings/shows", h.getShowSettings)
	m.HandleFunc("PUT /api/v1/settings/shows", h.putShowSettings)
}

// KeepHelp is the help text for a keep_up_to_date change.
const KeepHelp = `send {"keep_up_to_date": true or false}`

// ShowID reads the {id} Title through titleID and answers 404 when it is not a show.
func ShowID(titleID func(http.ResponseWriter, *http.Request) (mediafiles.Title, bool), w http.ResponseWriter, r *http.Request) (mediafiles.Title, bool) {
	t, ok := titleID(w, r)
	if ok && t.Kind != "show" {
		webserver.Fail(w, http.StatusNotFound, "that Title is not a show")
		return t, false
	}
	return t, ok
}

func (h *SeasonHandlers) listSeasons(w http.ResponseWriter, r *http.Request) {
	t, ok := ShowID(h.TitleID, w, r)
	if !ok {
		return
	}
	ss, err := Seasons(r.Context(), h.Store.Read(), t.ID, time.Now())
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"seasons": ss})
}

// listEpisodeGroups lists the TMDB episode orders a show can follow and
// the one it follows. A show without a TMDB id, or not on TMDB, has none.
func (h *SeasonHandlers) listEpisodeGroups(w http.ResponseWriter, r *http.Request) {
	t, ok := ShowID(h.TitleID, w, r)
	if !ok {
		return
	}
	groups := []metadata.EpisodeGroupInfo{}
	if tmdb := t.SourceIDs["tmdb"]; tmdb != "" && t.MetadataSource == "tmdb" && h.Lib != nil && h.Lib.EpisodeGroups != nil {
		gs, err := h.Lib.EpisodeGroups.EpisodeGroups(r.Context(), tmdb)
		switch {
		case errors.Is(err, metadata.ErrShowNotFound), errors.Is(err, metadata.ErrNotFound):
		case err != nil:
			webserver.Fail(w, http.StatusBadGateway, "TMDB could not be read: "+err.Error())
			return
		default:
			groups = gs
		}
	}
	var chosen *string
	if t.EpisodeGroupID != nil && *t.EpisodeGroupID != "" {
		chosen = t.EpisodeGroupID
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"groups": groups, "chosen": chosen})
}

func (h *SeasonHandlers) setSeason(w http.ResponseWriter, r *http.Request) {
	t, ok := ShowID(h.TitleID, w, r)
	if !ok {
		return
	}
	n, err := strconv.Atoi(r.PathValue("number"))
	if err != nil || n < 0 {
		webserver.Fail(w, http.StatusNotFound, "no such season")
		return
	}
	var body struct {
		KeepUpToDate *bool `json:"keep_up_to_date"`
	}
	if !webserver.Decode(w, r, &body, KeepHelp) {
		return
	}
	if body.KeepUpToDate == nil {
		webserver.Fail(w, http.StatusBadRequest, KeepHelp)
		return
	}
	if err := New(h.Lib).SetSeason(r.Context(), t.ID, n, *body.KeepUpToDate, actor(r)); err != nil {
		h.Fail(w, err)
		return
	}
	ss, err := Seasons(r.Context(), h.Store.Read(), t.ID, time.Now())
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	for _, s := range ss {
		if s.Number == n {
			webserver.WriteJSON(w, http.StatusOK, s)
			return
		}
	}
	webserver.Fail(w, http.StatusNotFound, "no such season")
}

func (h *SeasonHandlers) getShowSettings(w http.ResponseWriter, r *http.Request) {
	d, err := library.ReadShowDefaults(r.Context(), h.Store.Read())
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, d)
}

func (h *SeasonHandlers) putShowSettings(w http.ResponseWriter, r *http.Request) {
	d, err := library.ReadShowDefaults(r.Context(), h.Store.Read())
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	if !webserver.Decode(w, r, &d, "send any of season_choice, new_seasons, episode_order, details_source, keep_specials, episode_wait_minutes") {
		return
	}
	if err := SetShowDefaults(r.Context(), h.Store, d, actor(r)); err != nil {
		h.Fail(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, d)
}

// packNext answers POST /api/v1/titles/{id}/seasons/{number}/pack-next: the
// season's pack fetched a few episodes at a time gets its next episodes.
func (h *SeasonHandlers) packNext(w http.ResponseWriter, r *http.Request) {
	t, ok := ShowID(h.TitleID, w, r)
	if !ok {
		return
	}
	n, err := strconv.Atoi(r.PathValue("number"))
	if err != nil || n < 0 {
		webserver.Fail(w, http.StatusNotFound, "no such season")
		return
	}
	if h.PackNext == nil {
		webserver.Fail(w, http.StatusNotFound, downloads.ErrNoPack.Error())
		return
	}
	id, added, err := h.PackNext(r.Context(), t.ID, n, actor(r))
	switch {
	case errors.Is(err, downloads.ErrNoPack):
		webserver.Fail(w, http.StatusNotFound, err.Error())
	case errors.Is(err, downloads.ErrPackGone), errors.Is(err, downloads.ErrPackChanged):
		webserver.Fail(w, http.StatusConflict, err.Error())
	case err != nil:
		webserver.Fail(w, http.StatusBadGateway, err.Error())
	default:
		webserver.WriteJSON(w, http.StatusOK, map[string]any{"download_id": id, "added": added})
	}
}
