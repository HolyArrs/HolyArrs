package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/CoderLuii/holyarrs/internal/auth"
	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/lists"
	"github.com/CoderLuii/holyarrs/internal/secrets"
)

// ListView is an Outside list as the API shows it.
type ListView struct {
	ID              int64      `json:"id"`
	Name            string     `json:"name"`
	Kind            string     `json:"kind"`
	Ref             string     `json:"ref"`
	LibraryFolderID int64      `json:"library_folder_id"`
	PickRulesID     *int64     `json:"pick_rules_id"`
	KeepUpToDate    bool       `json:"keep_up_to_date"`
	SearchOnAdd     bool       `json:"search_on_add"`
	ReleaseGate     string     `json:"release_gate"`
	Enabled         bool       `json:"enabled"`
	SyncEveryHours  int        `json:"sync_every_hours"`
	LastSyncedAt    *time.Time `json:"last_synced_at,omitempty"`
	LastResult      string     `json:"last_result"`
	LastError       string     `json:"last_error"`
	JobID           int64      `json:"job_id,omitempty"` // the sync queued by a save
}

func listView(l domain.OutsideList) ListView {
	return ListView{ID: l.ID, Name: l.Name, Kind: l.Kind, Ref: l.Ref, LibraryFolderID: l.LibraryFolderID, PickRulesID: l.PickRulesID,
		KeepUpToDate: l.KeepUpToDate, SearchOnAdd: l.SearchOnAdd, ReleaseGate: l.ReleaseGate, Enabled: l.Enabled,
		SyncEveryHours: l.SyncEveryHours, LastSyncedAt: l.LastSyncedAt, LastResult: l.LastResult, LastError: l.LastError}
}

// listError answers list refusals; the rest go to libraryError.
func listError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, lists.ErrNameTaken):
		fail(w, http.StatusConflict, err.Error())
	case errors.Is(err, lists.ErrName), errors.Is(err, lists.ErrKind), errors.Is(err, lists.ErrRef), errors.Is(err, lists.ErrEvery):
		fail(w, http.StatusBadRequest, err.Error())
	default:
		libraryError(w, err)
	}
}

// syncList queues a read of one list now (Radarr reads a list when it is saved).
func (a *API) syncList(r *http.Request, id int64) (int64, error) {
	jid, _, err := a.s.Queue.Enqueue(r.Context(), lists.KindSync, nil, map[string]any{"list_id": id}, "sync_outside_list:"+itoa(id), actor(r))
	return jid, err
}

func (a *API) listOutsideLists(w http.ResponseWriter, r *http.Request) {
	ls, err := lists.All(r.Context(), a.s.Store.Read())
	if err != nil {
		internal(w, err)
		return
	}
	out := []ListView{}
	for _, l := range ls {
		out = append(out, listView(l))
	}
	writeJSON(w, http.StatusOK, map[string]any{"outside_lists": out})
}

const listHelp = `send {"name": "...", "kind": "tmdb_list" or "trakt_list", "ref": "<TMDB list number, Trakt user/list or a link>"} with optional library_folder_id, pick_rules_id, keep_up_to_date, search_on_add, release_gate, enabled, sync_every_hours`

func (a *API) addOutsideList(w http.ResponseWriter, r *http.Request) {
	var in lists.Input
	if !decode(w, r, &in, listHelp) {
		return
	}
	l, err := lists.Add(r.Context(), a.s.Store, in, actor(r))
	if err != nil {
		listError(w, err)
		return
	}
	v := listView(l)
	if l.Enabled {
		if v.JobID, err = a.syncList(r, l.ID); err != nil {
			internal(w, err)
			return
		}
	}
	writeJSON(w, http.StatusCreated, v)
}

func (a *API) editOutsideList(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in lists.Input
	if !decode(w, r, &in, listHelp) {
		return
	}
	l, err := lists.Edit(r.Context(), a.s.Store, id, in, actor(r))
	if err != nil {
		listError(w, err)
		return
	}
	v := listView(l)
	if l.Enabled {
		if v.JobID, err = a.syncList(r, l.ID); err != nil {
			internal(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, v)
}

func (a *API) deleteOutsideList(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := lists.Remove(r.Context(), a.s.Store, id); err != nil {
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
	if _, err := lists.Get(r.Context(), a.s.Store.Read(), id); err != nil {
		listError(w, err)
		return
	}
	jid, err := a.syncList(r, id)
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": jid})
}

// The Trakt app key (client id) is kept encrypted like the TMDB token.
const traktKeyGroup, traktKeyName = "links", "trakt_client_id"

func (a *API) getTraktKey(w http.ResponseWriter, r *http.Request) {
	k, err := a.s.Secrets.Setting(r.Context(), traktKeyGroup, traktKeyName)
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"key": secrets.Mask(k)})
}

func (a *API) putTraktKey(w http.ResponseWriter, r *http.Request) {
	if !auth.RequireStrong(w, r) {
		return
	}
	var body struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&body); err != nil || body.Key == "" {
		fail(w, http.StatusBadRequest, `send {"key": "<Trakt app client id>"}`)
		return
	}
	if err := a.s.Secrets.SetSetting(r.Context(), traktKeyGroup, traktKeyName, body.Key, actor(r)); err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"key": secrets.Masked})
}
