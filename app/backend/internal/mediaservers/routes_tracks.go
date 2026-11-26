package mediaservers

import (
	"encoding/json"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// trackInput is the owner's default track choice as sent.
type trackInput struct {
	Enabled   *bool    `json:"enabled"`
	Audio     []string `json:"audio"`
	Subtitles []string `json:"subtitles"`
}

func (h *Handlers) getTracks(w http.ResponseWriter, r *http.Request) {
	webserver.WriteJSON(w, http.StatusOK, ReadTrackSettings(r.Context(), h.Store.Read()))
}

// putTracks saves the default track choice; what is left out keeps its
// value. Saving it switched on sets the tracks now, in the background.
func (h *Handlers) putTracks(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	var in trackInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&in); err != nil {
		webserver.Fail(w, http.StatusBadRequest, `send {"enabled": true, "audio": ["en"], "subtitles": ["en", "off"]}`)
		return
	}
	set := ReadTrackSettings(r.Context(), h.Store.Read())
	if in.Enabled != nil {
		set.Enabled = *in.Enabled
	}
	var err error
	if in.Audio != nil {
		if set.Audio, err = CleanTrackLangs(h.Linker.Langs, in.Audio, false); err != nil {
			webserver.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if in.Subtitles != nil {
		if set.Subtitles, err = CleanTrackLangs(h.Linker.Langs, in.Subtitles, true); err != nil {
			webserver.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if set.Enabled && len(set.Audio) == 0 && len(set.Subtitles) == 0 {
		webserver.Fail(w, http.StatusBadRequest, "pick at least one language")
		return
	}
	if err := SaveTrackSettings(r.Context(), h.Store, set, actorOf(r)); err != nil {
		internalError(w, err)
		return
	}
	if set.Enabled {
		h.Linker.SetDefaultTracksSoon()
	}
	webserver.WriteJSON(w, http.StatusOK, ReadTrackSettings(r.Context(), h.Store.Read()))
}
