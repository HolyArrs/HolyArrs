package api

import (
	"context"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/health"
)

// UISettings is health's answer of GET /settings/ui.
type UISettings = health.UISettings

func (a *API) uiHandlers() *health.UIHandlers {
	h := &health.UIHandlers{Store: a.s.Store, Actor: actor, CountryCount: a.s.CountryCount}
	if a.s.Library != nil && a.s.Library.Artwork != nil {
		h.SetArtworkCap = a.s.Library.Artwork.SetCap
	}
	return h
}

func (a *API) uiSettings(ctx context.Context) UISettings { return health.UISettingsOf(ctx, a.s.Store) }

// applyArtworkCap sets the picture cache's cap from Settings.
func (a *API) applyArtworkCap(ctx context.Context) { a.uiHandlers().ApplyArtworkCap(ctx) }

// ApplyCountryCount passes the stored "Count this install" on to the
// requests to the metadata server (at start and after each change).
func (a *API) ApplyCountryCount(ctx context.Context) { a.uiHandlers().ApplyCountryCount(ctx) }

func (a *API) uiSettingsRoutes(m *http.ServeMux) { a.uiHandlers().Routes(m) }
