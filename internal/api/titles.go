package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs/internal/asks"
	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/library"
	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// decode reads a JSON body into v; false means an answer was already sent.
func decode(w http.ResponseWriter, r *http.Request, v any, help string) bool {
	err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(v)
	if mb := (*http.MaxBytesError)(nil); errors.As(err, &mb) {
		fail(w, http.StatusRequestEntityTooLarge, "request body too large")
		return false
	}
	if err != nil {
		fail(w, http.StatusBadRequest, help)
		return false
	}
	return true
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
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "title_id": dup.TitleID})
	case errors.As(err, &fe):
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "health": fe.Health.State, "reason": fe.Health.Reason})
	case errors.Is(err, library.ErrNeverAdd), errors.Is(err, library.ErrNoFolder), errors.Is(err, library.ErrFolderInUse),
		errors.Is(err, library.ErrFolderTaken), errors.Is(err, library.ErrMainVersion), errors.Is(err, library.ErrVersionHasFiles), errors.Is(err, library.ErrVersionBusy),
		errors.Is(err, library.ErrVersionFolder), errors.Is(err, library.ErrProtectedCopy):
		fail(w, http.StatusConflict, err.Error())
	case errors.Is(err, library.ErrBadGate), errors.Is(err, library.ErrBadDelay), errors.Is(err, library.ErrNotMovieFolder), errors.Is(err, library.ErrNotShowFolder), errors.Is(err, library.ErrNotMusicFolder),
		errors.Is(err, library.ErrNoPickRules), errors.Is(err, library.ErrNotMusicRules), errors.Is(err, library.ErrNotVideoRules), errors.Is(err, library.ErrFolderPath), errors.Is(err, library.ErrFolderKind),
		errors.Is(err, library.ErrFolderAbsent), errors.Is(err, library.ErrNoMetadataIDs), errors.Is(err, library.ErrVersionLabel),
		errors.Is(err, library.ErrTooManyVersions), errors.Is(err, library.ErrCopyChoice):
		fail(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, store.ErrNotFound):
		fail(w, http.StatusNotFound, "not found")
	case errors.Is(err, metadata.ErrNotFound):
		fail(w, http.StatusNotFound, "no movie with that id")
	case errors.Is(err, metadata.ErrNoAccess):
		fail(w, http.StatusServiceUnavailable, err.Error())
	default:
		internal(w, err)
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
	fail(w, status, message)
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
		internal(w, err)
		return
	}
	if body.ID == "" {
		fail(w, http.StatusBadRequest, `send {"id": "tmdb:<number>"}`)
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
	var t domain.Title
	if err := store.Get(r.Context(), a.s.Store.Read(), "titles", id, &t); err != nil {
		internal(w, err)
		return
	}
	v, err := a.titleView(r.Context(), t)
	if err != nil {
		internal(w, err)
		return
	}
	if a.s.Live != nil {
		a.s.Live.Publish("title", v)
	}
	writeJSON(w, http.StatusCreated, map[string]any{"title": v, "job_id": jobID})
}

func (a *API) getTitle(w http.ResponseWriter, r *http.Request) {
	t, ok := a.titleID(w, r)
	if !ok {
		return
	}
	v, err := a.titleView(r.Context(), t)
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
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
		if _, err := a.s.Library.SwitchSource(r.Context(), t.ID, *c.DetailsSource, actor(r)); err != nil {
			showError(w, err)
			return
		}
	}
	_ = store.Get(r.Context(), a.s.Store.Read(), "titles", t.ID, &t)
	v, err := a.titleView(r.Context(), t)
	if err != nil {
		internal(w, err)
		return
	}
	if !dry && a.s.Live != nil {
		a.s.Live.Publish("title", v)
	}
	writeJSON(w, http.StatusOK, map[string]any{"title": v, "changes": res})
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
	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": id, "dry_run": !body.Apply})
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
		fail(w, http.StatusBadRequest, "send ?term=<name>, tmdb:<id> or imdb:<tt id>, and &year=<year> with a name if you know it")
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
				internal(w, err)
				return
			}
			if v.NeverAdd, err = library.OnNeverAdd(r.Context(), db, id); err != nil {
				internal(w, err)
				return
			}
			if v.Ask, err = a.openAsk(r.Context(), id, a.person(r).UserID); err != nil {
				internal(w, err)
				return
			}
			if v.Ask4K, err = a.openMovieAsk(r.Context(), id, a.person(r).UserID, true); err != nil {
				internal(w, err)
				return
			}
			if v.TitleID != 0 {
				if v.Has4K, err = asks.HasFourK(r.Context(), db, v.TitleID); err != nil {
					internal(w, err)
					return
				}
			}
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, map[string]any{"movies": out, "attribution": metadata.Attribution})
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		fail(w, http.StatusNotFound, "not found")
		return 0, false
	}
	return id, true
}

// FolderView is a Library folder as the API shows it.
type FolderView struct {
	ID               int64      `json:"id"`
	Path             string     `json:"path"`
	Kind             string     `json:"kind"`
	Label            string     `json:"label"`
	DefaultPickRules *int64     `json:"default_pick_rules_id"`
	FreeSpace        int64      `json:"free_space"`
	Health           string     `json:"health"`
	HealthReason     string     `json:"health_reason"`
	HealthCheckedAt  *time.Time `json:"health_checked_at,omitempty"`
}

func folderView(f domain.LibraryFolder) FolderView {
	return FolderView{ID: f.ID, Path: f.Path, Kind: f.Kind, Label: f.Label, DefaultPickRules: f.DefaultPickRules, FreeSpace: f.FreeSpace,
		Health: f.Health, HealthReason: f.HealthReason, HealthCheckedAt: f.HealthCheckedAt}
}

// NeverAddView is a Never-add entry as the API shows it.
type NeverAddView struct {
	ID        int64             `json:"id"`
	Kind      string            `json:"kind"`
	SourceIDs map[string]string `json:"ids"`
	Name      string            `json:"name"`
	Reason    string            `json:"reason"`
	CreatedAt time.Time         `json:"created_at"`
}

func neverAddView(n domain.NeverAdd) NeverAddView {
	return NeverAddView{ID: n.ID, Kind: n.Kind, SourceIDs: n.SourceIDs, Name: n.Name, Reason: n.Reason, CreatedAt: n.CreatedAt}
}

// PickRulesView is a Pick rules set as choosers show it.
type PickRulesView struct {
	ID     int64    `json:"id"`
	Name   string   `json:"name"`
	Kinds  []string `json:"kinds"`
	Preset bool     `json:"preset"`
	// InUse counts every use (Versions, Library folders, Outside lists, ask
	// settings and Movies and TV defaults).
	InUse int `json:"in_use"`
}

func (a *API) listPickRules(w http.ResponseWriter, r *http.Request) {
	ps, err := store.Select[domain.PickRules](r.Context(), a.s.Store.Read(), "pick_rules", "1=1 ORDER BY id")
	if err != nil {
		internal(w, err)
		return
	}
	uses, err := library.PickRulesUses(r.Context(), a.s.Store)
	if err != nil {
		internal(w, err)
		return
	}
	out := []PickRulesView{}
	for _, p := range ps {
		k := p.Kinds
		if k == nil {
			k = []string{}
		}
		out = append(out, PickRulesView{ID: p.ID, Name: p.Name, Kinds: k, Preset: p.Preset, InUse: uses[p.ID].Total()})
	}
	writeJSON(w, http.StatusOK, map[string]any{"pick_rules": out})
}

func (a *API) listFolders(w http.ResponseWriter, r *http.Request) {
	fs, err := store.Select[domain.LibraryFolder](r.Context(), a.s.Store.Read(), "library_folders", "1=1 ORDER BY id")
	if err != nil {
		internal(w, err)
		return
	}
	out := []FolderView{}
	for _, f := range fs {
		out = append(out, folderView(f))
	}
	writeJSON(w, http.StatusOK, map[string]any{"library_folders": out})
}

func (a *API) downloadPaths(r *http.Request) []string {
	ps, _ := library.DownloadPaths(r.Context(), a.s.Store.Read(), a.s.Library.SavePath)
	return ps
}

func (a *API) addFolder(w http.ResponseWriter, r *http.Request) {
	var in library.FolderInput
	if !decode(w, r, &in, `send {"path": "/movies", "kind": "movie", "label": "Movies"}`) {
		return
	}
	lf, err := library.AddFolder(r.Context(), a.s.Store, in, a.downloadPaths(r), actor(r))
	if err != nil {
		libraryError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, folderView(lf))
}

func (a *API) editFolder(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var c library.FolderChange
	if !decode(w, r, &c, `send {"label": "...", "default_pick_rules_id": <id>}`) {
		return
	}
	lf, err := library.UpdateFolder(r.Context(), a.s.Store, id, c, actor(r))
	if err != nil {
		libraryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, folderView(lf))
}

func (a *API) deleteFolder(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := library.DeleteFolder(r.Context(), a.s.Store, id); err != nil {
		libraryError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) checkFolder(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	lf, _, err := library.RecheckFolder(r.Context(), a.s.Store, id, a.downloadPaths(r), actor(r))
	if err != nil {
		libraryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, folderView(lf))
}

func (a *API) listNeverAdd(w http.ResponseWriter, r *http.Request) {
	ns, err := library.NeverAddList(r.Context(), a.s.Store.Read())
	if err != nil {
		internal(w, err)
		return
	}
	out := []NeverAddView{}
	for _, n := range ns {
		out = append(out, neverAddView(n))
	}
	writeJSON(w, http.StatusOK, map[string]any{"never_add": out})
}

func (a *API) addNeverAdd(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Kind   string `json:"kind"`
		TMDB   string `json:"tmdb_id"`
		Name   string `json:"name"`
		Reason string `json:"reason"`
	}
	if !decode(w, r, &body, `send {"tmdb_id": "<number>", "name": "...", "reason": "...", "kind": "movie" or "show"}`) {
		return
	}
	if body.TMDB == "" || body.Name == "" || (body.Kind != "" && body.Kind != "movie" && body.Kind != "show") {
		fail(w, http.StatusBadRequest, "tmdb_id and name are needed; kind is movie (the default) or show")
		return
	}
	n, err := library.AddNeverAdd(r.Context(), a.s.Store, domain.NeverAdd{Kind: body.Kind, SourceIDs: map[string]string{"tmdb": body.TMDB}, Name: body.Name, Reason: body.Reason}, actor(r))
	if err != nil {
		libraryError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, neverAddView(n))
}

func (a *API) deleteNeverAdd(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := library.RemoveNeverAdd(r.Context(), a.s.Store, id); err != nil {
		libraryError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) getGateDefaults(w http.ResponseWriter, r *http.Request) {
	g, err := library.ReadGateDefaults(r.Context(), a.s.Store.Read())
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (a *API) putGateDefaults(w http.ResponseWriter, r *http.Request) {
	var g library.GateDefaults
	if !decode(w, r, &g, `send {"release_gate": "released", "gate_delay_days": 0, "bulk_search_respects_gate": true}`) {
		return
	}
	if err := library.SetGateDefaults(r.Context(), a.s.Store, g, actor(r)); err != nil {
		libraryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (a *API) refreshTitle(w http.ResponseWriter, r *http.Request) {
	t, ok := a.titleID(w, r)
	if !ok {
		return
	}
	id, _, err := a.s.Queue.Enqueue(r.Context(), library.KindRefreshTitle, &domain.Subject{Type: "title", ID: t.ID},
		map[string]any{"by_hand": true}, "refresh_title:"+itoa(t.ID), actor(r))
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job_ids": []int64{id}})
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
		for _, kind := range []string{library.KindRefreshTitles, library.KindRefreshShows, library.KindRefreshArtists} {
			id, _, err := a.s.Queue.Enqueue(r.Context(), kind, nil, map[string]any{"by_hand": true}, kind+":by_hand", actor(r))
			if err != nil {
				internal(w, err)
				return
			}
			ids = append(ids, id)
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"job_ids": ids})
		return
	}
	if len(body.TitleIDs) > 500 {
		fail(w, http.StatusBadRequest, "at most 500 title_ids at once")
		return
	}
	for _, tid := range body.TitleIDs {
		var t domain.Title
		if err := store.Get(r.Context(), a.s.Store.Read(), "titles", tid, &t); err != nil {
			fail(w, http.StatusNotFound, "no Title "+itoa(tid))
			return
		}
	}
	for _, tid := range body.TitleIDs {
		id, _, err := a.s.Queue.Enqueue(r.Context(), library.KindRefreshTitle, &domain.Subject{Type: "title", ID: tid},
			map[string]any{"by_hand": true}, "refresh_title:"+itoa(tid), actor(r))
		if err != nil {
			internal(w, err)
			return
		}
		ids = append(ids, id)
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job_ids": ids})
}

// calendar lists cinema, digital and physical dates of Library movies in
// [from, to); dates are YYYY-MM-DD, default today to today + 30 days.
func (a *API) calendar(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	from := time.Now().UTC().Truncate(24 * time.Hour)
	to := from.AddDate(0, 0, 30)
	var err error
	if v := q.Get("from"); v != "" {
		if from, err = time.Parse("2006-01-02", v); err != nil {
			fail(w, http.StatusBadRequest, "from must be YYYY-MM-DD")
			return
		}
		if q.Get("to") == "" {
			to = from.AddDate(0, 0, 30)
		}
	}
	if v := q.Get("to"); v != "" {
		if to, err = time.Parse("2006-01-02", v); err != nil {
			fail(w, http.StatusBadRequest, "to must be YYYY-MM-DD")
			return
		}
	}
	if !to.After(from) || to.Sub(from) > 366*24*time.Hour {
		fail(w, http.StatusBadRequest, "to must be after from, at most a year later")
		return
	}
	titles, ok := calendarTitles(q.Get("titles"))
	if !ok {
		fail(w, http.StatusBadRequest, "titles must be all, movies, shows or music")
		return
	}
	es, err := library.Calendar(r.Context(), a.s.Store.Read(), from, to, titles)
	if err != nil {
		internal(w, err)
		return
	}
	if max := a.kidMax(r); max != "" {
		ids := make([]int64, len(es))
		for i, e := range es {
			ids[i] = e.TitleID
		}
		ok, err := a.allowedTitleIDs(r.Context(), max, ids)
		if err != nil {
			internal(w, err)
			return
		}
		kept := []library.CalendarEntry{}
		for _, e := range es {
			if ok[e.TitleID] {
				kept = append(kept, e)
			}
		}
		es = kept
	}
	for i := range es {
		if es[i].Poster != "" {
			es[i].Poster = artworkLinks([]string{es[i].Poster})[0]
		}
		// An album carries its artist's genres when already known (read at
		// start for every Library artist); the calendar never looks them up.
		if es[i].Kind == "album" && es[i].ArtistMBID != "" {
			es[i].Genres = a.knownGenres(es[i].ArtistMBID)
		}
	}
	out, mine, people, err := a.calendarFollows(r.Context(), es)
	if err != nil {
		internal(w, err)
		return
	}
	a.calendarArt(r, out)
	writeJSON(w, http.StatusOK, map[string]any{"from": from.Format("2006-01-02"), "to": to.Format("2006-01-02"), "titles": titles, "dates": out,
		"my_follows": mine, "people": people})
}

// knownGenres is an artist's genres from the music genre cache, without
// queueing a lookup (nil when not known).
func (a *API) knownGenres(mbid string) []string {
	c := &a.musicx
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.genres[mbid].genres
}

// calendarRow is a calendar date with who follows its title: following
// says the viewer does; followers are everyone who follows it.
type calendarRow struct {
	library.CalendarEntry
	Following bool               `json:"following,omitempty"`
	Followers []calendarFollower `json:"followers,omitempty"`
	// Banner is the List view's wide picture (textless when a logo goes
	// over it); Logo is the title or artist logo. Both may be empty.
	Banner string `json:"banner,omitempty"`
	Logo   string `json:"logo,omitempty"`
}

// calendarArt adds the List view's banner pictures. Movies and shows get
// the stored card pick (a textless backdrop and the title logo, from
// card_art); albums get the artist's wide picture and logo through our
// metadata server's picture route. Only stored data is read: no outside
// call on the request, and a title without a pick keeps its backdrop.
func (a *API) calendarArt(r *http.Request, rows []calendarRow) {
	lang := trailerLanguage(r)
	ids := map[int64]string{}
	var args []any
	for _, e := range rows {
		if e.Kind != "album" {
			if _, ok := ids[e.TitleID]; !ok {
				ids[e.TitleID] = ""
				args = append(args, e.TitleID)
			}
		}
	}
	db := a.s.Store.Read()
	for len(args) > 0 {
		chunk := args[:min(len(args), 500)]
		args = args[len(chunk):]
		q, err := db.QueryContext(r.Context(), `SELECT id, kind, COALESCE(CAST(json_extract(source_ids, '$.tmdb') AS TEXT), '') FROM titles
			WHERE id IN (?`+strings.Repeat(", ?", len(chunk)-1)+`)`, chunk...)
		if err != nil {
			return
		}
		for q.Next() {
			var id int64
			var kind string
			var tmdb string
			if q.Scan(&id, &kind, &tmdb) == nil {
				ids[id] = kind + ":" + tmdb
			}
		}
		_ = q.Close()
	}
	var keys [][2]string
	for _, k := range ids {
		if kind, id, ok := strings.Cut(k, ":"); ok && id != "" && (kind == "movie" || kind == "show") {
			keys = append(keys, [2]string{kind, id})
		}
	}
	got := a.readCardArt(r.Context(), keys, lang)
	for i := range rows {
		e := &rows[i]
		if e.Kind == "album" {
			if e.ArtistMBID != "" {
				e.Banner = musicArtPrefix + "wide/" + e.ArtistMBID
				e.Logo = musicArtPrefix + "logo/" + e.ArtistMBID
			}
			continue
		}
		kind, id, _ := strings.Cut(ids[e.TitleID], ":")
		row, ok := got[cardKey(kind, id, lang)]
		if !ok {
			row, ok = got[cardKey(kind, id, "en")]
		}
		if !ok {
			continue
		}
		e.Logo = row.Logo
		// A backdrop with the title printed on it is never used under a logo.
		if row.Art != "" && !row.HasTitle {
			e.Banner = row.Art
		}
	}
}

type calendarFollower struct {
	ID     int64   `json:"id"`
	Name   string  `json:"name"`
	Avatar *string `json:"avatar,omitempty"`
	Me     bool    `json:"me,omitempty"`
}

// calendarFollows adds who follows each date's title, and says how many
// titles the viewer follows in all and how many people use this server.
func (a *API) calendarFollows(ctx context.Context, es []library.CalendarEntry) ([]calendarRow, int, int, error) {
	db := a.s.Store.Read()
	me := viewer(ctx)
	var mine, people int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM follows f JOIN titles t ON t.id = f.title_id
		WHERE f.user_id = ? AND t.state != 'removed'`, me).Scan(&mine); err != nil {
		return nil, 0, 0, err
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM users WHERE state = 'active'").Scan(&people); err != nil {
		return nil, 0, 0, err
	}
	type who struct {
		seen []calendarFollower
		me   bool
	}
	by := map[int64]*who{}
	var ids []any
	for _, e := range es {
		if by[e.TitleID] == nil {
			by[e.TitleID] = &who{}
			ids = append(ids, e.TitleID)
		}
	}
	for len(ids) > 0 {
		chunk := ids[:min(len(ids), 500)]
		ids = ids[len(chunk):]
		rows, err := db.QueryContext(ctx, `SELECT f.title_id, u.id, u.name, (SELECT etag FROM user_avatars WHERE user_id = u.id)
			FROM follows f JOIN users u ON u.id = f.user_id
			WHERE u.state != 'disabled' AND f.title_id IN (?`+strings.Repeat(", ?", len(chunk)-1)+`) ORDER BY f.created_at, u.id`, chunk...)
		if err != nil {
			return nil, 0, 0, err
		}
		for rows.Next() {
			var tid, uid int64
			var name string
			var etag sql.NullString
			if err := rows.Scan(&tid, &uid, &name, &etag); err != nil {
				rows.Close()
				return nil, 0, 0, err
			}
			w := by[tid]
			if uid == me {
				w.me = true
			}
			f := calendarFollower{ID: uid, Name: strings.TrimSpace(name), Me: uid == me}
			if etag.Valid && etag.String != "" {
				u := "/api/v1/users/" + strconv.FormatInt(uid, 10) + "/avatar?v=" + etag.String
				f.Avatar = &u
			}
			w.seen = append(w.seen, f)
		}
		if err := rows.Close(); err != nil {
			return nil, 0, 0, err
		}
	}
	out := make([]calendarRow, len(es))
	for i, e := range es {
		w := by[e.TitleID]
		out[i] = calendarRow{CalendarEntry: e, Following: w.me, Followers: w.seen}
	}
	return out, mine, people, nil
}

func (a *API) artwork(w http.ResponseWriter, r *http.Request) {
	if a.s.Library.Artwork == nil {
		http.NotFound(w, r)
		return
	}
	name := r.PathValue("name")
	p, ok := a.s.Library.Artwork.Path(name)
	if !ok {
		http.NotFound(w, r)
		return
	}
	// ?size=250: the small copy for thumbnails (made here once, like album covers).
	if r.URL.Query().Get("size") == "250" {
		if n, sp, err := a.s.Library.Artwork.Smaller(name, 250); err == nil && n != name {
			// A day, not a year: the address stays when the stored picture is replaced.
			serveArtworkCached(w, r, n, sp, "private, max-age=86400")
			return
		}
	}
	serveArtwork(w, r, name, p)
}

// tmdbArtwork serves a TMDB picture (search results, Explore rows, title
// pages) through the artwork cache: fetched once at TMDB's own size (with
// no Referer, through the metadata server's pass-through when TMDB cannot
// be reached), kept on disk at most six months, then sent from here, so
// the browser never calls TMDB itself.
func (a *API) tmdbArtwork(w http.ResponseWriter, r *http.Request) {
	if a.s.Library == nil || a.s.Library.Artwork == nil {
		http.NotFound(w, r)
		return
	}
	src, ok := library.TMDBArtworkURL(metadata.ImageBase, r.PathValue("size"), r.PathValue("file"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	a.serveRemoteArtwork(w, r, src, serveArtwork)
}

// serveRemoteArtwork sends the picture at src from the artwork cache with
// send, fetching it once on a miss (within the person's miss budget).
func (a *API) serveRemoteArtwork(w http.ResponseWriter, r *http.Request, src string, send func(http.ResponseWriter, *http.Request, string, string)) {
	c := a.s.Library.Artwork
	name := library.ArtworkName(src)
	p, ok := c.Path(name)
	if ok {
		c.Recheck(src, nil) // a copy past TMDB's six months is fetched again
	} else {
		if !a.artworkMissOK(w, r) {
			return
		}
		if _, err := c.Fetch(r.Context(), src); err != nil {
			http.Error(w, "picture unavailable", http.StatusBadGateway)
			return
		}
		if p, ok = c.Path(name); !ok {
			http.NotFound(w, r)
			return
		}
	}
	send(w, r, name, p)
}

// serveArtwork sends a cached picture as it is stored (no re-compression).
// Cache names come from the picture's address. Where the address names
// the bytes (TMDB, a versioned server address) browsers keep it a year; a
// picture from a stable address (one that may change in place) is kept a
// day and then revalidated with an ETag that follows the file.
func serveArtwork(w http.ResponseWriter, r *http.Request, name, p string) {
	if library.StableFile(p) {
		if st, err := os.Stat(p); err == nil {
			serveArtworkCached(w, r, strings.TrimSuffix(name, filepath.Ext(name))+"-"+strconv.FormatInt(st.ModTime().Unix(), 36)+filepath.Ext(name), p, "private, max-age=86400")
			return
		}
	}
	serveArtworkCached(w, r, name, p, "private, max-age=31536000, immutable")
}

func serveArtworkCached(w http.ResponseWriter, r *http.Request, name, p, cacheControl string) {
	f, err := os.Open(p)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	h := w.Header()
	h.Set("Cache-Control", cacheControl)
	h.Set("ETag", `"`+strings.TrimSuffix(name, filepath.Ext(name))+`"`)
	if ct := mime.TypeByExtension(filepath.Ext(name)); ct != "" {
		h.Set("Content-Type", ct)
	}
	http.ServeContent(w, r, name, time.Time{}, f)
}
