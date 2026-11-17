package mediaservers

import (
	"encoding/json"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// labelInput is the owner's "Label requested titles" choice as sent.
type labelInput struct {
	Enabled *bool   `json:"enabled"`
	Label   *string `json:"label"`
	Askers  *bool   `json:"askers"`
}

func (h *Handlers) getLabels(w http.ResponseWriter, r *http.Request) {
	webserver.WriteJSON(w, http.StatusOK, ReadLabelSettings(r.Context(), h.Store.Read()))
}

// putLabels saves the Plex label settings; what is left out keeps its value.
func (h *Handlers) putLabels(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	var in labelInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&in); err != nil {
		webserver.Fail(w, http.StatusBadRequest, `send {"enabled": true, "label": "holyarrs", "askers": false}`)
		return
	}
	set := ReadLabelSettings(r.Context(), h.Store.Read())
	if in.Enabled != nil {
		set.Enabled = *in.Enabled
	}
	if in.Askers != nil {
		set.Askers = *in.Askers
	}
	if in.Label != nil {
		lb := strings.Join(strings.Fields(*in.Label), " ")
		if lb == "" || utf8.RuneCountInString(lb) > maxLabel {
			webserver.Fail(w, http.StatusBadRequest, "a label needs 1 to 64 characters")
			return
		}
		set.Label = lb
	}
	if err := SaveLabelSettings(r.Context(), h.Store, set, actorOf(r)); err != nil {
		internalError(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, ReadLabelSettings(r.Context(), h.Store.Read()))
}

// runLabels labels every asked-for title already in the Library, in the
// background.
func (h *Handlers) runLabels(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	if !ReadLabelSettings(r.Context(), h.Store.Read()).Enabled {
		webserver.Fail(w, http.StatusConflict, "turn on Label requested titles first")
		return
	}
	h.Linker.LabelPlexSoon()
	webserver.WriteJSON(w, http.StatusAccepted, map[string]any{"started": true})
}
