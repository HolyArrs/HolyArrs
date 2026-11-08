package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"
)

// ListView is an Outside list as the API shows it.
type ListView struct {
	ID              int64      `json:"id"`
	Name            string     `json:"name"`
	Kind            string     `json:"kind"`
	TitleKind       string     `json:"title_kind"`
	Ref             string     `json:"ref"`
	LibraryFolderID int64      `json:"library_folder_id"`
	PickRulesID     *int64     `json:"pick_rules_id"`
	KeepUpToDate    bool       `json:"keep_up_to_date"`
	SearchOnAdd     bool       `json:"search_on_add"`
	ReleaseGate     string     `json:"release_gate"`
	Seasons         string     `json:"seasons"`
	NewSeasons      string     `json:"new_seasons"`
	Enabled         bool       `json:"enabled"`
	SyncEveryHours  int        `json:"sync_every_hours"`
	LastSyncedAt    *time.Time `json:"last_synced_at,omitempty"`
	LastResult      string     `json:"last_result"`
	LastError       string     `json:"last_error"`
	JobID           int64      `json:"job_id,omitempty"` // the sync queued by a save
}

func listView(l domain.OutsideList) ListView {
	return ListView{ID: l.ID, Name: l.Name, Kind: l.Kind, TitleKind: l.TitleKind, Ref: l.Ref, LibraryFolderID: l.LibraryFolderID, PickRulesID: l.PickRulesID,
		KeepUpToDate: l.KeepUpToDate, SearchOnAdd: l.SearchOnAdd, ReleaseGate: l.ReleaseGate, Seasons: l.Seasons, NewSeasons: l.NewSeasons, Enabled: l.Enabled,
		SyncEveryHours: l.SyncEveryHours, LastSyncedAt: l.LastSyncedAt, LastResult: l.LastResult, LastError: l.LastError}
}

// listError answers list refusals; the rest go to libraryError.
func listError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, requests.ErrNameTaken), errors.Is(err, library.ErrNoShowFolder):
		webserver.Fail(w, http.StatusConflict, err.Error())
	case errors.Is(err, requests.ErrName), errors.Is(err, requests.ErrKind), errors.Is(err, requests.ErrRef), errors.Is(err, requests.ErrEvery),
		errors.Is(err, requests.ErrTitleKind), errors.Is(err, requests.ErrSeasons), errors.Is(err, requests.ErrRulesKind):
		webserver.Fail(w, http.StatusBadRequest, err.Error())
	default:
		libraryError(w, err)
	}
}

// syncList queues a read of one list now (Radarr reads a list when it is saved).
func (a *API) syncList(r *http.Request, id int64) (int64, error) {
	jid, _, err := a.s.Queue.Enqueue(r.Context(), requests.OutsideListSync, nil, map[string]any{"list_id": id}, "sync_outside_list:"+itoa(id), actor(r))
	return jid, err
}

func (a *API) listOutsideLists(w http.ResponseWriter, r *http.Request) {
	ls, err := requests.AllOutsideLists(r.Context(), a.s.Store.Read())
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	out := []ListView{}
	for _, l := range ls {
		out = append(out, listView(l))
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"outside_lists": out})
}

const listHelp = `send {"name": "...", "kind": "tmdb_list" or "trakt_list", "ref": "<TMDB list number, Trakt user/list or a link>"} with optional title_kind ("movie" or "show"), library_folder_id, pick_rules_id, keep_up_to_date, search_on_add, release_gate (movies), seasons and new_seasons (shows), enabled, sync_every_hours`

func (a *API) addOutsideList(w http.ResponseWriter, r *http.Request) {
	var in requests.OutsideListInput
	if !decode(w, r, &in, listHelp) {
		return
	}
	l, err := requests.AddOutsideList(r.Context(), a.s.Store, in, actor(r))
	if err != nil {
		listError(w, err)
		return
	}
	v := listView(l)
	if l.Enabled {
		if v.JobID, err = a.syncList(r, l.ID); err != nil {
			webserver.Internal(w, "api", err)
			return
		}
	}
	webserver.WriteJSON(w, http.StatusCreated, v)
}

func (a *API) editOutsideList(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in requests.OutsideListInput
	if !decode(w, r, &in, listHelp) {
		return
	}
	l, err := requests.EditOutsideList(r.Context(), a.s.Store, id, in, actor(r))
	if err != nil {
		listError(w, err)
		return
	}
	v := listView(l)
	if l.Enabled {
		if v.JobID, err = a.syncList(r, l.ID); err != nil {
			webserver.Internal(w, "api", err)
			return
		}
	}
	webserver.WriteJSON(w, http.StatusOK, v)
}

func (a *API) deleteOutsideList(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := requests.RemoveOutsideList(r.Context(), a.s.Store, id); err != nil {
		listError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) syncOutsideList(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if _, err := requests.GetOutsideList(r.Context(), a.s.Store.Read(), id); err != nil {
		listError(w, err)
		return
	}
	jid, err := a.syncList(r, id)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusAccepted, map[string]any{"job_id": jid})
}

// The Trakt app key (client id) is kept encrypted like the TMDB token.
const traktKeyGroup, traktKeyName = "links", "trakt_client_id"

func (a *API) getTraktKey(w http.ResponseWriter, r *http.Request) {
	k, err := a.s.Secrets.Setting(r.Context(), traktKeyGroup, traktKeyName)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]string{"key": secrets.Mask(k)})
}

func (a *API) putTraktKey(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	var body struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&body); err != nil || body.Key == "" {
		webserver.Fail(w, http.StatusBadRequest, `send {"key": "<Trakt app client id>"}`)
		return
	}
	if err := a.s.Secrets.SetSetting(r.Context(), traktKeyGroup, traktKeyName, body.Key, actor(r)); err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]string{"key": secrets.Masked})
}
