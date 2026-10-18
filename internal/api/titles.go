package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

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
	case errors.Is(err, library.ErrBadGate), errors.Is(err, library.ErrBadDelay), errors.Is(err, library.ErrNotMovieFolder), errors.Is(err, library.ErrNotShowFolder),
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
	}
	if !decode(w, r, &body, `send {"id": "tmdb:<number>"} with optional library_folder_id, pick_rules_id, keep_up_to_date, release_gate, gate_delay_days, search_on_add, more_versions`) {
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
	// Ask is the open Wishlist ask for it, if someone asked.
	Ask *askBrief `json:"ask,omitempty"`
}

// lookupTitles finds movies to add by name, "tmdb:<id>" or "imdb:<tt id>".
func (a *API) lookupTitles(w http.ResponseWriter, r *http.Request) {
	term := strings.TrimSpace(r.URL.Query().Get("term"))
	if term == "" || len(term) > 200 {
		fail(w, http.StatusBadRequest, "send ?term=<name>, tmdb:<id> or imdb:<tt id>, and &year=<year> with a name if you know it")
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
	out := []PickRulesView{}
	for _, p := range ps {
		k := p.Kinds
		if k == nil {
			k = []string{}
		}
		u, err := library.PickRulesUse(r.Context(), a.s.Store, p.ID)
		if err != nil {
			internal(w, err)
			return
		}
		out = append(out, PickRulesView{ID: p.ID, Name: p.Name, Kinds: k, Preset: p.Preset, InUse: u.Total()})
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
		TMDB   string `json:"tmdb_id"`
		Name   string `json:"name"`
		Reason string `json:"reason"`
	}
	if !decode(w, r, &body, `send {"tmdb_id": "<number>", "name": "...", "reason": "..."}`) {
		return
	}
	if body.TMDB == "" || body.Name == "" {
		fail(w, http.StatusBadRequest, "tmdb_id and name are needed")
		return
	}
	n, err := library.AddNeverAdd(r.Context(), a.s.Store, domain.NeverAdd{SourceIDs: map[string]string{"tmdb": body.TMDB}, Name: body.Name, Reason: body.Reason}, actor(r))
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
	writeJSON(w, http.StatusOK, map[string]any{"from": from.Format("2006-01-02"), "to": to.Format("2006-01-02"), "titles": titles, "dates": es})
}

func (a *API) artwork(w http.ResponseWriter, r *http.Request) {
	if a.s.Library.Artwork == nil {
		http.NotFound(w, r)
		return
	}
	p, ok := a.s.Library.Artwork.Path(r.PathValue("name"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeFile(w, r, p)
}
