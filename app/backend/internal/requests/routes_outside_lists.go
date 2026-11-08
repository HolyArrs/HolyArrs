package requests

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// OutsideListHandlers are the Outside list routes and the Trakt app key.
type OutsideListHandlers struct {
	Store   *database.Store
	Queue   *jobs.Queue
	Secrets *secrets.Vault
	// Actor is the signed-in person (the owner without sign-in).
	Actor func(r *http.Request) domain.Actor
	// LibraryError answers the Library's refusals.
	LibraryError func(w http.ResponseWriter, err error)
}

// Routes registers the Outside list routes.
func (h *OutsideListHandlers) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/outside-lists", h.listOutsideLists)
	m.HandleFunc("POST /api/v1/outside-lists", h.addOutsideList)
	m.HandleFunc("PUT /api/v1/outside-lists/{id}", h.editOutsideList)
	m.HandleFunc("DELETE /api/v1/outside-lists/{id}", h.deleteOutsideList)
	m.HandleFunc("POST /api/v1/outside-lists/{id}/sync", h.syncOutsideList)
}

// TraktKeyRoutes registers the Trakt app key routes (needs Secrets).
func (h *OutsideListHandlers) TraktKeyRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/settings/trakt-key", h.getTraktKey)
	m.HandleFunc("PUT /api/v1/settings/trakt-key", h.putTraktKey)
}

func listPathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		webserver.Fail(w, http.StatusNotFound, "not found")
		return 0, false
	}
	return id, true
}

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
func (h *OutsideListHandlers) listError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNameTaken), errors.Is(err, library.ErrNoShowFolder):
		webserver.Fail(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrName), errors.Is(err, ErrKind), errors.Is(err, ErrRef), errors.Is(err, ErrEvery),
		errors.Is(err, ErrTitleKind), errors.Is(err, ErrSeasons), errors.Is(err, ErrRulesKind):
		webserver.Fail(w, http.StatusBadRequest, err.Error())
	default:
		h.LibraryError(w, err)
	}
}

// syncList queues a read of one list now (Radarr reads a list when it is saved).
func (h *OutsideListHandlers) syncList(r *http.Request, id int64) (int64, error) {
	jid, _, err := h.Queue.Enqueue(r.Context(), OutsideListSync, nil, map[string]any{"list_id": id}, "sync_outside_list:"+strconv.FormatInt(id, 10), h.Actor(r))
	return jid, err
}

func (h *OutsideListHandlers) listOutsideLists(w http.ResponseWriter, r *http.Request) {
	ls, err := AllOutsideLists(r.Context(), h.Store.Read())
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

func (h *OutsideListHandlers) addOutsideList(w http.ResponseWriter, r *http.Request) {
	var in OutsideListInput
	if !webserver.Decode(w, r, &in, listHelp) {
		return
	}
	l, err := AddOutsideList(r.Context(), h.Store, in, h.Actor(r))
	if err != nil {
		h.listError(w, err)
		return
	}
	v := listView(l)
	if l.Enabled {
		if v.JobID, err = h.syncList(r, l.ID); err != nil {
			webserver.Internal(w, "api", err)
			return
		}
	}
	webserver.WriteJSON(w, http.StatusCreated, v)
}

func (h *OutsideListHandlers) editOutsideList(w http.ResponseWriter, r *http.Request) {
	id, ok := listPathID(w, r)
	if !ok {
		return
	}
	var in OutsideListInput
	if !webserver.Decode(w, r, &in, listHelp) {
		return
	}
	l, err := EditOutsideList(r.Context(), h.Store, id, in, h.Actor(r))
	if err != nil {
		h.listError(w, err)
		return
	}
	v := listView(l)
	if l.Enabled {
		if v.JobID, err = h.syncList(r, l.ID); err != nil {
			webserver.Internal(w, "api", err)
			return
		}
	}
	webserver.WriteJSON(w, http.StatusOK, v)
}

func (h *OutsideListHandlers) deleteOutsideList(w http.ResponseWriter, r *http.Request) {
	id, ok := listPathID(w, r)
	if !ok {
		return
	}
	if err := RemoveOutsideList(r.Context(), h.Store, id); err != nil {
		h.listError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *OutsideListHandlers) syncOutsideList(w http.ResponseWriter, r *http.Request) {
	id, ok := listPathID(w, r)
	if !ok {
		return
	}
	if _, err := GetOutsideList(r.Context(), h.Store.Read(), id); err != nil {
		h.listError(w, err)
		return
	}
	jid, err := h.syncList(r, id)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusAccepted, map[string]any{"job_id": jid})
}

// The Trakt app key (client id) is kept encrypted like the TMDB token.
const traktKeyGroup, traktKeyName = metadata.TraktKeyGroup, metadata.TraktKeyName

func (h *OutsideListHandlers) getTraktKey(w http.ResponseWriter, r *http.Request) {
	k, err := h.Secrets.Setting(r.Context(), traktKeyGroup, traktKeyName)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]string{"key": secrets.Mask(k)})
}

func (h *OutsideListHandlers) putTraktKey(w http.ResponseWriter, r *http.Request) {
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
	if err := h.Secrets.SetSetting(r.Context(), traktKeyGroup, traktKeyName, body.Key, h.Actor(r)); err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]string{"key": secrets.Masked})
}
