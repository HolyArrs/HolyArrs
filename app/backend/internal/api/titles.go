package api

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"
)

// decode reads a JSON body into v; false means an answer was already sent.
func decode(w http.ResponseWriter, r *http.Request, v any, help string) bool {
	return webserver.Decode(w, r, v, help)
}

// libraryError answers the library's refusals with a status and a reason.
func libraryError(w http.ResponseWriter, err error) {
	if metadataServiceError(w, err) {
		return
	}
	var dup *library.AlreadyAddedError
	var fe *library.FolderError
	switch {
	case errors.As(err, &dup):
		webserver.WriteJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "title_id": dup.TitleID})
	case errors.As(err, &fe):
		webserver.WriteJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "health": fe.Health.State, "reason": fe.Health.Reason})
	case errors.Is(err, library.ErrNeverAdd), errors.Is(err, library.ErrNoFolder), errors.Is(err, library.ErrFolderInUse),
		errors.Is(err, library.ErrFolderTaken), errors.Is(err, library.ErrFolderName), errors.Is(err, library.ErrMainVersion), errors.Is(err, library.ErrVersionHasFiles), errors.Is(err, library.ErrVersionBusy),
		errors.Is(err, library.ErrVersionFolder), errors.Is(err, library.ErrProtectedCopy):
		webserver.Fail(w, http.StatusConflict, err.Error())
	case errors.Is(err, library.ErrBadGate), errors.Is(err, library.ErrBadDelay), errors.Is(err, library.ErrNotMovieFolder), errors.Is(err, library.ErrNotShowFolder), errors.Is(err, library.ErrNotMusicFolder),
		errors.Is(err, library.ErrNoPickRules), errors.Is(err, library.ErrNotMusicRules), errors.Is(err, library.ErrNotVideoRules), errors.Is(err, library.ErrFolderPath), errors.Is(err, library.ErrFolderKind),
		errors.Is(err, library.ErrFolderAbsent), errors.Is(err, library.ErrNoMetadataIDs), errors.Is(err, library.ErrVersionLabel),
		errors.Is(err, library.ErrTooManyVersions), errors.Is(err, library.ErrCopyChoice):
		webserver.Fail(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, database.ErrNotFound):
		webserver.Fail(w, http.StatusNotFound, "not found")
	case errors.Is(err, metadata.ErrNotFound):
		webserver.Fail(w, http.StatusNotFound, "no movie with that id")
	case errors.Is(err, metadata.ErrNoAccess):
		webserver.Fail(w, http.StatusServiceUnavailable, err.Error())
	default:
		webserver.Internal(w, "api", err)
	}
}

// metadataServiceError keeps service failures useful without exposing transport
// or credential details. Direct provider errors retain their existing answers.
func metadataServiceError(w http.ResponseWriter, err error) bool {
	var message string
	status := http.StatusServiceUnavailable
	switch {
	case errors.Is(err, metadata.ErrServiceAccess):
		message = metadata.ErrServiceAccess.Error()
	case errors.Is(err, metadata.ErrServiceUnavailable):
		message = metadata.ErrServiceUnavailable.Error()
	case errors.Is(err, metadata.ErrServiceBusy):
		message = metadata.ErrServiceBusy.Error()
	case errors.Is(err, metadata.ErrServiceIncomplete):
		message = metadata.ErrServiceIncomplete.Error()
	case errors.Is(err, metadata.ErrServiceInvalid):
		status = http.StatusBadGateway
		message = metadata.ErrServiceInvalid.Error()
	default:
		return false
	}
	webserver.Fail(w, status, message)
	return true
}

func (a *API) addTitle(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID string `json:"id"` // "tmdb:693134" or "imdb:tt15239678"
		library.AddOptions
		Follow *bool `json:"follow"` // put it on the adder's calendar (default true)
	}
	sent, ok := decodeKeys(w, r, &body, `send {"id": "tmdb:<number>"} with optional library_folder_id, pick_rules_id, keep_up_to_date, release_gate, gate_delay_days, search_on_add, more_versions, follow`)
	if !ok {
		return
	}
	o := &body.AddOptions
	if err := a.kindDefaultsFor(r.Context(), "movie", sent, &o.LibraryFolderID, &o.PickRulesID, &o.KeepUpToDate, &o.SearchOnAdd); err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	if body.ID == "" {
		webserver.Fail(w, http.StatusBadRequest, `send {"id": "tmdb:<number>"}`)
		return
	}
	m, err := a.s.Metadata.Movie(r.Context(), body.ID)
	if err != nil {
		libraryError(w, err)
		return
	}
	id, jobID, err := a.s.Library.Add(r.Context(), m, body.AddOptions, actor(r))
	if err != nil {
		libraryError(w, err)
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

func (a *API) getTitle(w http.ResponseWriter, r *http.Request) {
	t, ok := a.titleID(w, r)
	if !ok {
		return
	}
	v, err := a.titleView(r.Context(), t)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, v)
}

func (a *API) editTitle(w http.ResponseWriter, r *http.Request) {
	t, ok := a.titleID(w, r)
	if !ok {
		return
	}
	var c struct {
		library.TitleChange
		DetailsSource *string `json:"details_source"`
	}
	if !decode(w, r, &c, "send any of keep_up_to_date, pick_rules_id, library_folder_id; for movies release_gate, gate_delay_days; for shows episode_order, new_seasons, details_source") {
		return
	}
	dry := r.URL.Query().Get("dry_run") == "1" || r.URL.Query().Get("dry_run") == "true"
	if c.DetailsSource != nil {
		if t.Kind != "show" {
			showError(w, library.ErrShowOnly)
			return
		}
		if !metadata.ValidShowSource(*c.DetailsSource) {
			showError(w, metadata.ErrUnknownSource)
			return
		}
	}
	res, err := a.s.Library.Edit(r.Context(), t.ID, c.TitleChange, dry, actor(r))
	if err != nil {
		showError(w, err)
		return
	}
	if c.DetailsSource != nil && !dry && *c.DetailsSource != t.MetadataSource {
		if _, err := shows.New(a.s.Library).SwitchSource(r.Context(), t.ID, *c.DetailsSource, actor(r)); err != nil {
			showError(w, err)
			return
		}
	}
	_ = database.Get(r.Context(), a.s.Store.Read(), "titles", t.ID, &t)
	v, err := a.titleView(r.Context(), t)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	if !dry && a.s.Live != nil {
		a.publishTitleView(v)
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"title": v, "changes": res})
}

func (a *API) moveTitle(w http.ResponseWriter, r *http.Request) {
	t, ok := a.titleID(w, r)
	if !ok {
		return
	}
	var body struct {
		LibraryFolderID int64 `json:"library_folder_id"`
		Apply           bool  `json:"apply"`
	}
	if !decode(w, r, &body, `send {"library_folder_id": <id>, "apply": false} for a dry run, then "apply": true`) {
		return
	}
	id, err := a.s.Library.Move(r.Context(), t.ID, body.LibraryFolderID, body.Apply, actor(r))
	if err != nil {
		libraryError(w, err)
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
	Ask *askBrief `json:"ask,omitempty"`
	// Ask4K is the open request for a 4K copy, if someone asked.
	Ask4K *askBrief `json:"ask_4k,omitempty"`
	// Has4K: the Library copy has a 4K Version, so no 4K ask is needed.
	Has4K bool `json:"has_4k,omitempty"`
}

// lookupTitles finds movies to add by name, "tmdb:<id>" or "imdb:<tt id>".
func (a *API) lookupTitles(w http.ResponseWriter, r *http.Request) {
	term := strings.TrimSpace(r.URL.Query().Get("term"))
	if term == "" || len(term) > 200 {
		webserver.Fail(w, http.StatusBadRequest, "send ?term=<name>, tmdb:<id> or imdb:<tt id>, and &year=<year> with a name if you know it")
		return
	}
	if a.kidLimited(w, r) {
		return
	}
	var ms []metadata.Movie
	low := strings.ToLower(term)
	if strings.HasPrefix(low, "tmdb:") || strings.HasPrefix(low, "imdb:") {
		m, err := a.s.Metadata.Movie(r.Context(), low[:5]+strings.TrimSpace(term[5:]))
		if err != nil && !errors.Is(err, metadata.ErrNotFound) {
			libraryError(w, err)
			return
		}
		if err == nil {
			ms = []metadata.Movie{m}
		}
	} else {
		year, _ := strconv.Atoi(r.URL.Query().Get("year"))
		var err error
		if ms, err = a.s.Metadata.Search(r.Context(), term, year); err != nil {
			libraryError(w, err)
			return
		}
	}
	out := []LookupView{}
	db := a.s.Store.Read()
	for _, m := range a.kidMovies(r.Context(), a.kidMax(r), ms) {
		v := LookupView{Movie: m}
		if id := m.IDs["tmdb"]; id != "" {
			var err error
			if v.TitleID, err = library.TitleByTMDB(r.Context(), db, id); err != nil {
				webserver.Internal(w, "api", err)
				return
			}
			if v.NeverAdd, err = library.OnNeverAdd(r.Context(), db, id); err != nil {
				webserver.Internal(w, "api", err)
				return
			}
			if v.Ask, err = a.openAsk(r.Context(), id, a.person(r).UserID); err != nil {
				webserver.Internal(w, "api", err)
				return
			}
			if v.Ask4K, err = a.openMovieAsk(r.Context(), id, a.person(r).UserID, true); err != nil {
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

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		webserver.Fail(w, http.StatusNotFound, "not found")
		return 0, false
	}
	return id, true
}

func (a *API) refreshTitle(w http.ResponseWriter, r *http.Request) {
	t, ok := a.titleID(w, r)
	if !ok {
		return
	}
	id, _, err := a.s.Queue.Enqueue(r.Context(), library.KindRefreshTitle, &domain.Subject{Type: "title", ID: t.ID},
		map[string]any{"by_hand": true}, "refresh_title:"+itoa(t.ID), actor(r))
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusAccepted, map[string]any{"job_ids": []int64{id}})
}

// refreshTitles refreshes every movie, show and artist, or only title_ids
// when given.
func (a *API) refreshTitles(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TitleIDs []int64 `json:"title_ids"`
	}
	if r.ContentLength != 0 && !decode(w, r, &body, `send {} for every movie or {"title_ids": [1, 2]}`) {
		return
	}
	ids := []int64{}
	if len(body.TitleIDs) == 0 {
		for _, kind := range []string{library.KindRefreshTitles, shows.KindRefreshShows, library.KindRefreshArtists} {
			id, _, err := a.s.Queue.Enqueue(r.Context(), kind, nil, map[string]any{"by_hand": true}, kind+":by_hand", actor(r))
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
		if err := database.Get(r.Context(), a.s.Store.Read(), "titles", tid, &t); err != nil {
			webserver.Fail(w, http.StatusNotFound, "no Title "+itoa(tid))
			return
		}
	}
	for _, tid := range body.TitleIDs {
		id, _, err := a.s.Queue.Enqueue(r.Context(), library.KindRefreshTitle, &domain.Subject{Type: "title", ID: tid},
			map[string]any{"by_hand": true}, "refresh_title:"+itoa(tid), actor(r))
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		ids = append(ids, id)
	}
	webserver.WriteJSON(w, http.StatusAccepted, map[string]any{"job_ids": ids})
}

// calendar lists cinema, digital and physical dates of Library movies in
// [from, to); dates are YYYY-MM-DD, default today to today + 30 days.
// artwork serves a stored Library picture (discover.LibraryArtwork).
func (a *API) artwork(w http.ResponseWriter, r *http.Request) {
	discover.LibraryArtwork(w, r, a.s.Library.Artwork)
}

// tmdbArtwork serves a TMDB picture through the artwork cache (discover.TMDBArtwork).
func (a *API) tmdbArtwork(w http.ResponseWriter, r *http.Request) {
	var c *library.ArtworkCache
	if a.s.Library != nil {
		c = a.s.Library.Artwork
	}
	discover.TMDBArtwork(w, r, c, &a.artMisses)
}

// serveRemoteArtwork is discover.ServeRemoteArtwork on the Library's cache.
func (a *API) serveRemoteArtwork(w http.ResponseWriter, r *http.Request, src string, send func(http.ResponseWriter, *http.Request, string, string)) {
	discover.ServeRemoteArtwork(w, r, a.s.Library.Artwork, &a.artMisses, src, send)
}

func (a *API) serverArtLink(u string) string { return discover.ServerArtLink(a.s.ImageBase, u) }

func (a *API) serverArtwork(w http.ResponseWriter, r *http.Request) {
	var c *library.ArtworkCache
	if a.s.Library != nil {
		c = a.s.Library.Artwork
	}
	discover.ServerArtwork(w, r, a.s.ImageBase, c, &a.artMisses)
}

func serveArtwork(w http.ResponseWriter, r *http.Request, name, p string) {
	library.ServeArtwork(w, r, name, p)
}

func serveArtworkCached(w http.ResponseWriter, r *http.Request, name, p, cacheControl string) {
	library.ServeArtworkCached(w, r, name, p, cacheControl)
}

func (a *API) downloadPaths(r *http.Request) []string {
	ps, _ := library.DownloadPaths(r.Context(), a.s.Store.Read(), a.s.Library.SavePath)
	return ps
}

// FolderView and NeverAddView are the library module's views.
type (
	FolderView   = library.FolderView
	NeverAddView = library.NeverAddView
)
