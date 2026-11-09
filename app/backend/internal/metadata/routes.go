package metadata

import (
	"context"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// Down is the plain-words notice shown while the metadata server is not
// answering.
const Down = "Metadata server is down, searches and new artwork will return when it's back"

// Handlers serves the metadata routes that need no other module.
type Handlers struct {
	// Up reports whether the metadata server answers; nil counts as up.
	Up func(context.Context) bool
}

// Routes registers the metadata routes.
func (h *Handlers) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/metadata/health", h.health)
}

// health answers whether the metadata server is up. The check is cheap and
// cached for about a minute.
func (h *Handlers) health(w http.ResponseWriter, r *http.Request) {
	up := h.Up == nil || h.Up(r.Context())
	body := map[string]any{"up": up}
	if !up {
		body["message"] = Down
	}
	webserver.WriteJSON(w, http.StatusOK, body)
}
