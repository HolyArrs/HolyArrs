package api

import (
	"encoding/json"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// TVDBKeyGroup and TVDBKeyName are where the person's own TVDB key is kept
// in the vault. HolyArrs ships no TVDB key.
const TVDBKeyGroup, TVDBKeyName = "links", "tvdb_key"

func (a *API) getTVDBKey(w http.ResponseWriter, r *http.Request) {
	k, err := a.s.Secrets.Setting(r.Context(), TVDBKeyGroup, TVDBKeyName)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]string{"key": secrets.Mask(k)})
}

// putTVDBKey stores the key; an empty key turns TVDB off.
func (a *API) putTVDBKey(w http.ResponseWriter, r *http.Request) {
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
	if err := a.s.Secrets.SetSetting(r.Context(), TVDBKeyGroup, TVDBKeyName, *body.Key, actor(r)); err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]string{"key": secrets.Mask(*body.Key)})
}
