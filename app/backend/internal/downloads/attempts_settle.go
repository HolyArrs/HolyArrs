package downloads

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// settleAttempts looks again, once the grace has passed, at sends whose outcome
// is unknown, so none waits on a person forever. A job found in the Downloader
// becomes the Download; one the Downloader answers it does not have was never
// sent, and its Slot goes back to wanted with the reason. A Downloader that does
// not answer leaves the record as it is for the next pass.
func (m *Manager) settleAttempts(ctx context.Context) {
	due, err := m.dueAttempts(ctx, m.now().Add(-notArrivedGrace).Format(time.RFC3339Nano), 20)
	if err != nil {
		log.Printf("downloads: could not read unconfirmed sends: %v", err)
		return
	}
	for _, p := range due {
		if ctx.Err() != nil {
			return
		}
		var sl mediafiles.Slot
		if database.Get(ctx, m.Store.Read(), "slots", p[0], &sl) != nil || sl.State != mediafiles.SlotChosen || sl.ChosenReleaseID == nil || *sl.ChosenReleaseID != p[1] {
			continue // a pack send is settled through the Slot still chosen for it
		}
		_, err := m.Grab(ctx, p[0], p[1], domain.System)
		if err == nil || errors.Is(err, ErrUncertain) || errors.Is(err, ErrAttemptStopped) {
			continue
		}
		log.Printf("downloads: warning: a send for Slot %d was not confirmed by its Downloader and goes back to wanted: %s", p[0], firstLine(err.Error()))
		m.giveUp(ctx, p[0], p[1], err.Error(), false, domain.System)
	}
}
