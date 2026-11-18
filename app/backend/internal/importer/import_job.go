package importer

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"slices"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
)

// Why a Slot that was left importing waits for a person again.
const (
	wentWrongWhy    = "something went wrong; see the log"
	gaveUpWhy       = "the file could not be placed: " + wentWrongWhy
	downloadGoneWhy = "the download HolyArrs was importing is gone; search again or import the files by hand"
)

// HandleJob runs an import job.
func (im *Importer) HandleJob(ctx context.Context, r *jobs.Run) error {
	if r.Job.Subject == nil || r.Job.Subject.Type != "download" {
		return errors.New("import needs a download subject")
	}
	dry, _ := r.Job.Params["dry_run"].(bool)
	imp, err := im.Import(ctx, r.Job.Subject.ID, dry, domain.Actor{Kind: "job", ID: r.Job.ID})
	if errors.Is(err, ErrEpisodeImport) || errors.Is(err, ErrAlbumImport) || errors.Is(err, ErrPackWaiting) {
		r.SetResult(map[string]any{"waiting": err.Error()})
		return nil
	}
	if err != nil {
		// The last attempt: nothing imports this again on its own, so the Slots wait for a person with a reason.
		// An import the stopping app cut short did not fail: its Slots stay for the next start.
		if !dry && ctx.Err() == nil && !errors.Is(err, context.Canceled) && r.Job.MaxAttempts > 0 && r.Job.Attempt >= r.Job.MaxAttempts {
			log.Printf("importer: the import of download %d failed on its last attempt: %v", r.Job.Subject.ID, err)
			im.backToCard(ctx, r.Job.Subject.ID, gaveUpWhy)
		}
		return err
	}
	r.SetResult(map[string]any{"import_id": imp.ID, "state": imp.State})
	return nil
}

// backToCard moves the Slots of a download that were left importing back to
// waiting for a person, with reason.
func (im *Importer) backToCard(ctx context.Context, downloadID int64, reason string) {
	var d domain.Download
	if err := database.Get(ctx, im.Store.Read(), "downloads", downloadID, &d); err != nil {
		return
	}
	im.cardSlots(ctx, d.SlotIDs, &downloadID, reason)
}

// cardSlots moves the given Slots that are importing (for download, when
// given) to waiting for a person, with reason.
func (im *Importer) cardSlots(ctx context.Context, ids []int64, download *int64, reason string) {
	err := im.Store.Write(ctx, func(tx *sql.Tx) error {
		for _, id := range ids {
			var sl mediafiles.Slot
			if err := mediafiles.LoadSlot(ctx, tx, id, &sl); err != nil || sl.State != mediafiles.SlotImporting {
				continue
			}
			if download != nil && sl.ActiveDownloadID != nil && *sl.ActiveDownloadID != *download {
				continue
			}
			if err := mediafiles.MoveSlotTx(tx, id, mediafiles.SlotNeedsADecision, reason, domain.System); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		log.Printf("importer: returning Slots %v to a card: %v", ids, err)
	}
}

// wakeGaveUp puts the Slots of a download that wait on the card of an import
// that gave up back to importing, keeping that reason, so the import the
// sweeper queues next takes them. It returns how many it woke.
func (im *Importer) wakeGaveUp(ctx context.Context, downloadID int64) (n int) {
	var d domain.Download
	if err := database.Get(ctx, im.Store.Read(), "downloads", downloadID, &d); err != nil {
		return 0
	}
	err := im.Store.Write(ctx, func(tx *sql.Tx) error {
		n = 0
		for _, id := range d.SlotIDs {
			var sl mediafiles.Slot
			if err := mediafiles.LoadSlot(ctx, tx, id, &sl); err != nil || sl.State != mediafiles.SlotNeedsADecision || sl.LastReason == nil || *sl.LastReason != gaveUpWhy {
				continue
			}
			if sl.ActiveDownloadID != nil && *sl.ActiveDownloadID != d.ID {
				continue
			}
			if err := mediafiles.MoveSlotTx(tx, id, mediafiles.SlotImporting, gaveUpWhy, domain.System); err != nil {
				return err
			}
			n++
		}
		return nil
	})
	if err != nil {
		log.Printf("importer: waking the Slots of download %d: %v", downloadID, err)
		return 0
	}
	return n
}

// leftImporting adds to ids the finished downloads with a Slot left importing
// and no import job, movies included, and the show and album downloads whose
// import gave up half-way, which are tried again as they always were. It
// returns to a card the Slots left importing whose download is gone. gaveUp
// holds the ids only the gave-up list gave.
func (im *Importer) leftImporting(ctx context.Context, ids []int64) (_ []int64, gaveUp map[int64]bool, err error) {
	db := im.Store.Read()
	gaveUp = map[int64]bool{}
	for i, list := range []func(context.Context, *sql.DB) ([]int64, error){importingDownloads, gaveUpDownloads} {
		more, err := list(ctx, db)
		if err != nil {
			return ids, gaveUp, err
		}
		for _, id := range more {
			if !slices.Contains(ids, id) {
				ids = append(ids, id)
				gaveUp[id] = i == 1
			}
		}
	}
	lost, err := importingWithoutDownload(ctx, db)
	if err != nil {
		return ids, gaveUp, err
	}
	if len(lost) > 0 {
		im.cardSlots(ctx, lost, nil, downloadGoneWhy)
	}
	return ids, gaveUp, nil
}
