package shows

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// ShowResult is a looked-up show (without its episodes) with what the
// Library knows of it.
type ShowResult struct {
	metadata.Show
	TitleID  int64 `json:"title_id,omitempty"` // set when already in the Library
	NeverAdd bool  `json:"never_add,omitempty"`
	Ask      any   `json:"ask,omitempty"` // the open ask for it, if any
	// Hidden: hidden from browsing; managers still find it here to add it.
	Hidden bool `json:"hidden,omitempty"`
}

// AddHandlers serves looking up and adding shows.
type AddHandlers struct {
	Store *database.Store
	Lib   *library.Service
	// Fail answers a show or library error.
	Fail func(w http.ResponseWriter, err error)
	// KidLimited answers a Kid who may not look up, reporting true when it did.
	KidLimited func(w http.ResponseWriter, r *http.Request) bool
	// KidShows drops the shows above the person's limit.
	KidShows func(r *http.Request, list []metadata.Show) []metadata.Show
	// OpenAsk is the person's view of the open ask for a show, nil when none.
	OpenAsk func(r *http.Request, ids map[string]string) (any, error)
	// Defaults fills the add options left out of the body from the saved TV defaults.
	Defaults func(ctx context.Context, sent map[string]json.RawMessage, o *ShowAddOptions) error
	// Follow puts an added show on the adder's calendar; nil when asks are off.
	Follow func(r *http.Request, titleID int64) error
	// Added is the answer's title view for the added show; it also tells live pages.
	Added func(ctx context.Context, t mediafiles.Title) (any, error)
	// Hidden says whether the person's hides cover a show (hidden) and
	// whether they should not see it at all (drop); nil hides nothing.
	Hidden func(ctx context.Context, kind, tmdbID string, genres []string) (hidden, drop bool)
}

// Routes registers the lookup and add routes.
func (h *AddHandlers) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/shows/lookup", h.lookupShows)
	m.HandleFunc("POST /api/v1/shows", h.addShow)
}

// lookupShows finds shows to add by name, or by "tmdb:", "tvdb:",
// "tvmaze:" or "imdb:" id, in one details source (the default when none
// is named).
func (h *AddHandlers) lookupShows(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	term := strings.TrimSpace(q.Get("term"))
	if term == "" || len(term) > 200 {
		webserver.Fail(w, http.StatusBadRequest, "send ?term=<name> or an id such as tmdb:1399, tvdb:121361, tvmaze:82 or imdb:tt0944947, and &source=tmdb, tvmaze or tvdb")
		return
	}
	source := q.Get("source")
	if source == "" {
		d, err := library.ReadShowDefaults(r.Context(), h.Store.Read())
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		source = d.Source
	}
	if !metadata.ValidShowSource(source) {
		h.Fail(w, metadata.ErrUnknownSource)
		return
	}
	if h.KidLimited(w, r) {
		return
	}
	var list []metadata.Show
	low := strings.ToLower(term)
	k, _, byID := strings.Cut(low, ":")
	byID = byID && (k == "tmdb" || k == "tvdb" || k == "tvmaze" || k == "imdb")
	if byID {
		sh, err := New(h.Lib).LookupShow(r.Context(), k+":"+strings.TrimSpace(term[len(k)+1:]), source)
		if err != nil && !errors.Is(err, metadata.ErrShowNotFound) {
			h.Fail(w, err)
			return
		}
		if err == nil {
			list = []metadata.Show{sh}
		}
	} else {
		p, err := Source(h.Lib, source)
		if err != nil {
			h.Fail(w, err)
			return
		}
		if list, err = p.SearchShows(r.Context(), term); err != nil {
			h.Fail(w, err)
			return
		}
	}
	fillTMDBIDs(r.Context(), h.finderFor(source), list)
	out := []ShowResult{}
	for _, sh := range h.KidShows(r, list) {
		sh.Episodes = nil
		v := ShowResult{Show: sh}
		if id := sh.IDs["tmdb"]; id != "" && h.Hidden != nil {
			hidden, drop := h.Hidden(r.Context(), "show", id, sh.Genres)
			if drop && !byID { // an id is a link: it still finds the show
				continue
			}
			v.Hidden = hidden
		}
		var err error
		if v.TitleID, err = library.ShowByIDs(r.Context(), h.Store.Read(), sh.IDs); err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		if v.NeverAdd, err = library.ShowOnNeverAdd(r.Context(), h.Store.Read(), sh.IDs); err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		if v.Ask, err = h.OpenAsk(r, sh.IDs); err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		out = append(out, v)
	}
	res := map[string]any{"shows": out, "source": source}
	if source == "tmdb" {
		res["attribution"] = metadata.Attribution
	}
	webserver.WriteJSON(w, http.StatusOK, res)
}

// Source is the named show details source.
func Source(lib *library.Service, name string) (metadata.ShowProvider, error) {
	if lib.Shows == nil {
		return nil, library.ErrNoShowSources
	}
	return lib.Shows.Get(name)
}

const addShowHelp = `send {"id": "tmdb:<number>"} (or tvdb:, tvmaze:, imdb:) with optional library_folder_id, pick_rules_id, keep_up_to_date, seasons (all, future, latest, none), new_seasons (all, none), episode_order (aired, daily, absolute), details_source (tmdb, tvmaze, tvdb), search_on_add, more_versions ([{"label": "4K", "pick_rules_id": <id>, "library_folder_id": <id>}])`

func (h *AddHandlers) addShow(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID string `json:"id"`
		ShowAddOptions
		Follow *bool `json:"follow"` // put it on the adder's calendar (default true)
	}
	sent, ok := webserver.DecodeKeys(w, r, &body, addShowHelp)
	if !ok {
		return
	}
	if err := h.Defaults(r.Context(), sent, &body.ShowAddOptions); err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	if strings.TrimSpace(body.ID) == "" {
		webserver.Fail(w, http.StatusBadRequest, addShowHelp)
		return
	}
	id, jobID, err := New(h.Lib).AddShow(r.Context(), strings.TrimSpace(body.ID), body.ShowAddOptions, actor(r))
	if err != nil {
		h.Fail(w, err)
		return
	}
	if (body.Follow == nil || *body.Follow) && h.Follow != nil {
		if err := h.Follow(r, id); err != nil {
			log.Printf("api: follow on add: %v", err)
		}
	}
	var t mediafiles.Title
	if err := mediafiles.LoadTitle(r.Context(), h.Store.Read(), id, &t); err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	v, err := h.Added(r.Context(), t)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusCreated, map[string]any{"title": v, "job_id": jobID})
}
