package importer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"slices"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// covers says whether one step holds every episode of an old copy. Only
// then may that copy leave its spot before the step runs: a copy that also
// holds an episode of another step would be gone if that other step then
// fails.
func covers(st mediafiles.PlanStep, f mediafiles.File) bool {
	for _, id := range f.SlotIDs {
		if !slices.Contains(st.TargetSlotIDs, id) {
			return false
		}
	}
	return true
}

// placedSlots lists the episodes that really got a new copy. The plan says
// which steps should fill an episode, but a step can still fail when it
// runs, and an old copy that holds two episodes is then the only copy of
// the one whose step failed.
func placedSlots(plan []mediafiles.PlanStep, res []mediafiles.PlaceResult) map[int64]bool {
	out := map[int64]bool{}
	for k, st := range plan {
		if k < len(res) && res[k].Outcome == "placed" {
			for _, id := range st.TargetSlotIDs {
				out[id] = true
			}
		}
	}
	return out
}

// allIn says whether every id is in set.
func allIn(ids []int64, set map[int64]bool) bool {
	for _, id := range ids {
		if !set[id] {
			return false
		}
	}
	return true
}

// notIn lists the ids that are not in set, in order.
func notIn(ids []int64, set map[int64]bool) []int64 {
	var out []int64
	for _, id := range ids {
		if !set[id] {
			out = append(out, id)
		}
	}
	return out
}

// keepOldCopy brings an old copy back to its spot when a run cut short had
// set it aside, since a step for one of its episodes failed and it stays.
// When it cannot come back (its spot is taken by another file, or the move
// fails) it says so, and the import stops before it records anything, so
// no episode is ever recorded on a file that is not its own.
func keepOldCopy(recorded []string, rel, spot string) error {
	to := heldFrom(recorded, rel)
	if to == "" {
		return nil
	}
	if _, err := os.Lstat(spot); !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("the old copy waits at %s, but its place %s is taken; move one of them and import again", to, spot)
	}
	if err := moveFile(to, spot); err != nil {
		return fmt.Errorf("putting the old copy back from %s: %w", to, err)
	}
	return nil
}

// slotFiles lists the present files the download's slots held when this
// run began, each once. A file stays a candidate to bring back even when
// this run refused or dropped the step that would have replaced it.
func slotFiles(ctx context.Context, db *sql.DB, slots map[int64]*showSlot) []mediafiles.File {
	seen := map[int64]bool{}
	var out []mediafiles.File
	for _, ss := range slots {
		if ss == nil || ss.sl.FileID == nil || seen[*ss.sl.FileID] {
			continue
		}
		seen[*ss.sl.FileID] = true
		var f mediafiles.File
		if err := database.Get(ctx, db, "files", *ss.sl.FileID, &f); err == nil && f.State == "present" {
			out = append(out, f)
		}
	}
	return out
}

// keepOldCopies brings back every old copy that keeps an episode, when an
// earlier run that was cut short had set it aside: whether or not this
// run's plan holds all of its episodes, whether or not its step was
// refused, and whether or not any step placed. Copies set aside in this
// run are already back by now.
func keepOldCopies(fs []mediafiles.File, recorded []string, replaced func(mediafiles.File) bool, abs func(mediafiles.File) string) error {
	var errs []error
	for _, f := range fs {
		if !replaced(f) {
			errs = append(errs, keepOldCopy(recorded, f.RelativePath, abs(f)))
		}
	}
	return errors.Join(errs...)
}
