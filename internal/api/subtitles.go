package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/subtitles"
	"github.com/CoderLuii/holyarrs/internal/subtitles/sites"
	"github.com/CoderLuii/holyarrs/internal/subtitles/subtext"
)

// subtitleRoutes are the subtitle routes (stage 4.16).
func (a *API) subtitleRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/subtitles/languages", a.subtitleLanguages)
	m.HandleFunc("GET /api/v1/settings/subtitles", a.getSubtitleSettings)
	m.HandleFunc("PUT /api/v1/settings/subtitles", a.putSubtitleSettings)
	m.HandleFunc("GET /api/v1/titles/{id}/subtitles", a.titleSubtitles)
	m.HandleFunc("PUT /api/v1/titles/{id}/subtitle-wish", a.putSubtitleWish)
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
	m.HandleFunc("POST /api/v1/subtitle-sites/{id}/test", a.testSubtitleSite)
}

func subtitleError(w http.ResponseWriter, err error) {
	var fe *sites.Error
	switch {
	case errors.Is(err, subtitles.ErrNotFound):
		fail(w, http.StatusNotFound, err.Error())
	case errors.Is(err, subtitles.ErrMusic), errors.Is(err, subtitles.ErrNotMain), errors.Is(err, subtitles.ErrNotBeside),
		errors.Is(err, subtitles.ErrNotOurs), errors.Is(err, subtext.ErrBadFix), errors.Is(err, subtext.ErrNotSubtitle),
		errors.Is(err, subtitles.ErrNeedsAccount):
		fail(w, http.StatusConflict, err.Error())
	case errors.As(err, &fe):
		fail(w, http.StatusBadGateway, err.Error())
	default:
		fail(w, http.StatusBadRequest, err.Error())
	}
}

func subID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil && id > 0
}

func (a *API) subtitleLanguages(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"languages": subtitles.Languages()})
}

func (a *API) getSubtitleSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, subtitles.ReadSettings(r.Context(), a.s.Store.Read()))
}

func (a *API) putSubtitleSettings(w http.ResponseWriter, r *http.Request) {
	st := subtitles.ReadSettings(r.Context(), a.s.Store.Read()) // fields left out keep their value
	if !decode(w, r, &st, `send any of languages, pass_mark_movie, pass_mark_episode, use_built_in, ignore_picture_tracks, hi_suffix, search_hours, better, better_hours, better_days, batch, adaptive, sync_from_fps, sync_to_fps, sync_step_ms`) {
		return
	}
	if err := subtitles.SaveSettings(r.Context(), a.s.Store, st, actor(r)); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	// New default languages: list what every File now misses.
	_ = a.s.Subtitles.QueueLook(r.Context())
	writeJSON(w, http.StatusOK, subtitles.ReadSettings(r.Context(), a.s.Store.Read()))
}

// SubtitleFileView is one main File with its subtitles.
type SubtitleFileView struct {
	FileID       int64                `json:"file_id"`
	ItemID       int64                `json:"item_id"`
	VersionID    int64                `json:"version_id"`
	RelativePath string               `json:"relative_path"`
	Wish         subtitles.Resolved   `json:"wish"`
	Subtitles    []subtitles.Subtitle `json:"subtitles"`
}

func (a *API) titleSubtitles(w http.ResponseWriter, r *http.Request) {
	t, ok := a.titleID(w, r)
	if !ok {
		return
	}
	ctx, db := r.Context(), a.s.Store.Read()
	wishes, err := subtitles.Wishes(ctx, db, t.ID)
	if err != nil {
		subtitleError(w, err)
		return
	}
	ids, err := subtitles.FilesOf(ctx, db, t.ID, 0)
	if err != nil {
		internal(w, err)
		return
	}
	files := []SubtitleFileView{}
	for _, id := range ids {
		v := SubtitleFileView{FileID: id}
		if err := db.QueryRowContext(ctx, `SELECT f.relative_path, f.version_id, s.item_id FROM files f JOIN slots s ON s.id = json_extract(f.slot_ids, '$[0]') WHERE f.id = ?`,
			id).Scan(&v.RelativePath, &v.VersionID, &v.ItemID); err != nil {
			continue
		}
		v.Wish, _ = subtitles.WishFor(ctx, db, t.ID, v.VersionID, v.ItemID)
		v.Subtitles, _ = subtitles.ListFor(ctx, db, id)
		files = append(files, v)
	}
	writeJSON(w, http.StatusOK, map[string]any{"wishes": wishes, "files": files})
}

func (a *API) putSubtitleWish(w http.ResponseWriter, r *http.Request) {
	t, ok := a.titleID(w, r)
	if !ok {
		return
	}
	var in struct {
		VersionID int64            `json:"version_id"`
		ItemID    int64            `json:"item_id"`
		Languages []subtitles.Want `json:"languages"`
		Inherit   bool             `json:"inherit"`
	}
	if !decode(w, r, &in, `send {"languages": [{"language": "en", "hearing_impaired": "normal", "forced": false}]}, with version_id or item_id for a Version or an episode, or "inherit": true to remove a wish`) {
		return
	}
	if err := subtitles.SetWish(r.Context(), a.s.Store, t.ID, in.VersionID, in.ItemID, in.Languages, in.Inherit, actor(r)); err != nil {
		subtitleError(w, err)
		return
	}
	// Look at the title's Files now (disk only, no site) so its page lists
	// what the new wish misses straight away.
	if ids, err := subtitles.FilesOf(r.Context(), a.s.Store.Read(), t.ID, in.ItemID); err == nil {
		for _, id := range ids {
			_, _ = a.s.Subtitles.Look(r.Context(), id, actor(r))
		}
	}
	ws, _ := subtitles.Wishes(r.Context(), a.s.Store.Read(), t.ID)
	writeJSON(w, http.StatusOK, map[string]any{"wishes": ws})
}

func (a *API) queueSubtitleSearch(ctx context.Context, fileID int64) error {
	return a.s.Subtitles.Queue(ctx, fileID, nil)
}

func (a *API) searchSubtitles(w http.ResponseWriter, r *http.Request) {
	var in struct {
		TitleID int64 `json:"title_id"`
		ItemID  int64 `json:"item_id"`
		FileID  int64 `json:"file_id"`
	}
	if !decode(w, r, &in, `send {"title_id": 1}, optionally with "item_id" for one episode, or {"file_id": 1}`) {
		return
	}
	ctx := r.Context()
	var ids []int64
	switch {
	case in.FileID > 0:
		ids = []int64{in.FileID}
	case in.TitleID > 0:
		var kind string
		if a.s.Store.Read().QueryRowContext(ctx, "SELECT kind FROM titles WHERE id = ?", in.TitleID).Scan(&kind) != nil {
			fail(w, http.StatusNotFound, "no such Title")
			return
		}
		if kind == "artist" {
			subtitleError(w, subtitles.ErrMusic)
			return
		}
		var err error
		if ids, err = subtitles.FilesOf(ctx, a.s.Store.Read(), in.TitleID, in.ItemID); err != nil {
			internal(w, err)
			return
		}
	default:
		fail(w, http.StatusBadRequest, "send a title_id or a file_id")
		return
	}
	for _, id := range ids {
		if err := a.queueSubtitleSearch(ctx, id); err != nil {
			internal(w, err)
			return
		}
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"queued": len(ids), "file_ids": ids, "job_ids": a.subtitleJobIDs(ctx, ids)})
}

// subtitleJobIDs finds the search Job of each File (the one just queued, or
// the one already waiting), so a screen can follow them on the Jobs stream.
func (a *API) subtitleJobIDs(ctx context.Context, fileIDs []int64) []int64 {
	out := []int64{}
	for _, id := range fileIDs {
		var jid int64
		if a.s.Store.Read().QueryRowContext(ctx, "SELECT id FROM jobs WHERE dedupe_key = ? ORDER BY id DESC LIMIT 1",
			subtitles.KindSearch+":"+strconv.FormatInt(id, 10)).Scan(&jid) == nil {
			out = append(out, jid)
		}
	}
	return out
}

func (a *API) missingSubtitles(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	lang := ""
	if v := q.Get("language"); v != "" {
		if lang = subtitles.NormLanguage(v); lang == "" {
			fail(w, http.StatusBadRequest, "unknown language")
			return
		}
	}
	kind := q.Get("kind")
	if kind != "" && kind != "movie" && kind != "episode" {
		fail(w, http.StatusBadRequest, "kind must be movie or episode")
		return
	}
	var titleID int64
	if v := q.Get("title"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			fail(w, http.StatusBadRequest, "title must be a movie or show id")
			return
		}
		titleID = id
	}
	offset, _ := strconv.Atoi(q.Get("offset"))
	ctx, db := r.Context(), a.s.Store.Read()
	rows, total, err := subtitles.MissingList(ctx, db, subtitles.MissingFilter{Language: lang, Kind: kind, TitleID: titleID}, limit(r, 100), max(offset, 0))
	if err != nil {
		internal(w, err)
		return
	}
	titles, err := subtitles.MissingTitles(ctx, db)
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"missing": rows, "total": total, "titles": titles})
}

func (a *API) searchMissingSubtitles(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Language string `json:"language"`
		Kind     string `json:"kind"`
		TitleID  int64  `json:"title_id"`
	}
	if !decode(w, r, &in, `send {} for every missing subtitle, or {"language": "es", "kind": "movie", "title_id": 1}`) {
		return
	}
	lang := ""
	if in.Language != "" {
		if lang = subtitles.NormLanguage(in.Language); lang == "" {
			fail(w, http.StatusBadRequest, "unknown language")
			return
		}
	}
	if in.Kind != "" && in.Kind != "movie" && in.Kind != "episode" {
		fail(w, http.StatusBadRequest, "kind must be movie or episode")
		return
	}
	rows, _, err := subtitles.MissingList(r.Context(), a.s.Store.Read(), subtitles.MissingFilter{Language: lang, Kind: in.Kind, TitleID: in.TitleID}, 1000, 0)
	if err != nil {
		internal(w, err)
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
			internal(w, err)
			return
		}
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"queued": len(ids), "file_ids": ids, "job_ids": a.subtitleJobIDs(r.Context(), ids)})
}

func (a *API) subtitleChoices(w http.ResponseWriter, r *http.Request) {
	id, ok := subID(r)
	if !ok {
		fail(w, http.StatusNotFound, "no such File")
		return
	}
	res, err := a.s.Subtitles.Search(r.Context(), id, subtitles.SearchOptions{Show: true}, actor(r))
	if err != nil {
		subtitleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (a *API) chooseSubtitle(w http.ResponseWriter, r *http.Request) {
	id, ok := subID(r)
	if !ok {
		fail(w, http.StatusNotFound, "no such File")
		return
	}
	var in struct {
		Site string `json:"site"`
		Key  string `json:"key"`
	}
	if !decode(w, r, &in, `send {"site": "Lab subtitle site", "key": "the site's id from the choices"}`) {
		return
	}
	p, err := a.s.Subtitles.Choose(r.Context(), id, in.Site, in.Key, actor(r))
	if err != nil {
		subtitleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": p})
}

func (a *API) syncSubtitle(w http.ResponseWriter, r *http.Request) {
	id, ok := subID(r)
	if !ok {
		fail(w, http.StatusNotFound, "no such subtitle")
		return
	}
	var fix subtext.Fix
	if !decode(w, r, &fix, `send {"offset_ms": 1500} and/or {"from_fps": 25, "to_fps": 23.976}`) {
		return
	}
	row, err := a.s.Subtitles.Sync(r.Context(), id, fix, actor(r))
	if err != nil {
		subtitleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (a *API) turnDownSubtitle(w http.ResponseWriter, r *http.Request) {
	id, ok := subID(r)
	if !ok {
		fail(w, http.StatusNotFound, "no such subtitle")
		return
	}
	var in struct {
		Reason string `json:"reason"`
	}
	if !decode(w, r, &in, `send {"reason": "wrong film"}`) {
		return
	}
	if err := a.s.Subtitles.TurnDown(r.Context(), id, in.Reason, actor(r)); err != nil {
		subtitleError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) listSubtitleSites(w http.ResponseWriter, r *http.Request) {
	rs, err := subtitles.Sites(r.Context(), a.s.Store.Read())
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sites": rs})
}

func (a *API) subtitleSiteKinds(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"kinds": sites.Kinds})
}

func (a *API) addSubtitleSite(w http.ResponseWriter, r *http.Request) {
	var in subtitles.SiteInput
	if !decode(w, r, &in, `send {"kind": "podnapisi"} with optional name, url, enabled, limits, allow_local_network and priority, and the account details (username, password, api_key) a site that needs your account asks for`) {
		return
	}
	row, err := subtitles.AddSite(r.Context(), a.s.Store, a.s.Secrets, in, actor(r))
	if err != nil {
		subtitleError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, row)
}

func (a *API) editSubtitleSite(w http.ResponseWriter, r *http.Request) {
	id, ok := subID(r)
	if !ok {
		fail(w, http.StatusNotFound, "no such Subtitle site")
		return
	}
	var in subtitles.SiteInput
	if !decode(w, r, &in, `send the site's name, url, enabled, limits, allow_local_network and priority; account details only to change them`) {
		return
	}
	row, err := subtitles.EditSite(r.Context(), a.s.Store, a.s.Secrets, id, in, actor(r))
	if err != nil {
		subtitleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (a *API) deleteSubtitleSite(w http.ResponseWriter, r *http.Request) {
	id, ok := subID(r)
	if !ok {
		fail(w, http.StatusNotFound, "no such Subtitle site")
		return
	}
	if err := subtitles.DeleteSite(r.Context(), a.s.Store, id); err != nil {
		subtitleError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) testSubtitleSite(w http.ResponseWriter, r *http.Request) {
	id, ok := subID(r)
	if !ok {
		fail(w, http.StatusNotFound, "no such Subtitle site")
		return
	}
	res, err := a.s.Subtitles.TestSite(r.Context(), id)
	if err != nil {
		subtitleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// newSubtitles makes the subtitle service for a Server without one.
func newSubtitles(s Server) *subtitles.Service {
	sv := &subtitles.Service{Store: s.Store, Vault: s.Secrets}
	if s.Live != nil {
		sv.Live = s.Live
	}
	if q := s.Queue; q != nil {
		sv.Enqueue = func(ctx context.Context, kind string, subj *domain.Subject, params map[string]any, key string) error {
			_, _, err := q.Enqueue(ctx, kind, subj, params, key, domain.System)
			return err
		}
	}
	return sv
}
