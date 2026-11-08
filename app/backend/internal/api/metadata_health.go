package api

import (
	"net/http"
)

// MetadataDown is the plain-words notice shown while the metadata server is
// not answering.
const MetadataDown = "Metadata server is down, searches and new artwork will return when it's back"

// metadataHealth answers whether the metadata server is up. The check is
// cheap and cached for about a minute.
func (a *API) metadataHealth(w http.ResponseWriter, r *http.Request) {
	up := a.s.MetadataUp == nil || a.s.MetadataUp(r.Context())
	body := map[string]any{"up": up}
	if !up {
		body["message"] = MetadataDown
	}
	writeJSON(w, http.StatusOK, body)
}
