package library

import (
	"context"
	"database/sql"
	"errors"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
)

// errDryRun rolls back a dry-run edit after its changes were worked out.
var errDryRun = errors.New("dry run")

// TitleChange is an edit of a movie or a show. Nil fields stay as they
// are. The Release gate is for movies; the episode order and new seasons
// are for shows. A show's details source is changed with SwitchSource.
type TitleChange struct {
	KeepUpToDate    *bool   `json:"keep_up_to_date"`
	PickRulesID     *int64  `json:"pick_rules_id"`
	ReleaseGate     *string `json:"release_gate"`
	GateDelayDays   *int    `json:"gate_delay_days"`
	LibraryFolderID *int64  `json:"library_folder_id"`
	EpisodeOrder    *string `json:"episode_order"`
	NewSeasons      *string `json:"new_seasons"`
	// Tags route the title's grabs and searches (tagset); nil keeps them.
	Tags *[]string `json:"tags"`
}

// SlotChange is one Slot whose state or Good enough flag an edit changes.
type SlotChange struct {
	SlotID         int64  `json:"slot_id"`
	From           string `json:"from"`
	To             string `json:"to"`
	GoodEnoughFrom bool   `json:"good_enough_from"`
	GoodEnoughTo   bool   `json:"good_enough_to"`
}

// EditResult says what an edit changed, or would change in a dry run.
type EditResult struct {
	DryRun           bool         `json:"dry_run"`
	SlotChanges      []SlotChange `json:"slot_changes"`
	GoodEnoughChange int          `json:"good_enough_changes"`
	GateDriver       string       `json:"gate_driver"`
	// FolderMoved is true when the Version had no files and simply points at
	// the new Library folder; MoveJobID names the dry-run move Job when it had.
	FolderMoved bool   `json:"folder_moved,omitempty"`
	MoveJobID   *int64 `json:"move_job_id,omitempty"`
}

// Edit changes Keep up to date, Pick rules, the Release gate and the Library
// folder of a movie. Slots follow at once: the Good enough flag is worked out
// again from each File's release name and the new rules, in_library and
// better_wanted swap when needed, and the gate moves not_out_yet and wanted.
// With dryRun nothing is saved. A Library folder change for a Version with
// files only queues a dry-run move Job; the files move after approval.
func (s *Service) Edit(ctx context.Context, titleID int64, c TitleChange, dryRun bool, by domain.Actor) (EditResult, error) {
	res := EditResult{DryRun: dryRun, SlotChanges: []SlotChange{}}
	if c.ReleaseGate != nil && !ValidGate(*c.ReleaseGate) {
		return res, ErrBadGate
	}
	if c.GateDelayDays != nil && (*c.GateDelayDays < 0 || *c.GateDelayDays > 365) {
		return res, ErrBadDelay
	}
	var cur mediafiles.Title
	if err := database.Get(ctx, s.Store.Read(), "titles", titleID, &cur); err != nil {
		return res, err
	}
	show := cur.Kind == "show"
	switch {
	case show && (c.ReleaseGate != nil || c.GateDelayDays != nil):
		return res, ErrMovieOnly
	case !show && (c.EpisodeOrder != nil || c.NewSeasons != nil):
		return res, ErrShowOnly
	case c.EpisodeOrder != nil && !ValidOrder(*c.EpisodeOrder):
		return res, ErrBadOrder
	case c.NewSeasons != nil && *c.NewSeasons != "all" && *c.NewSeasons != "none":
		return res, ErrBadNewSeasons
	}
	var newFolder *mediafiles.LibraryFolder
	if c.LibraryFolderID != nil {
		var lf mediafiles.LibraryFolder
		if err := database.Get(ctx, s.Store.Read(), "library_folders", *c.LibraryFolderID, &lf); err != nil {
			return res, ErrNoFolder
		}
		if want, wrong := folderKindOf(cur.Kind); lf.Kind != want {
			return res, wrong
		}
		if h := CheckFolder(lf.Path, s.DownloadFolders(ctx)); !h.Usable() {
			return res, &FolderError{Health: h}
		}
		newFolder = &lf
	}
	now := s.now()
	var t mediafiles.Title
	var opened, closed []int64
	needMove := false
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		if err := database.Get(ctx, tx, "titles", titleID, &t); err != nil {
			return err
		}
		if c.KeepUpToDate != nil {
			t.KeepUpToDate = *c.KeepUpToDate
			qs := []string{"UPDATE items SET keep_up_to_date = ? WHERE title_id = ?", "UPDATE versions SET keep_up_to_date = ? WHERE title_id = ?"}
			if show {
				// A show's seasons and episodes keep their own switches.
				qs = qs[1:]
			}
			for _, q := range qs {
				if _, err := tx.Exec(q, t.KeepUpToDate, titleID); err != nil {
					return err
				}
			}
		}
		if c.ReleaseGate != nil {
			t.ReleaseGate = *c.ReleaseGate
		}
		if c.GateDelayDays != nil {
			t.GateDelayDays = *c.GateDelayDays
		}
		if c.EpisodeOrder != nil {
			t.EpisodeOrder = c.EpisodeOrder
		}
		if c.NewSeasons != nil {
			t.NewPartsPolicy = *c.NewSeasons
		}
		if c.Tags != nil {
			t.Tags = mediafiles.CleanTags(*c.Tags)
		} else if t.Tags == nil {
			t.Tags = []string{}
		}
		if !show {
			t.GateDriver, t.GateOpensAt = Gate(t.ReleaseGate, t.GateDelayDays, DatesOf(t))
		}
		if err := database.Update(tx, "titles", &t, by); err != nil {
			return err
		}
		vs, err := database.Select[mediafiles.Version](ctx, tx, "versions", "title_id = ?", titleID)
		if err != nil {
			return err
		}
		for _, v := range vs {
			// Pick rules and Library folder here are the main Version's;
			// other Versions are changed one by one.
			if c.PickRulesID != nil && v.IsMain && *c.PickRulesID != v.PickRulesID {
				if err := PickRulesFor(tx, c.PickRulesID, t.Kind); err != nil {
					return err
				}
				v.PickRulesID = *c.PickRulesID
				if err := database.Update(tx, "versions", &v, by); err != nil {
					return err
				}
			}
			if newFolder != nil && v.IsMain && newFolder.ID != v.LibraryFolderID {
				var n int
				if err := tx.QueryRow("SELECT count(*) FROM files WHERE version_id = ? AND state = 'present'", v.ID).Scan(&n); err != nil {
					return err
				}
				if n == 0 {
					v.LibraryFolderID = newFolder.ID
					if err := database.Update(tx, "versions", &v, by); err != nil {
						return err
					}
					res.FolderMoved = true
				} else {
					needMove = true
				}
			}
			ch, err := goodEnoughTx(ctx, tx, v, t.KeepUpToDate, by)
			if err != nil {
				return err
			}
			res.SlotChanges = append(res.SlotChanges, ch...)
		}
		if !show {
			opened, closed, err = applyGateTx(ctx, tx, t, now, by)
			if err != nil {
				return err
			}
		}
		for _, id := range opened {
			res.SlotChanges = append(res.SlotChanges, SlotChange{SlotID: id, From: string(mediafiles.SlotNotOutYet), To: string(mediafiles.SlotWanted)})
		}
		for _, id := range closed {
			res.SlotChanges = append(res.SlotChanges, SlotChange{SlotID: id, From: string(mediafiles.SlotWanted), To: string(mediafiles.SlotNotOutYet)})
		}
		if dryRun {
			return errDryRun
		}
		return nil
	})
	if err != nil && !errors.Is(err, errDryRun) {
		return res, err
	}
	res.GateDriver = t.GateDriver
	for _, ch := range res.SlotChanges {
		if ch.GoodEnoughFrom != ch.GoodEnoughTo {
			res.GoodEnoughChange++
		}
	}
	if dryRun {
		return res, nil
	}
	s.Emit(ctx, &domain.Event{Actor: by, Kind: domain.EventTitleEdited, TitleID: &titleID, Summary: "Changed " + t.Name,
		Data: map[string]any{"change": c, "slot_changes": res.SlotChanges}, GroupKey: GroupKey(titleID)})
	s.gateEvents(ctx, t, opened, closed, by, nil)
	if needMove {
		id, _, err := s.Queue.Enqueue(ctx, KindMoveTitle, &domain.Subject{Type: "title", ID: titleID},
			map[string]any{"library_folder_id": newFolder.ID, "apply": false}, moveKey(titleID, false), by)
		if err != nil {
			return res, err
		}
		res.MoveJobID = &id
	}
	return res, nil
}

// goodEnoughTx works out the Good enough flag of each of the Version's Slots
// with a File, and swaps in_library and better_wanted to match.
func goodEnoughTx(ctx context.Context, tx *sql.Tx, v mediafiles.Version, keep bool, by domain.Actor) ([]SlotChange, error) {
	var pr mediafiles.PickRules
	if err := database.Get(ctx, tx, "pick_rules", v.PickRulesID, &pr); err != nil {
		return nil, err
	}
	slots, err := database.Select[mediafiles.Slot](ctx, tx, "slots", "version_id = ? AND file_id IS NOT NULL", v.ID)
	if err != nil {
		return nil, err
	}
	var out []SlotChange
	for _, sl := range slots {
		var f mediafiles.File
		if err := database.Get(ctx, tx, "files", *sl.FileID, &f); err != nil {
			continue
		}
		name := f.RelativePath
		if f.ReleaseName != nil && *f.ReleaseName != "" {
			name = *f.ReleaseName
		}
		ge := mediafiles.OwnedReachesGoodEnough(name, f.MediaInfo, pr.Body) && fileMeetsVersionChoices(v, f)
		to := sl.State
		switch {
		case sl.State == mediafiles.SlotInLibrary && !ge && keep && v.KeepUpToDate:
			to = mediafiles.SlotBetterWanted
		case sl.State == mediafiles.SlotBetterWanted && (ge || !keep):
			to = mediafiles.SlotInLibrary
		}
		if to == sl.State && ge == sl.GoodEnough {
			continue
		}
		if to != sl.State {
			if err := mediafiles.MoveSlotTx(tx, sl.ID, to, "Pick rules, copy requirements or Keep up to date changed", by); err != nil {
				return nil, err
			}
		}
		if _, err := tx.Exec("UPDATE slots SET good_enough = ? WHERE id = ?", ge, sl.ID); err != nil {
			return nil, err
		}
		out = append(out, SlotChange{SlotID: sl.ID, From: string(sl.State), To: string(to), GoodEnoughFrom: sl.GoodEnough, GoodEnoughTo: ge})
	}
	return out, nil
}

func fileMeetsVersionChoices(v mediafiles.Version, f mediafiles.File) bool {
	if v.Edition != "" {
		edition, _ := f.MediaInfo["edition"].(string)
		if edition == "" || importer.EditionKey(edition) != importer.EditionKey(v.Edition) {
			return false
		}
	}
	return importer.AudioMeets(f.MediaInfo, v.AudioLanguages)
}
