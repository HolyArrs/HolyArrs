package music

import "net/http"

// Handlers serves the Music routes. Deps grows the route services and
// cross-module funcs as the routes move out of internal/api (t14 split
// design, section 4). Its state fields are private and zero-value ready.
type Handlers struct {
	Deps
}

// Routes registers the Music routes; empty until they move here.
func (h *Handlers) Routes(m *http.ServeMux) {}
