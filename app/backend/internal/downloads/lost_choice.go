package downloads

import (
	"context"
	"fmt"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// GrabKey is the dedupe key of the grab Job for one choice. It names the
// Release, so a grab still waiting for an earlier choice of the same Slot (or
// of a deleted title whose Slot id was reused) never swallows a new one.
func GrabKey(slotID, releaseID int64) string {
	return fmt.Sprintf("grab:%d:%d", slotID, releaseID)
}

// lostChoiceWait is how long a chosen Slot may wait with no grab Job and no
// send on record before it is taken as lost.
const lostChoiceWait = 10 * time.Minute

// lostChoiceReason is shown on a Slot whose choice was never sent.
const lostChoiceReason = "no Downloader was asked to take it; choose it again or let a search pick one"

// notSentEvent records in History that a chosen Release never reached a
// Downloader, with the reason, so the choice does not read as sent.
func (m *Manager) notSentEvent(ctx context.Context, slotID, releaseID int64, reason string, by domain.Actor) {
	db := m.Store.Read()
	data := map[string]any{"reason": reason, "release_id": releaseID}
	var rel domain.Release
	if database.Get(ctx, db, "releases", releaseID, &rel) == nil {
		data["release"] = rel.Name
	}
	e := &domain.Event{Actor: by, Kind: "download_failed", SlotID: &slotID, GroupKey: fmt.Sprintf("slot:%d", slotID), Summary: "Download failed: " + reason, Data: data}
	if id, err := mediafiles.SlotTitleID(ctx, db, slotID); err == nil {
		e.TitleID = &id
	}
	if m.Store.AddEvent(ctx, e) == nil {
		m.publish("event", e)
	}
}
