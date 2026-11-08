package mediaservers

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// Handlers serves the media server link routes.
type Handlers struct {
	Store  *database.Store
	Vault  *secrets.Vault
	Linker *Linker
}

// Routes registers the media server routes. Without a vault there is
// nowhere to keep tokens, so none are registered.
func (h *Handlers) Routes(m *http.ServeMux) {
	if h.Vault == nil {
		return
	}
	m.HandleFunc("GET /api/v1/media-servers", h.list)
	m.HandleFunc("POST /api/v1/media-servers", h.add)
	m.HandleFunc("PUT /api/v1/media-servers/{id}", h.edit)
	m.HandleFunc("DELETE /api/v1/media-servers/{id}", h.remove)
	m.HandleFunc("POST /api/v1/media-servers/{id}/test", h.Test)
}

func (h *Handlers) list(w http.ResponseWriter, r *http.Request) {
	ms, err := listServers(r.Context(), h.Store.Read())
	if err != nil {
		internalError(w, err)
		return
	}
	out := []View{}
	for _, m := range ms {
		out = append(out, ViewOf(m))
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"media_servers": out})
}

func (h *Handlers) add(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	in, ok := readInput(w, r)
	if !ok {
		return
	}
	m, err := Add(r.Context(), h.Store, h.Vault, in, actorOf(r))
	if err != nil {
		writeError(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusCreated, ViewOf(m))
}

func (h *Handlers) edit(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	in, ok := readInput(w, r)
	if !ok {
		return
	}
	m, err := Change(r.Context(), h.Store, h.Vault, id, in, actorOf(r))
	if err != nil {
		writeError(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, ViewOf(m))
}

func (h *Handlers) remove(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	if _, err := getServer(r.Context(), h.Store.Read(), id); err != nil {
		writeError(w, err)
		return
	}
	if err := Remove(r.Context(), h.Store, h.Vault, id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Test checks the address and token: Plex must list a movie library,
// Jellyfin and Emby must answer their system info.
func (h *Handlers) Test(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	m, err := getServer(r.Context(), h.Store.Read(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	res, err := h.Linker.Test(ctx, m)
	if err != nil {
		webserver.Fail(w, http.StatusBadGateway, err.Error())
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "found": res})
}

const inputHelp = `send name, kind (plex, jellyfin or emby), url, token, and optionally enabled, on_import, on_rename, on_delete, path_from and path_to, allow_local_network`

func readInput(w http.ResponseWriter, r *http.Request) (Input, bool) {
	var in Input
	err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in)
	if mb := (*http.MaxBytesError)(nil); errors.As(err, &mb) {
		webserver.Fail(w, http.StatusRequestEntityTooLarge, "request body too large")
		return in, false
	}
	if err != nil {
		webserver.Fail(w, http.StatusBadRequest, inputHelp)
		return in, false
	}
	if in.Token == secrets.Masked {
		in.Token = ""
	}
	return in, true
}

func idParam(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		webserver.Fail(w, http.StatusNotFound, "no such media server")
		return 0, false
	}
	return id, true
}

func writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNameUsed):
		webserver.Fail(w, http.StatusConflict, err.Error())
	case IsInputError(err):
		webserver.Fail(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, database.ErrNotFound):
		webserver.Fail(w, http.StatusNotFound, "no such media server")
	default:
		internalError(w, err)
	}
}

// actorOf is the signed-in person; without sign-in (tests) it is the owner.
func actorOf(r *http.Request) domain.Actor {
	if _, ok := webserver.From(r.Context()); ok {
		return webserver.Actor(r.Context())
	}
	return domain.Actor{Kind: "user", ID: 1}
}

func internalError(w http.ResponseWriter, err error) {
	log.Printf("mediaservers: %v", err)
	webserver.Fail(w, http.StatusInternalServerError, "something went wrong; see the log")
}
