package subtitles

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/sites"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/subtext"
)

// Handlers serves the subtitle HTTP routes.
type Handlers struct {
	store   *database.Store
	secrets *secrets.Vault
	svc     *Service
}

// NewHandlers builds the subtitle routes on top of a store, the key vault and
// the subtitle service.
func NewHandlers(store *database.Store, vault *secrets.Vault, svc *Service) *Handlers {
	return &Handlers{store: store, secrets: vault, svc: svc}
}

// Routes registers the subtitle routes on m.
func (a *Handlers) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/subtitles/languages", a.subtitleLanguages)
	m.HandleFunc("GET /api/v1/settings/subtitles", a.getSubtitleSettings)
	m.HandleFunc("PUT /api/v1/settings/subtitles", a.putSubtitleSettings)
	m.HandleFunc("GET /api/v1/titles/{id}/subtitles", a.titleSubtitles)
	m.HandleFunc("PUT /api/v1/titles/{id}/subtitle-wish", a.putSubtitleWish)
	m.HandleFunc("GET /api/v1/settings/subtitles/translator", a.getTranslator)
	m.HandleFunc("PUT /api/v1/settings/subtitles/translator", a.putTranslator)
	m.HandleFunc("POST /api/v1/settings/subtitles/translator/test", a.testTranslator)
	m.HandleFunc("POST /api/v1/files/{id}/subtitles/translate", a.translateSubtitle)
	m.HandleFunc("POST /api/v1/subtitles/search", a.searchSubtitles)
	m.HandleFunc("GET /api/v1/subtitles/missing", a.missingSubtitles)
	m.HandleFunc("POST /api/v1/subtitles/missing/search", a.searchMissingSubtitles)
	m.HandleFunc("POST /api/v1/files/{id}/subtitles/choices", a.subtitleChoices)
	m.HandleFunc("POST /api/v1/files/{id}/subtitles/choose", a.chooseSubtitle)
	m.HandleFunc("POST /api/v1/subtitles/{id}/sync", a.syncSubtitle)
	m.HandleFunc("POST /api/v1/subtitles/{id}/turn-down", a.turnDownSubtitle)
	m.HandleFunc("GET /api/v1/subtitle-sites", a.listSubtitleSites)
	m.HandleFunc("GET /api/v1/subtitle-sites/kinds", a.subtitleSiteKinds)
	m.HandleFunc("POST /api/v1/subtitle-sites", a.addSubtitleSite)
	m.HandleFunc("PUT /api/v1/subtitle-sites/{id}", a.editSubtitleSite)
	m.HandleFunc("DELETE /api/v1/subtitle-sites/{id}", a.deleteSubtitleSite)
	m.HandleFunc("POST /api/v1/subtitle-sites/{id}/test", a.ServeSiteTest)
}

func subtitleError(w http.ResponseWriter, err error) {
	var fe *sites.Error
	switch {
	case errors.Is(err, ErrNotFound):
		webserver.Fail(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrMusic), errors.Is(err, ErrNotMain), errors.Is(err, ErrNotBeside),
		errors.Is(err, ErrNotOurs), errors.Is(err, subtext.ErrBadFix), errors.Is(err, subtext.ErrNotSubtitle),
		errors.Is(err, ErrNeedsAccount):
		webserver.Fail(w, http.StatusConflict, err.Error())
	case errors.As(err, &fe):
		webserver.Fail(w, http.StatusBadGateway, err.Error())
	default:
		webserver.Fail(w, http.StatusBadRequest, err.Error())
	}
}

func subID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil && id > 0
}

func (a *Handlers) subtitleLanguages(w http.ResponseWriter, r *http.Request) {
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"languages": Languages()})
}

func (a *Handlers) getSubtitleSettings(w http.ResponseWriter, r *http.Request) {
	webserver.WriteJSON(w, http.StatusOK, ReadSettings(r.Context(), a.store.Read()))
}

func (a *Handlers) putSubtitleSettings(w http.ResponseWriter, r *http.Request) {
	st := ReadSettings(r.Context(), a.store.Read()) // fields the request leaves out keep their current value
	if !webserver.Decode(w, r, &st, `send any of languages, pass_mark_movie, pass_mark_episode, use_built_in, ignore_picture_tracks, hi_suffix, search_hours, better, better_hours, better_days, batch, adaptive, sync_from_fps, sync_to_fps, sync_step_ms`) {
		return
	}
	if err := SaveSettings(r.Context(), a.store, st, actorOf(r)); err != nil {
		webserver.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	// Default languages changed, so list what every File now misses.
	_ = a.svc.QueueLook(r.Context())
	webserver.WriteJSON(w, http.StatusOK, ReadSettings(r.Context(), a.store.Read()))
}

// FileView is one main File together with its subtitle state.
type FileView struct {
	FileID       int64      `json:"file_id"`
	ItemID       int64      `json:"item_id"`
	VersionID    int64      `json:"version_id"`
	RelativePath string     `json:"relative_path"`
	Wish         Resolved   `json:"wish"`
	Subtitles    []Subtitle `json:"subtitles"`
}

func (a *Handlers) titleSubtitles(w http.ResponseWriter, r *http.Request) {
	t, ok := a.titleID(w, r)
	if !ok {
		return
	}
	ctx, db := r.Context(), a.store.Read()
	wishes, err := Wishes(ctx, db, t.ID)
	if err != nil {
		subtitleError(w, err)
		return
	}
	ids, err := FilesOf(ctx, db, t.ID, 0)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	files := []FileView{}
	for _, id := range ids {
		v := FileView{FileID: id}
		if err := db.QueryRowContext(ctx, `SELECT f.relative_path, f.version_id, s.item_id FROM files f JOIN slots s ON s.id = json_extract(f.slot_ids, '$[0]') WHERE f.id = ?`,
			id).Scan(&v.RelativePath, &v.VersionID, &v.ItemID); err != nil {
			continue
		}
		v.Wish, _ = WishFor(ctx, db, t.ID, v.VersionID, v.ItemID)
		v.Subtitles, _ = ListFor(ctx, db, id)
		files = append(files, v)
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"wishes": wishes, "files": files})
}

func (a *Handlers) putSubtitleWish(w http.ResponseWriter, r *http.Request) {
	t, ok := a.titleID(w, r)
	if !ok {
		return
	}
	var in struct {
		VersionID int64  `json:"version_id"`
		ItemID    int64  `json:"item_id"`
		Languages []Want `json:"languages"`
		Inherit   bool   `json:"inherit"`
	}
	if !webserver.Decode(w, r, &in, `send {"languages": [{"language": "en", "hearing_impaired": "normal", "forced": false}]}, with version_id or item_id for a Version or an episode, or "inherit": true to remove a wish`) {
		return
	}
	if err := SetWish(r.Context(), a.store, t.ID, in.VersionID, in.ItemID, in.Languages, in.Inherit, actorOf(r)); err != nil {
		subtitleError(w, err)
		return
	}
	// Look at the title's Files right away, from disk only, so its page shows
	// what the new wish is missing without waiting for the schedule.
	if ids, err := FilesOf(r.Context(), a.store.Read(), t.ID, in.ItemID); err == nil {
		for _, id := range ids {
			_, _ = a.svc.Look(r.Context(), id, actorOf(r))
		}
	}
	ws, _ := Wishes(r.Context(), a.store.Read(), t.ID)
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"wishes": ws})
}

func (a *Handlers) queueSubtitleSearch(ctx context.Context, fileID int64) error {
	return a.svc.Queue(ctx, fileID, nil)
}

func (a *Handlers) searchSubtitles(w http.ResponseWriter, r *http.Request) {
	var in struct {
		TitleID int64 `json:"title_id"`
		ItemID  int64 `json:"item_id"`
		FileID  int64 `json:"file_id"`
	}
	if !webserver.Decode(w, r, &in, `send {"title_id": 1}, optionally with "item_id" for one episode, or {"file_id": 1}`) {
		return
	}
	ctx := r.Context()
	var ids []int64
	switch {
	case in.FileID > 0:
		ids = []int64{in.FileID}
	case in.TitleID > 0:
		var kind string
		if a.store.Read().QueryRowContext(ctx, "SELECT kind FROM titles WHERE id = ?", in.TitleID).Scan(&kind) != nil {
			webserver.Fail(w, http.StatusNotFound, "no such Title")
			return
		}
		if kind == "artist" {
			subtitleError(w, ErrMusic)
			return
		}
		var err error
		if ids, err = FilesOf(ctx, a.store.Read(), in.TitleID, in.ItemID); err != nil {
			webserver.Internal(w, "api", err)
			return
		}
	default:
		webserver.Fail(w, http.StatusBadRequest, "send a title_id or a file_id")
		return
	}
	for _, id := range ids {
		if err := a.queueSubtitleSearch(ctx, id); err != nil {
			webserver.Internal(w, "api", err)
			return
		}
	}
	webserver.WriteJSON(w, http.StatusAccepted, map[string]any{"queued": len(ids), "file_ids": ids, "job_ids": a.subtitleJobIDs(ctx, ids)})
}

// subtitleJobIDs finds the search Job for each File, either the one just
// queued or one already waiting, so a screen can follow it on the Jobs stream.
func (a *Handlers) subtitleJobIDs(ctx context.Context, fileIDs []int64) []int64 {
	out := []int64{}
	for _, id := range fileIDs {
		if jid, ok := a.jobByKey(ctx, KindSearch+":"+strconv.FormatInt(id, 10)); ok {
			out = append(out, jid)
		}
	}
	return out
}

// jobByKey finds the newest Job queued under a dedupe key.
func (a *Handlers) jobByKey(ctx context.Context, key string) (int64, bool) {
	var jid int64
	err := a.store.Read().QueryRowContext(ctx, "SELECT id FROM jobs WHERE dedupe_key = ? ORDER BY id DESC LIMIT 1", key).Scan(&jid)
	return jid, err == nil
}

func (a *Handlers) missingSubtitles(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	lang := ""
	if v := q.Get("language"); v != "" {
		if lang = NormLanguage(v); lang == "" {
			webserver.Fail(w, http.StatusBadRequest, "unknown language")
			return
		}
	}
	kind := q.Get("kind")
	if kind != "" && kind != "movie" && kind != "episode" {
		webserver.Fail(w, http.StatusBadRequest, "kind must be movie or episode")
		return
	}
	var titleID int64
	if v := q.Get("title"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			webserver.Fail(w, http.StatusBadRequest, "title must be a movie or show id")
			return
		}
		titleID = id
	}
	offset, _ := strconv.Atoi(q.Get("offset"))
	ctx, db := r.Context(), a.store.Read()
	rows, total, err := MissingList(ctx, db, MissingFilter{Language: lang, Kind: kind, TitleID: titleID}, limitOf(r, 100), max(offset, 0))
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	titles, err := MissingTitles(ctx, db)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"missing": rows, "total": total, "titles": titles})
}

func (a *Handlers) searchMissingSubtitles(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Language string `json:"language"`
		Kind     string `json:"kind"`
		TitleID  int64  `json:"title_id"`
	}
	if !webserver.Decode(w, r, &in, `send {} for every missing subtitle, or {"language": "es", "kind": "movie", "title_id": 1}`) {
		return
	}
	lang := ""
	if in.Language != "" {
		if lang = NormLanguage(in.Language); lang == "" {
			webserver.Fail(w, http.StatusBadRequest, "unknown language")
			return
		}
	}
	if in.Kind != "" && in.Kind != "movie" && in.Kind != "episode" {
		webserver.Fail(w, http.StatusBadRequest, "kind must be movie or episode")
		return
	}
	rows, _, err := MissingList(r.Context(), a.store.Read(), MissingFilter{Language: lang, Kind: in.Kind, TitleID: in.TitleID}, 1000, 0)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	seen := map[int64]bool{}
	ids := []int64{}
	for _, m := range rows {
		if seen[m.FileID] {
			continue
		}
		seen[m.FileID] = true
		ids = append(ids, m.FileID)
		if err := a.queueSubtitleSearch(r.Context(), m.FileID); err != nil {
			webserver.Internal(w, "api", err)
			return
		}
	}
	webserver.WriteJSON(w, http.StatusAccepted, map[string]any{"queued": len(ids), "file_ids": ids, "job_ids": a.subtitleJobIDs(r.Context(), ids)})
}

func (a *Handlers) subtitleChoices(w http.ResponseWriter, r *http.Request) {
	id, ok := subID(r)
	if !ok {
		webserver.Fail(w, http.StatusNotFound, "no such File")
		return
	}
	res, err := a.svc.Search(r.Context(), id, SearchOptions{Show: true}, actorOf(r))
	if err != nil {
		subtitleError(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, res)
}

func (a *Handlers) chooseSubtitle(w http.ResponseWriter, r *http.Request) {
	id, ok := subID(r)
	if !ok {
		webserver.Fail(w, http.StatusNotFound, "no such File")
		return
	}
	var in struct {
		Site string `json:"site"`
		Key  string `json:"key"`
	}
	if !webserver.Decode(w, r, &in, `send {"site": "Lab subtitle site", "key": "the site's id from the choices"}`) {
		return
	}
	p, err := a.svc.Choose(r.Context(), id, in.Site, in.Key, actorOf(r))
	if err != nil {
		subtitleError(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"path": p})
}

func (a *Handlers) syncSubtitle(w http.ResponseWriter, r *http.Request) {
	id, ok := subID(r)
	if !ok {
		webserver.Fail(w, http.StatusNotFound, "no such subtitle")
		return
	}
	var fix subtext.Fix
	if !webserver.Decode(w, r, &fix, `send {"offset_ms": 1500} and/or {"from_fps": 25, "to_fps": 23.976}`) {
		return
	}
	row, err := a.svc.Sync(r.Context(), id, fix, actorOf(r))
	if err != nil {
		subtitleError(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, row)
}

func (a *Handlers) turnDownSubtitle(w http.ResponseWriter, r *http.Request) {
	id, ok := subID(r)
	if !ok {
		webserver.Fail(w, http.StatusNotFound, "no such subtitle")
		return
	}
	var in struct {
		Reason string `json:"reason"`
	}
	if !webserver.Decode(w, r, &in, `send {"reason": "wrong film"}`) {
		return
	}
	if err := a.svc.TurnDown(r.Context(), id, in.Reason, actorOf(r)); err != nil {
		subtitleError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *Handlers) listSubtitleSites(w http.ResponseWriter, r *http.Request) {
	rs, err := Sites(r.Context(), a.store.Read())
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"sites": rs})
}

func (a *Handlers) subtitleSiteKinds(w http.ResponseWriter, r *http.Request) {
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"kinds": sites.Kinds})
}

func (a *Handlers) addSubtitleSite(w http.ResponseWriter, r *http.Request) {
	var in SiteInput
	if !webserver.Decode(w, r, &in, `send {"kind": "podnapisi"} with optional name, url, enabled, limits, allow_local_network and priority, and the account details (username, password, api_key) a site that needs your account asks for`) {
		return
	}
	row, err := AddSite(r.Context(), a.store, a.secrets, in, actorOf(r))
	if err != nil {
		subtitleError(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusCreated, row)
}

func (a *Handlers) editSubtitleSite(w http.ResponseWriter, r *http.Request) {
	id, ok := subID(r)
	if !ok {
		webserver.Fail(w, http.StatusNotFound, "no such Subtitle site")
		return
	}
	var in SiteInput
	if !webserver.Decode(w, r, &in, `send the site's name, url, enabled, limits, allow_local_network and priority; account details only to change them`) {
		return
	}
	row, err := EditSite(r.Context(), a.store, a.secrets, id, in, actorOf(r))
	if err != nil {
		subtitleError(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, row)
}

func (a *Handlers) deleteSubtitleSite(w http.ResponseWriter, r *http.Request) {
	id, ok := subID(r)
	if !ok {
		webserver.Fail(w, http.StatusNotFound, "no such Subtitle site")
		return
	}
	if err := DeleteSite(r.Context(), a.store, id); err != nil {
		subtitleError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ServeSiteTest tests one subtitle site. The reconnect control calls it too.
func (a *Handlers) ServeSiteTest(w http.ResponseWriter, r *http.Request) {
	id, ok := subID(r)
	if !ok {
		webserver.Fail(w, http.StatusNotFound, "no such Subtitle site")
		return
	}
	res, err := a.svc.TestSite(r.Context(), id)
	if err != nil {
		subtitleError(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, res)
}

// NewService builds the subtitle service for a server that was not given one.
func NewService(store *database.Store, vault *secrets.Vault, live *webserver.LiveHub, q *jobs.Queue) *Service {
	sv := &Service{Store: store, Vault: vault}
	if live != nil {
		sv.Live = live
	}
	if q != nil {
		sv.Enqueue = func(ctx context.Context, kind string, subj *domain.Subject, params map[string]any, key string) error {
			_, _, err := q.Enqueue(ctx, kind, subj, params, key, domain.System)
			return err
		}
	}
	return sv
}

// titleID reads the {id} Title. It returns false after it has sent a 404.
func (a *Handlers) titleID(w http.ResponseWriter, r *http.Request) (mediafiles.Title, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var t mediafiles.Title
	if err == nil {
		err = database.Get(r.Context(), a.store.Read(), "titles", id, &t)
	}
	if err != nil {
		webserver.Fail(w, http.StatusNotFound, "no such Title")
		return t, false
	}
	return t, true
}

// actorOf returns the signed-in caller, or the owner when the routes run
// without sign-in, as in tests and first-run setup.
func actorOf(r *http.Request) domain.Actor {
	if _, ok := webserver.From(r.Context()); ok {
		return webserver.Actor(r.Context())
	}
	return domain.Actor{Kind: "user", ID: 1}
}

// limitOf returns the ?limit= value, or def when it is missing or out of range.
func limitOf(r *http.Request, def int) int {
	n, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || n <= 0 || n > 500 {
		return def
	}
	return n
}
