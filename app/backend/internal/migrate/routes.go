package migrate

import (
	"net/http"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/auth"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
)

// readTimeout bounds one whole read of an old app.
const readTimeout = 5 * time.Minute

// Handlers serves the migrate routes.
type Handlers struct {
	Store *database.Store
	// Secrets is nil when the app runs without a key store; the routes that
	// keep keys are then left out.
	Secrets *secrets.Vault
	// Limits are the readers' limits; the zero value means the defaults.
	Limits Limits
	// Mover returns the move runner.
	Mover func() *Mover
	// SafeMode says the app started with only the core, which keeps the
	// Library still.
	SafeMode func() bool
}

// Routes registers the migrate routes.
func (h *Handlers) Routes(r *webserver.Router) {
	h.readRoutes(r)
	h.moveRoutes(r)
	if h.Secrets != nil {
		h.secretsFileRoutes(r)
	}
}

func (h *Handlers) mover() *Mover { return h.Mover() }

func (h *Handlers) safeMode() bool { return h.SafeMode() }

// actor is the signed-in person; without sign-in (tests) it is the owner.
func actor(r *http.Request) domain.Actor {
	if _, ok := auth.From(r.Context()); ok {
		return auth.Actor(r.Context())
	}
	return domain.Actor{Kind: "user", ID: 1}
}
