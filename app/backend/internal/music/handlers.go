package music

import (
	"context"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
)

// Services are the program services the Music routes use, named as the
// old api.Server fields.
type Services struct {
	Store *database.Store
	// Packages are the kept artist packages; nil: none.
	Packages *PackageSync
	Queue    *jobs.Queue

	// Funcs owned by other modules (shows), filled by the caller.

	// SlotOfTitle says a track item belongs to the title.
	SlotOfTitle func(ctx context.Context, itemID, titleID int64) bool
	// SeasonLabel names a part's title and season ("S01").
	SeasonLabel func(ctx context.Context, partID int64) (int64, string, string)
}

// Handlers serves the Music routes. Deps points at the caller's flags and
// metadata calls (tests change them after building), Services holds the
// program services. Its state fields are private and zero-value ready.
type Handlers struct {
	*Deps
	Services
	pics    picRecords
	side    sideStore
	artWarm artWarmer
}

// Routes registers the Music routes; empty until they move here.
func (h *Handlers) Routes(m *http.ServeMux) {}
