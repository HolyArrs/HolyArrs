package api

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/store"
)

// showRoutes serves adding shows, their seasons and episodes, and the show
// settings.
func (a *API) showRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/shows/lookup", a.lookupShows)
	m.HandleFunc("POST /api/v1/shows", a.addShow)
	m.HandleFunc("GET /api/v1/titles/{id}/seasons", a.listSeasons)
	m.HandleFunc("PUT /api/v1/titles/{id}/seasons/{number}", a.setSeason)
	m.HandleFunc("GET /api/v1/titles/{id}/episodes", a.listEpisodes)
	m.HandleFunc("PUT /api/v1/episodes/{id}", a.setEpisode)
	m.HandleFunc("GET /api/v1/settings/shows", a.getShowSettings)
	m.HandleFunc("PUT /api/v1/settings/shows", a.putShowSettings)
}

// showError answers show refusals with a status and a reason; anything
// else goes to libraryError.
func showError(w http.ResponseWriter, err error) {
	var dup *library.ShowAlreadyAddedError
	switch {
	case errors.As(err, &dup):
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "title_id": dup.TitleID})
	case errors.Is(err, library.ErrNoShowFolder), errors.Is(err, library.ErrShowNeverAdd):
		fail(w, http.StatusConflict, err.Error())
	case errors.Is(err, library.ErrBadSeasonChoice), errors.Is(err, library.ErrBadNewSeasons), errors.Is(err, library.ErrBadOrder),
		errors.Is(err, library.ErrBadWait), errors.Is(err, library.ErrNotShowFolder), errors.Is(err, library.ErrMovieOnly),
		errors.Is(err, library.ErrShowOnly), errors.Is(err, metadata.ErrUnknownSource):
		fail(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, library.ErrNotShow), errors.Is(err, library.ErrNoSuchSeason), errors.Is(err, metadata.ErrShowNotFound):
		fail(w, http.StatusNotFound, err.Error())
	case errors.Is(err, metadata.ErrNoTVDBKey), errors.Is(err, library.ErrNoShowSources):
		fail(w, http.StatusServiceUnavailable, err.Error())
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
		fail(w, http.StatusBadRequest, "send ?term=<name> or an id such as tmdb:1399, tvdb:121361, tvmaze:82 or imdb:tt0944947, and &source=tmdb, tvmaze or tvdb")
		return
	}
	source := q.Get("source")
	if source == "" {
		d, err := library.ReadShowDefaults(r.Context(), a.s.Store.Read())
		if err != nil {
			internal(w, err)
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
	var shows []metadata.Show
	low := strings.ToLower(term)
	if k, _, ok := strings.Cut(low, ":"); ok && (k == "tmdb" || k == "tvdb" || k == "tvmaze" || k == "imdb") {
		sh, err := a.s.Library.LookupShow(r.Context(), k+":"+strings.TrimSpace(term[len(k)+1:]), source)
		if err != nil && !errors.Is(err, metadata.ErrShowNotFound) {
			showError(w, err)
			return
		}
		if err == nil {
			shows = []metadata.Show{sh}
		}
	} else {
		p, err := a.showSource(source)
		if err != nil {
			showError(w, err)
			return
		}
		if shows, err = p.SearchShows(r.Context(), term); err != nil {
			showError(w, err)
			return
		}
	}
	out := []ShowResult{}
	for _, sh := range a.kidShows(r.Context(), a.kidMax(r), shows) {
		sh.Episodes = nil
		v := ShowResult{Show: sh}
		var err error
		if v.TitleID, err = library.ShowByIDs(r.Context(), a.s.Store.Read(), sh.IDs); err != nil {
			internal(w, err)
			return
		}
		if v.NeverAdd, err = library.ShowOnNeverAdd(r.Context(), a.s.Store.Read(), sh.IDs); err != nil {
			internal(w, err)
			return
		}
		if v.Ask, err = a.openShowAsk(r.Context(), sh.IDs, a.person(r).UserID); err != nil {
			internal(w, err)
			return
		}
		out = append(out, v)
	}
	res := map[string]any{"shows": out, "source": source}
	if source == "tmdb" {
		res["attribution"] = metadata.Attribution
	}
	writeJSON(w, http.StatusOK, res)
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
		library.ShowAddOptions
		Follow *bool `json:"follow"` // put it on the adder's calendar (default true)
	}
	sent, ok := decodeKeys(w, r, &body, addShowHelp)
	if !ok {
		return
	}
	o := &body.ShowAddOptions
	if err := a.kindDefaultsFor(r.Context(), "show", sent, &o.LibraryFolderID, &o.PickRulesID, &o.KeepUpToDate, &o.SearchOnAdd); err != nil {
		internal(w, err)
		return
	}
	if strings.TrimSpace(body.ID) == "" {
		fail(w, http.StatusBadRequest, addShowHelp)
		return
	}
	id, jobID, err := a.s.Library.AddShow(r.Context(), strings.TrimSpace(body.ID), body.ShowAddOptions, actor(r))
	if err != nil {
		showError(w, err)
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
		a.publishTitleView(v)
	}
	writeJSON(w, http.StatusCreated, map[string]any{"title": v, "job_id": jobID})
}

// showID reads a show Title from the path.
func (a *API) showID(w http.ResponseWriter, r *http.Request) (domain.Title, bool) {
	t, ok := a.titleID(w, r)
	if ok && t.Kind != "show" {
		fail(w, http.StatusNotFound, "that Title is not a show")
		return t, false
	}
	return t, ok
}

func (a *API) listSeasons(w http.ResponseWriter, r *http.Request) {
	t, ok := a.showID(w, r)
	if !ok {
		return
	}
	ss, err := library.Seasons(r.Context(), a.s.Store.Read(), t.ID, time.Now())
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"seasons": ss})
}

const keepHelp = `send {"keep_up_to_date": true or false}`

func (a *API) setSeason(w http.ResponseWriter, r *http.Request) {
	t, ok := a.showID(w, r)
	if !ok {
		return
	}
	n, err := strconv.Atoi(r.PathValue("number"))
	if err != nil || n < 0 {
		fail(w, http.StatusNotFound, "no such season")
		return
	}
	var body struct {
		KeepUpToDate *bool `json:"keep_up_to_date"`
	}
	if !decode(w, r, &body, keepHelp) {
		return
	}
	if body.KeepUpToDate == nil {
		fail(w, http.StatusBadRequest, keepHelp)
		return
	}
	if err := a.s.Library.SetSeason(r.Context(), t.ID, n, *body.KeepUpToDate, actor(r)); err != nil {
		showError(w, err)
		return
	}
	ss, err := library.Seasons(r.Context(), a.s.Store.Read(), t.ID, time.Now())
	if err != nil {
		internal(w, err)
		return
	}
	for _, s := range ss {
		if s.Number == n {
			writeJSON(w, http.StatusOK, s)
			return
		}
	}
	fail(w, http.StatusNotFound, "no such season")
}

// EpisodeSlot is one episode in one Version.
type EpisodeSlot struct {
	ID         int64  `json:"id"`
	Version    string `json:"version"`
	VersionID  int64  `json:"version_id"`
	State      string `json:"state"`
	StateLabel string `json:"state_label"`
	Reason     string `json:"reason,omitempty"`
	FileID     *int64 `json:"file_id,omitempty"`
	// File is the episode's file (path, size, quality, media info), asked for with files=1.
	File *domain.File `json:"file,omitempty"`
}

// EpisodeView is one episode with its Slots.
type EpisodeView struct {
	ID       int64      `json:"id"`
	Season   int        `json:"season"`
	Episode  int        `json:"episode"`
	Absolute *int       `json:"absolute,omitempty"`
	Name     string     `json:"name,omitempty"`
	AirDate  *time.Time `json:"air_date,omitempty"`
	AirDay   string     `json:"air_day,omitempty"`
	Runtime  *int       `json:"runtime,omitempty"`
	// RuntimeMinutes repeats Runtime under the name title views use.
	RuntimeMinutes *int              `json:"runtime_minutes,omitempty"`
	Still          string            `json:"still,omitempty"` // still picture URL (w780)
	KeepUpToDate   bool              `json:"keep_up_to_date"`
	Dropped        bool              `json:"dropped,omitempty"`
	IDs            map[string]string `json:"ids"`
	Slots          []EpisodeSlot     `json:"slots"`
}

func episodeView(it domain.Item) EpisodeView {
	v := EpisodeView{ID: it.ID, Absolute: it.Numbers.Absolute, AirDate: it.AirDate, Runtime: it.Runtime, RuntimeMinutes: it.Runtime, Still: it.Still, KeepUpToDate: it.KeepUpToDate,
		Dropped: it.DroppedAt != nil, IDs: it.SourceIDs, Slots: []EpisodeSlot{}}
	if it.Numbers.Season != nil {
		v.Season = *it.Numbers.Season
	}
	if it.Numbers.Episode != nil {
		v.Episode = *it.Numbers.Episode
	}
	if it.Name != nil {
		v.Name = *it.Name
	}
	if it.AirDay != nil {
		v.AirDay = *it.AirDay
	}
	if v.IDs == nil {
		v.IDs = map[string]string{}
	}
	return v
}

// listEpisodes lists a show's episodes in aired order, one season with
// ?season=<number>.
func (a *API) listEpisodes(w http.ResponseWriter, r *http.Request) {
	t, ok := a.showID(w, r)
	if !ok {
		return
	}
	db := a.s.Store.Read()
	where, args := "title_id = ? AND kind = 'episode'", []any{t.ID}
	if sv := r.URL.Query().Get("season"); sv != "" {
		n, err := strconv.Atoi(sv)
		if err != nil || n < 0 {
			fail(w, http.StatusBadRequest, "season must be a number")
			return
		}
		where += " AND json_extract(numbers, '$.season') = ?"
		args = append(args, n)
	}
	its, err := store.Select[domain.Item](r.Context(), db, "items", where+" ORDER BY json_extract(numbers, '$.season'), json_extract(numbers, '$.episode')", args...)
	if err != nil {
		internal(w, err)
		return
	}
	vs, err := store.Select[domain.Version](r.Context(), db, "versions", "title_id = ? ORDER BY is_main DESC, id", t.ID)
	if err != nil {
		internal(w, err)
		return
	}
	label := map[int64]string{}
	for _, v := range vs {
		label[v.ID] = v.Label
	}
	slots, err := store.Select[domain.Slot](r.Context(), db, "slots", "item_id IN (SELECT id FROM items WHERE title_id = ? AND kind = 'episode') ORDER BY id", t.ID)
	if err != nil {
		internal(w, err)
		return
	}
	// files=1 adds each episode's file, for the episode details on the title page.
	files := map[int64]*domain.File{}
	if r.URL.Query().Get("files") == "1" {
		fs, err := store.Select[domain.File](r.Context(), db, "files", `id IN (SELECT file_id FROM slots WHERE file_id IS NOT NULL AND item_id IN (SELECT id FROM items WHERE title_id = ? AND kind = 'episode'))`, t.ID)
		if err != nil {
			internal(w, err)
			return
		}
		for i := range fs {
			files[fs[i].ID] = &fs[i]
		}
	}
	byItem := map[int64][]EpisodeSlot{}
	for _, s := range slots {
		es := EpisodeSlot{ID: s.ID, Version: label[s.VersionID], VersionID: s.VersionID, State: string(s.State), StateLabel: StateLabels[s.State], FileID: s.FileID}
		if s.FileID != nil {
			es.File = files[*s.FileID]
		}
		if s.LastReason != nil {
			es.Reason = *s.LastReason
		}
		byItem[s.ItemID] = append(byItem[s.ItemID], es)
	}
	trimEpisodeSlots(r.Context(), byItem)
	out := make([]EpisodeView, 0, len(its))
	for _, it := range its {
		v := episodeView(it)
		if sl := byItem[it.ID]; sl != nil {
			v.Slots = sl
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, map[string]any{"episodes": out, "episode_order": t.EpisodeOrder})
}

func (a *API) setEpisode(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		KeepUpToDate *bool `json:"keep_up_to_date"`
	}
	if !decode(w, r, &body, keepHelp) {
		return
	}
	if body.KeepUpToDate == nil {
		fail(w, http.StatusBadRequest, keepHelp)
		return
	}
	it, err := a.s.Library.SetEpisode(r.Context(), id, *body.KeepUpToDate, actor(r))
	if errors.Is(err, store.ErrNotFound) {
		fail(w, http.StatusNotFound, "no such episode")
		return
	}
	if err != nil {
		showError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, episodeView(it))
}

func (a *API) getShowSettings(w http.ResponseWriter, r *http.Request) {
	d, err := library.ReadShowDefaults(r.Context(), a.s.Store.Read())
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (a *API) putShowSettings(w http.ResponseWriter, r *http.Request) {
	d, err := library.ReadShowDefaults(r.Context(), a.s.Store.Read())
	if err != nil {
		internal(w, err)
		return
	}
	if !decode(w, r, &d, "send any of season_choice, new_seasons, episode_order, details_source, keep_specials, episode_wait_minutes") {
		return
	}
	if err := library.SetShowDefaults(r.Context(), a.s.Store, d, actor(r)); err != nil {
		showError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}
