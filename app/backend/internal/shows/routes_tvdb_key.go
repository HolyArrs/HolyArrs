package shows

import (
	"encoding/json"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
)

// TVDBKeyGroup and TVDBKeyName are where the person's own TVDB key is kept
// in the vault. HolyArrs ships no TVDB key.
const TVDBKeyGroup, TVDBKeyName = "links", "tvdb_key"

// TVDBKeyHandlers serves the person's own TVDB key setting.
type TVDBKeyHandlers struct {
	Secrets *secrets.Vault
}

// Routes registers GET and PUT /api/v1/settings/tvdb-key.
func (h *TVDBKeyHandlers) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/settings/tvdb-key", h.getTVDBKey)
	m.HandleFunc("PUT /api/v1/settings/tvdb-key", h.putTVDBKey)
}

func (h *TVDBKeyHandlers) getTVDBKey(w http.ResponseWriter, r *http.Request) {
	k, err := h.Secrets.Setting(r.Context(), TVDBKeyGroup, TVDBKeyName)
	if err != nil {
		webserver.Internal(w, "shows", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]string{"key": secrets.Mask(k)})
}

// putTVDBKey stores the key; an empty key turns TVDB off.
func (h *TVDBKeyHandlers) putTVDBKey(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	var body struct {
		Key *string `json:"key"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&body); err != nil || body.Key == nil {
		webserver.Fail(w, http.StatusBadRequest, `send {"key": "<your own TVDB key>"}, or {"key": ""} to stop using TVDB`)
		return
	}
	if err := h.Secrets.SetSetting(r.Context(), TVDBKeyGroup, TVDBKeyName, *body.Key, actor(r)); err != nil {
		webserver.Internal(w, "shows", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]string{"key": secrets.Mask(*body.Key)})
}

// actor is the signed-in person, or the owner when no session is attached.
func actor(r *http.Request) domain.Actor {
	if _, ok := webserver.From(r.Context()); ok {
		return webserver.Actor(r.Context())
	}
	return domain.Actor{Kind: "user", ID: 1}
}
