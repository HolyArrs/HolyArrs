package requests

import (
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// StreamingRoutes registers the library check for titles on the owner's
// streaming services. It only lists them; removing goes through the
// title's own delete, which the owner confirms.
func (h *AskHandlers) StreamingRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/settings/asks/streaming-cleanup", h.getCleanup)
	m.HandleFunc("POST /api/v1/settings/asks/streaming-cleanup", h.startCleanup)
}

func (h *AskHandlers) getCleanup(w http.ResponseWriter, r *http.Request) {
	webserver.WriteJSON(w, http.StatusOK, h.Svc.CleanupState())
}

func (h *AskHandlers) startCleanup(w http.ResponseWriter, r *http.Request) {
	c, err := h.Svc.StartCleanup(r.Context())
	if err != nil {
		h.askError(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusAccepted, c)
}
