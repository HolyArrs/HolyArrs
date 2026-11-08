// Package setup holds the state of the first-run wizard. Creating the owner
// and the sign-in that starts belong to accounts; this package only says
// whether the wizard is still open.
package setup

import (
	"context"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

const (
	settingsGroup = "this_app"
	settingDone   = "first_run_done"
)

// Service reads the wizard state.
type Service struct {
	Store *database.Store
}

// New returns a Service over the main database.
func New(st *database.Store) *Service { return &Service{Store: st} }

// Open says whether the first-run wizard is still open: it stays open until
// the owner finishes it.
func (s *Service) Open(ctx context.Context) (bool, error) {
	v, err := database.Setting(ctx, s.Store.Read(), settingsGroup, settingDone)
	if err != nil {
		return false, err
	}
	return v != "1", nil
}
