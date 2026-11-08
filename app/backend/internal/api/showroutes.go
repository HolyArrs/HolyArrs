package api

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// showRoutes serves adding shows, their seasons and episodes, and the show
// settings.
func (a *API) showRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/shows/lookup", a.lookupShows)
	m.HandleFunc("POST /api/v1/shows", a.addShow)
	(&shows.EpisodeHandlers{Store: a.s.Store, Lib: a.s.Library, TitleID: a.titleID, Fail: showError, StateLabels: StateLabels, Trim: trimEpisodeSlots}).Routes(m)
	(&shows.SeasonHandlers{Store: a.s.Store, Lib: a.s.Library, TitleID: a.titleID, Fail: showError}).Routes(m)
}

// showError answers show refusals with a status and a reason; anything
// else goes to libraryError.
func showError(w http.ResponseWriter, err error) {
	var dup *shows.ShowAlreadyAddedError
	switch {
	case errors.As(err, &dup):
		webserver.WriteJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "title_id": dup.TitleID})
	case errors.Is(err, library.ErrNoShowFolder), errors.Is(err, shows.ErrShowNeverAdd):
		webserver.Fail(w, http.StatusConflict, err.Error())
	case errors.Is(err, library.ErrBadSeasonChoice), errors.Is(err, library.ErrBadNewSeasons), errors.Is(err, library.ErrBadOrder),
		errors.Is(err, library.ErrBadWait), errors.Is(err, library.ErrNotShowFolder), errors.Is(err, library.ErrMovieOnly),
		errors.Is(err, library.ErrShowOnly), errors.Is(err, metadata.ErrUnknownSource):
		webserver.Fail(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, library.ErrNotShow), errors.Is(err, library.ErrNoSuchSeason), errors.Is(err, metadata.ErrShowNotFound):
		webserver.Fail(w, http.StatusNotFound, err.Error())
	case errors.Is(err, metadata.ErrNoTVDBKey), errors.Is(err, library.ErrNoShowSources):
		webserver.Fail(w, http.StatusServiceUnavailable, err.Error())
	default:
		libraryError(w, err)
	}
}

// ShowResult is a looked-up show (without its episodes) with what the
// Library knows of it.
type ShowResult struct {
	metadata.Show
	TitleID  int64     `json:"title_id,omitempty"` // set when already in the Library
	NeverAdd bool      `json:"never_add,omitempty"`
	Ask      *askBrief `json:"ask,omitempty"` // the open ask for it, if any
}

// lookupShows finds shows to add by name, or by "tmdb:", "tvdb:",
// "tvmaze:" or "imdb:" id, in one details source (the default when none
// is named).
func (a *API) lookupShows(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	term := strings.TrimSpace(q.Get("term"))
	if term == "" || len(term) > 200 {
		webserver.Fail(w, http.StatusBadRequest, "send ?term=<name> or an id such as tmdb:1399, tvdb:121361, tvmaze:82 or imdb:tt0944947, and &source=tmdb, tvmaze or tvdb")
		return
	}
	source := q.Get("source")
	if source == "" {
		d, err := library.ReadShowDefaults(r.Context(), a.s.Store.Read())
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		source = d.Source
	}
	if !metadata.ValidShowSource(source) {
		showError(w, metadata.ErrUnknownSource)
		return
	}
	if a.kidLimited(w, r) {
		return
	}
	var list []metadata.Show
	low := strings.ToLower(term)
	if k, _, ok := strings.Cut(low, ":"); ok && (k == "tmdb" || k == "tvdb" || k == "tvmaze" || k == "imdb") {
		sh, err := shows.New(a.s.Library).LookupShow(r.Context(), k+":"+strings.TrimSpace(term[len(k)+1:]), source)
		if err != nil && !errors.Is(err, metadata.ErrShowNotFound) {
			showError(w, err)
			return
		}
		if err == nil {
			list = []metadata.Show{sh}
		}
	} else {
		p, err := a.showSource(source)
		if err != nil {
			showError(w, err)
			return
		}
		if list, err = p.SearchShows(r.Context(), term); err != nil {
			showError(w, err)
			return
		}
	}
	out := []ShowResult{}
	for _, sh := range a.kidShows(r.Context(), a.kidMax(r), list) {
		sh.Episodes = nil
		v := ShowResult{Show: sh}
		var err error
		if v.TitleID, err = library.ShowByIDs(r.Context(), a.s.Store.Read(), sh.IDs); err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		if v.NeverAdd, err = library.ShowOnNeverAdd(r.Context(), a.s.Store.Read(), sh.IDs); err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		if v.Ask, err = a.openShowAsk(r.Context(), sh.IDs, a.person(r).UserID); err != nil {
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

func (a *API) showSource(name string) (metadata.ShowProvider, error) {
	if a.s.Library.Shows == nil {
		return nil, library.ErrNoShowSources
	}
	return a.s.Library.Shows.Get(name)
}

const addShowHelp = `send {"id": "tmdb:<number>"} (or tvdb:, tvmaze:, imdb:) with optional library_folder_id, pick_rules_id, keep_up_to_date, seasons (all, future, latest, none), new_seasons (all, none), episode_order (aired, daily, absolute), details_source (tmdb, tvmaze, tvdb), search_on_add, more_versions ([{"label": "4K", "pick_rules_id": <id>, "library_folder_id": <id>}])`

func (a *API) addShow(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID string `json:"id"`
		shows.ShowAddOptions
		Follow *bool `json:"follow"` // put it on the adder's calendar (default true)
	}
	sent, ok := decodeKeys(w, r, &body, addShowHelp)
	if !ok {
		return
	}
	o := &body.ShowAddOptions
	if err := a.kindDefaultsFor(r.Context(), "show", sent, &o.LibraryFolderID, &o.PickRulesID, &o.KeepUpToDate, &o.SearchOnAdd); err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	if strings.TrimSpace(body.ID) == "" {
		webserver.Fail(w, http.StatusBadRequest, addShowHelp)
		return
	}
	id, jobID, err := shows.New(a.s.Library).AddShow(r.Context(), strings.TrimSpace(body.ID), body.ShowAddOptions, actor(r))
	if err != nil {
		showError(w, err)
		return
	}
	if (body.Follow == nil || *body.Follow) && a.s.Asks != nil {
		if err := a.s.Asks.FollowAdded(r.Context(), a.person(r), id); err != nil {
			log.Printf("api: follow on add: %v", err)
		}
	}
	var t mediafiles.Title
	if err := database.Get(r.Context(), a.s.Store.Read(), "titles", id, &t); err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	v, err := a.titleView(r.Context(), t)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	if a.s.Live != nil {
		a.publishTitleView(v)
	}
	webserver.WriteJSON(w, http.StatusCreated, map[string]any{"title": v, "job_id": jobID})
}

// EpisodeSlot and EpisodeView are the shows module's episode views.
type (
	EpisodeSlot = shows.EpisodeSlot
	EpisodeView = shows.EpisodeView
)
