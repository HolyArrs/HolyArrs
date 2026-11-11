package discover

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"maps"
	"net/http"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"
)

// TitleHandlers serve the Library title routes: add, read, edit, move,
// look up and refresh.
type TitleHandlers struct {
	Store    *database.Store
	Library  *library.Service
	Metadata metadata.Provider
	Queue    *jobs.Queue
	Asks     *requests.Service
	// Live says whether title changes are published live.
	Live   bool
	Actor  func(*http.Request) domain.Actor
	Person func(*http.Request) requests.Person
	// Fail answers a library refusal; ShowError a show edit's refusal.
	Fail            func(http.ResponseWriter, error)
	ShowError       func(http.ResponseWriter, error)
	KindDefaultsFor func(ctx context.Context, kind string, sent map[string]json.RawMessage,
		folder, rules *int64, keep **bool, search *bool) error
	TitleID          func(http.ResponseWriter, *http.Request) (mediafiles.Title, bool)
	TitleView        func(ctx context.Context, t mediafiles.Title) (TitleView, error)
	PublishTitleView func(v TitleView)
	KidMax           func(*http.Request) string
	KidLimited       func(http.ResponseWriter, *http.Request) bool
	KidMovies        func(ctx context.Context, max string, ms []metadata.Movie) []metadata.Movie
	OpenAsk          func(ctx context.Context, tmdbID string, me int64) (*requests.AskBrief, error)
	OpenMovieAsk     func(ctx context.Context, tmdbID string, me int64, fourK bool) (*requests.AskBrief, error)
}

// Routes registers the title routes.
func (h *TitleHandlers) Routes(m *http.ServeMux) {
	m.HandleFunc("POST /api/v1/titles", h.addTitle)
	m.HandleFunc("GET /api/v1/titles/lookup", h.lookupTitles)
	m.HandleFunc("GET /api/v1/titles/{id}", h.getTitle)
	m.HandleFunc("PUT /api/v1/titles/{id}", h.editTitle)
	m.HandleFunc("POST /api/v1/titles/{id}/move", h.moveTitle)
}

// RefreshRoutes registers the refresh routes (after the Versions routes,
// as before the move).
func (h *TitleHandlers) RefreshRoutes(m *http.ServeMux) {
	m.HandleFunc("POST /api/v1/titles/{id}/refresh", h.refreshTitle)
	m.HandleFunc("POST /api/v1/titles/refresh", h.refreshTitles)
}

func (h *TitleHandlers) addTitle(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID string `json:"id"` // "tmdb:693134" or "imdb:tt15239678"
		library.AddOptions
		Follow *bool `json:"follow"` // put it on the adder's calendar (default true)
	}
	sent, ok := webserver.DecodeKeys(w, r, &body, `send {"id": "tmdb:<number>"} with optional library_folder_id, pick_rules_id, keep_up_to_date, release_gate, gate_delay_days, search_on_add, more_versions, follow`)
	if !ok {
		return
	}
	o := &body.AddOptions
	if err := h.KindDefaultsFor(r.Context(), "movie", sent, &o.LibraryFolderID, &o.PickRulesID, &o.KeepUpToDate, &o.SearchOnAdd); err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	if body.ID == "" {
		webserver.Fail(w, http.StatusBadRequest, `send {"id": "tmdb:<number>"}`)
		return
	}
	m, err := h.Metadata.Movie(r.Context(), body.ID)
	if err != nil {
		h.Fail(w, err)
		return
	}
	id, jobID, err := h.Library.Add(r.Context(), m, body.AddOptions, h.Actor(r))
	if err != nil {
		h.Fail(w, err)
		return
	}
	if (body.Follow == nil || *body.Follow) && h.Asks != nil {
		if err := h.Asks.FollowAdded(r.Context(), h.Person(r), id); err != nil {
			log.Printf("api: follow on add: %v", err)
		}
	}
	var t mediafiles.Title
	if err := database.Get(r.Context(), h.Store.Read(), "titles", id, &t); err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	v, err := h.TitleView(r.Context(), t)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	if h.Live {
		h.PublishTitleView(v)
	}
	webserver.WriteJSON(w, http.StatusCreated, map[string]any{"title": v, "job_id": jobID})
}

func (h *TitleHandlers) getTitle(w http.ResponseWriter, r *http.Request) {
	t, ok := h.TitleID(w, r)
	if !ok {
		return
	}
	v, err := h.TitleView(r.Context(), t)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, v)
}

func (h *TitleHandlers) editTitle(w http.ResponseWriter, r *http.Request) {
	t, ok := h.TitleID(w, r)
	if !ok {
		return
	}
	var c struct {
		library.TitleChange
		DetailsSource *string `json:"details_source"`
	}
	if !webserver.Decode(w, r, &c, "send any of keep_up_to_date, pick_rules_id, library_folder_id; for movies release_gate, gate_delay_days; for shows episode_order, episode_group_id, new_seasons, details_source") {
		return
	}
	dry := r.URL.Query().Get("dry_run") == "1" || r.URL.Query().Get("dry_run") == "true"
	if c.DetailsSource != nil {
		if t.Kind != "show" {
			h.ShowError(w, library.ErrShowOnly)
			return
		}
		if !metadata.ValidShowSource(*c.DetailsSource) {
			h.ShowError(w, metadata.ErrUnknownSource)
			return
		}
	}
	// A new episode order renumbers the episodes: the show is refreshed
	// after the edit, and a rename is offered when the numbers moved.
	regroup := c.EpisodeGroupID != nil && !dry && t.Kind == "show" && *c.EpisodeGroupID != strOr(t.EpisodeGroupID)
	var before map[int64][2]int
	if regroup {
		var err error
		if before, err = library.EpisodeNamedNumbers(r.Context(), h.Store.Read(), t.ID); err != nil {
			webserver.Internal(w, "api", err)
			return
		}
	}
	res, err := h.Library.Edit(r.Context(), t.ID, c.TitleChange, dry, h.Actor(r))
	if err != nil {
		h.ShowError(w, err)
		return
	}
	if c.DetailsSource != nil && !dry && *c.DetailsSource != t.MetadataSource {
		if _, err := shows.New(h.Library).SwitchSource(r.Context(), t.ID, *c.DetailsSource, h.Actor(r)); err != nil {
			h.ShowError(w, err)
			return
		}
	}
	renamed := false
	if regroup {
		// A refresh that fails puts the old order back.
		if _, err := shows.New(h.Library).Regroup(r.Context(), t.ID, t.EpisodeGroupID, h.Actor(r)); err != nil {
			h.ShowError(w, err)
			return
		}
		after, err := library.EpisodeNamedNumbers(r.Context(), h.Store.Read(), t.ID)
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		renamed = !maps.Equal(before, after)
	}
	_ = database.Get(r.Context(), h.Store.Read(), "titles", t.ID, &t)
	v, err := h.TitleView(r.Context(), t)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	if !dry && h.Live {
		h.PublishTitleView(v)
	}
	out := map[string]any{"title": v, "changes": res}
	if regroup {
		out["rename_suggested"] = renamed
	}
	webserver.WriteJSON(w, http.StatusOK, out)
}

func (h *TitleHandlers) moveTitle(w http.ResponseWriter, r *http.Request) {
	t, ok := h.TitleID(w, r)
	if !ok {
		return
	}
	var body struct {
		LibraryFolderID int64 `json:"library_folder_id"`
		Apply           bool  `json:"apply"`
	}
	if !webserver.Decode(w, r, &body, `send {"library_folder_id": <id>, "apply": false} for a dry run, then "apply": true`) {
		return
	}
	id, err := h.Library.Move(r.Context(), t.ID, body.LibraryFolderID, body.Apply, h.Actor(r))
	if err != nil {
		h.Fail(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusAccepted, map[string]any{"job_id": id, "dry_run": !body.Apply})
}

// LookupView is a looked-up movie with what the Library knows of it.
type LookupView struct {
	metadata.Movie
	TitleID  int64 `json:"title_id,omitempty"` // set when already in the Library
	NeverAdd bool  `json:"never_add"`
	// Ask is the open request for it, if someone asked.
	Ask *requests.AskBrief `json:"ask,omitempty"`
	// Ask4K is the open request for a 4K copy, if someone asked.
	Ask4K *requests.AskBrief `json:"ask_4k,omitempty"`
	// Has4K: the Library copy has a 4K Version, so no 4K ask is needed.
	Has4K bool `json:"has_4k,omitempty"`
	// Hidden: hidden from browsing; managers still find it here to add it.
	Hidden bool `json:"hidden,omitempty"`
}

// lookupTitles finds movies to add by name, "tmdb:<id>" or "imdb:<tt id>".
func (h *TitleHandlers) lookupTitles(w http.ResponseWriter, r *http.Request) {
	term := strings.TrimSpace(r.URL.Query().Get("term"))
	if term == "" || len(term) > 200 {
		webserver.Fail(w, http.StatusBadRequest, "send ?term=<name>, tmdb:<id> or imdb:<tt id>, and &year=<year> with a name if you know it")
		return
	}
	if h.KidLimited(w, r) {
		return
	}
	var ms []metadata.Movie
	low := strings.ToLower(term)
	byID := strings.HasPrefix(low, "tmdb:") || strings.HasPrefix(low, "imdb:")
	if byID {
		m, err := h.Metadata.Movie(r.Context(), low[:5]+strings.TrimSpace(term[5:]))
		if err != nil && !errors.Is(err, metadata.ErrNotFound) {
			h.Fail(w, err)
			return
		}
		if err == nil {
			ms = []metadata.Movie{m}
		}
	} else {
		year, _ := strconv.Atoi(r.URL.Query().Get("year"))
		var err error
		if ms, err = h.Metadata.Search(r.Context(), term, year); err != nil {
			h.Fail(w, err)
			return
		}
	}
	out := []LookupView{}
	db := h.Store.Read()
	for _, m := range h.KidMovies(r.Context(), h.KidMax(r), ms) {
		v := LookupView{Movie: m}
		if id := m.IDs["tmdb"]; id != "" {
			hidden, drop := HiddenMark(r.Context(), h.Store, "movie", id, m.Genres)
			if drop && !byID { // an id is a link: it still finds the title
				continue
			}
			v.Hidden = hidden
			var err error
			if v.TitleID, err = library.TitleByTMDB(r.Context(), db, id); err != nil {
				webserver.Internal(w, "api", err)
				return
			}
			if v.NeverAdd, err = library.OnNeverAdd(r.Context(), db, id); err != nil {
				webserver.Internal(w, "api", err)
				return
			}
			if v.Ask, err = h.OpenAsk(r.Context(), id, h.Person(r).UserID); err != nil {
				webserver.Internal(w, "api", err)
				return
			}
			if v.Ask4K, err = h.OpenMovieAsk(r.Context(), id, h.Person(r).UserID, true); err != nil {
				webserver.Internal(w, "api", err)
				return
			}
			if v.TitleID != 0 {
				if v.Has4K, err = requests.HasFourK(r.Context(), db, v.TitleID); err != nil {
					webserver.Internal(w, "api", err)
					return
				}
			}
		}
		out = append(out, v)
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"movies": out, "attribution": metadata.Attribution})
}

func (h *TitleHandlers) refreshTitle(w http.ResponseWriter, r *http.Request) {
	t, ok := h.TitleID(w, r)
	if !ok {
		return
	}
	id, _, err := h.Queue.Enqueue(r.Context(), library.KindRefreshTitle, &domain.Subject{Type: "title", ID: t.ID},
		map[string]any{"by_hand": true}, "refresh_title:"+strconv.FormatInt(t.ID, 10), h.Actor(r))
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusAccepted, map[string]any{"job_ids": []int64{id}})
}

// refreshTitles refreshes every movie, show and artist, or only title_ids
// when given.
func (h *TitleHandlers) refreshTitles(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TitleIDs []int64 `json:"title_ids"`
	}
	if r.ContentLength != 0 && !webserver.Decode(w, r, &body, `send {} for every movie or {"title_ids": [1, 2]}`) {
		return
	}
	ids := []int64{}
	if len(body.TitleIDs) == 0 {
		for _, kind := range []string{library.KindRefreshTitles, shows.KindRefreshShows, library.KindRefreshArtists} {
			id, _, err := h.Queue.Enqueue(r.Context(), kind, nil, map[string]any{"by_hand": true}, kind+":by_hand", h.Actor(r))
			if err != nil {
				webserver.Internal(w, "api", err)
				return
			}
			ids = append(ids, id)
		}
		webserver.WriteJSON(w, http.StatusAccepted, map[string]any{"job_ids": ids})
		return
	}
	if len(body.TitleIDs) > 500 {
		webserver.Fail(w, http.StatusBadRequest, "at most 500 title_ids at once")
		return
	}
	for _, tid := range body.TitleIDs {
		var t mediafiles.Title
		if err := database.Get(r.Context(), h.Store.Read(), "titles", tid, &t); err != nil {
			webserver.Fail(w, http.StatusNotFound, "no Title "+strconv.FormatInt(tid, 10))
			return
		}
	}
	for _, tid := range body.TitleIDs {
		id, _, err := h.Queue.Enqueue(r.Context(), library.KindRefreshTitle, &domain.Subject{Type: "title", ID: tid},
			map[string]any{"by_hand": true}, "refresh_title:"+strconv.FormatInt(tid, 10), h.Actor(r))
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		ids = append(ids, id)
	}
	webserver.WriteJSON(w, http.StatusAccepted, map[string]any{"job_ids": ids})
}
