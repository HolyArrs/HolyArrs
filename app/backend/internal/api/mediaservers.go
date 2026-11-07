package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/auth"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediaserver"
)

// mediaServerRoutes are the media server link routes (5.1.14).
func (a *API) mediaServerRoutes(m *http.ServeMux) {
	if a.s.Secrets == nil {
		return
	}
	m.HandleFunc("GET /api/v1/media-servers", a.listMediaServers)
	m.HandleFunc("POST /api/v1/media-servers", a.addMediaServer)
	m.HandleFunc("PUT /api/v1/media-servers/{id}", a.editMediaServer)
	m.HandleFunc("DELETE /api/v1/media-servers/{id}", a.deleteMediaServer)
	m.HandleFunc("POST /api/v1/media-servers/{id}/test", a.testMediaServer)
}

// MediaServerView is a media server with its token masked.
type MediaServerView struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	Kind       string     `json:"kind"`
	URL        string     `json:"url"`
	Token      string     `json:"token"`
	Enabled    bool       `json:"enabled"`
	OnImport   bool       `json:"on_import"`
	OnRename   bool       `json:"on_rename"`
	OnDelete   bool       `json:"on_delete"`
	PathFrom   string     `json:"path_from"`
	PathTo     string     `json:"path_to"`
	AllowLocal bool       `json:"allow_local_network"`
	LastOKAt   *time.Time `json:"last_ok_at,omitempty"`
	LastError  string     `json:"last_error,omitempty"`
}

func mediaServerView(m domain.MediaServer) MediaServerView {
	v := MediaServerView{ID: m.ID, Name: m.Name, Kind: m.Kind, URL: m.URL, Enabled: m.Enabled, OnImport: m.OnImport, OnRename: m.OnRename,
		OnDelete: m.OnDelete, PathFrom: m.PathFrom, PathTo: m.PathTo, AllowLocal: m.AllowLocal, LastOKAt: m.LastOKAt, LastError: m.LastError}
	if m.TokenSecretID != nil {
		v.Token = secrets.Masked
	}
	return v
}

func mediaServerError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, mediaserver.ErrNameUsed):
		fail(w, http.StatusConflict, err.Error())
	case mediaserver.IsInputError(err):
		fail(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, database.ErrNotFound):
		fail(w, http.StatusNotFound, "no such media server")
	default:
		internal(w, err)
	}
}

const mediaServerHelp = `send name, kind (plex, jellyfin or emby), url, token, and optionally enabled, on_import, on_rename, on_delete, path_from and path_to, allow_local_network`

func readMediaServer(w http.ResponseWriter, r *http.Request) (mediaserver.Input, bool) {
	var in mediaserver.Input
	if !decode(w, r, &in, mediaServerHelp) {
		return in, false
	}
	if in.Token == secrets.Masked {
		in.Token = ""
	}
	return in, true
}

func (a *API) listMediaServers(w http.ResponseWriter, r *http.Request) {
	ms, err := database.Select[domain.MediaServer](r.Context(), a.s.Store.Read(), "media_servers", "1=1 ORDER BY name")
	if err != nil {
		internal(w, err)
		return
	}
	out := []MediaServerView{}
	for _, m := range ms {
		out = append(out, mediaServerView(m))
	}
	writeJSON(w, http.StatusOK, map[string]any{"media_servers": out})
}

func (a *API) addMediaServer(w http.ResponseWriter, r *http.Request) {
	if !auth.RequireStrong(w, r) {
		return
	}
	in, ok := readMediaServer(w, r)
	if !ok {
		return
	}
	m, err := mediaserver.Add(r.Context(), a.s.Store, a.s.Secrets, in, actor(r))
	if err != nil {
		mediaServerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, mediaServerView(m))
}

func mediaServerID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		fail(w, http.StatusNotFound, "no such media server")
		return 0, false
	}
	return id, true
}

func (a *API) editMediaServer(w http.ResponseWriter, r *http.Request) {
	if !auth.RequireStrong(w, r) {
		return
	}
	id, ok := mediaServerID(w, r)
	if !ok {
		return
	}
	in, ok := readMediaServer(w, r)
	if !ok {
		return
	}
	m, err := mediaserver.Change(r.Context(), a.s.Store, a.s.Secrets, id, in, actor(r))
	if err != nil {
		mediaServerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, mediaServerView(m))
}

func (a *API) deleteMediaServer(w http.ResponseWriter, r *http.Request) {
	if !auth.RequireStrong(w, r) {
		return
	}
	id, ok := mediaServerID(w, r)
	if !ok {
		return
	}
	var m domain.MediaServer
	if err := database.Get(r.Context(), a.s.Store.Read(), "media_servers", id, &m); err != nil {
		mediaServerError(w, err)
		return
	}
	if err := mediaserver.Remove(r.Context(), a.s.Store, id); err != nil {
		mediaServerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// testMediaServer checks the address and token: Plex must list a movie
// library, Jellyfin and Emby must answer their system info.
func (a *API) testMediaServer(w http.ResponseWriter, r *http.Request) {
	if !auth.RequireStrong(w, r) {
		return
	}
	id, ok := mediaServerID(w, r)
	if !ok {
		return
	}
	var m domain.MediaServer
	if err := database.Get(r.Context(), a.s.Store.Read(), "media_servers", id, &m); err != nil {
		mediaServerError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	res, err := a.s.Media.Test(ctx, m)
	if err != nil {
		fail(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "found": res})
}
