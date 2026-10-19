package api

import (
	"encoding/json"
	"net/http"

	"github.com/CoderLuii/holyarrs/internal/auth"
	"github.com/CoderLuii/holyarrs/internal/secrets"
)

// AudioDBKeyGroup and AudioDBKeyName are where the person's own
// TheAudioDB key is kept in the vault. HolyArrs ships no TheAudioDB key:
// its shared test key is for development, so TheAudioDB is an artist
// picture source only with a key of your own.
const AudioDBKeyGroup, AudioDBKeyName = "links", "theaudiodb_key"

func (a *API) getAudioDBKey(w http.ResponseWriter, r *http.Request) {
	k, err := a.s.Secrets.Setting(r.Context(), AudioDBKeyGroup, AudioDBKeyName)
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"key": secrets.Mask(k)})
}

// putAudioDBKey stores the key; an empty key turns TheAudioDB off.
func (a *API) putAudioDBKey(w http.ResponseWriter, r *http.Request) {
	if !auth.RequireStrong(w, r) {
		return
	}
	var body struct {
		Key *string `json:"key"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&body); err != nil || body.Key == nil {
		fail(w, http.StatusBadRequest, `send {"key": "<your own TheAudioDB key>"}, or {"key": ""} to stop using TheAudioDB`)
		return
	}
	if err := a.s.Secrets.SetSetting(r.Context(), AudioDBKeyGroup, AudioDBKeyName, *body.Key, actor(r)); err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"key": secrets.Mask(*body.Key)})
}
