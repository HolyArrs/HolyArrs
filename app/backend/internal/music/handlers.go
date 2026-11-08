package music

import (
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// Services are the program services the Music routes use, named as the
// old api.Server fields.
type Services struct {
	Store *database.Store
	// Packages are the kept artist packages; nil: none.
	Packages *PackageSync
}

// Handlers serves the Music routes. Deps points at the caller's flags and
// metadata calls (tests change them after building), Services holds the
// program services. Its state fields are private and zero-value ready.
type Handlers struct {
	*Deps
	Services
	pics picRecords
	side sideStore
}

// Routes registers the Music routes; empty until they move here.
func (h *Handlers) Routes(m *http.ServeMux) {}
