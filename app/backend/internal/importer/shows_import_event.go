package importer

import (
	"context"
	"fmt"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// noteCleanupWait: a partly done torrent on a Downloader that removes after
// seeding stays until the rest is in; History names what it waits for.
func (im *Importer) noteCleanupWait(ctx context.Context, d domain.Download, slots map[int64]*showSlot, e *domain.Event) {
	var dr domain.Downloader
	if database.Get(ctx, im.Store.Read(), "downloaders", d.DownloaderID, &dr) != nil || dr.Protocol != "torrent" || dr.AfterImport != "remove" {
		return
	}
	wait, err := mediafiles.CleanupWaiting(ctx, im.Store.Read(), map[int64][]int64{d.ID: d.SlotIDs})
	if err != nil || len(wait[d.ID]) == 0 {
		return
	}
	labels := []string{}
	for _, id := range wait[d.ID] {
		if ss, ok := slots[id]; ok {
			labels = append(labels, refLabel(ss.it))
		}
	}
	e.Data["cleanup_wait"] = labels
}

// showImportSummary is the History line of a show import, and the Slots it
// placed.
func showImportSummary(state string, placed int, unpackWhy string, slotIDs []int64, slots map[int64]*showSlot) (string, []int64) {
	var placedLabels []string
	placedIDs := []int64{}
	for _, id := range slotIDs {
		if slots[id].outcome == "placed" {
			placedLabels = append(placedLabels, refLabel(slots[id].it))
			placedIDs = append(placedIDs, id)
		}
	}
	summary := fmt.Sprintf("Import %s: %d placed", state, placed)
	if len(placedLabels) > 0 {
		summary += " (" + strings.Join(placedLabels, ", ") + ")"
	}
	if placed == 0 && state == "failed" {
		why := unpackWhy
		for _, id := range slotIDs {
			if why == "" && slots[id].reason != "" {
				why = slots[id].reason
			}
		}
		if why != "" {
			summary = "Import failed: " + why
		}
	}
	return summary, placedIDs
}
