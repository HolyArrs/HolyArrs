package sources

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// protectedChoiceErr is the error for choosing a Release into a Slot whose copy is protected. It wraps ErrBusy so callers treat it as a lost race.
func protectedChoiceErr() error {
	return fmt.Errorf("%w: the copy in this Slot is protected", ErrBusy)
}

// choiceStateTx reads a Slot's state inside the write that chooses for it, and refuses when its Title is not active or its copy is protected. Checking in the same transaction keeps a protection set meanwhile from being overwritten.
func choiceStateTx(tx *sql.Tx, slotID int64) (mediafiles.SlotState, error) {
	slotState, titleState, protected, err := mediafiles.SlotChoiceFacts(tx, slotID)
	if err != nil {
		return "", err
	}
	if titleState != "active" {
		return mediafiles.SlotState(slotState), fmt.Errorf("%w: the Title is %s", ErrBusy, strings.ReplaceAll(titleState, "_", " "))
	}
	if protected {
		return mediafiles.SlotState(slotState), protectedChoiceErr()
	}
	return mediafiles.SlotState(slotState), nil
}

// slotHasProtectedCopy reports whether the Slot's current file is protected.
func slotHasProtectedCopy(ctx context.Context, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, slotID int64) bool {
	protected, err := mediafiles.SlotCopyProtected(ctx, db, slotID)
	return err == nil && protected
}

// refuseCopyChoice turns a verdict into a refusal with one copy-step reason and clears everything that would let it win.
func refuseCopyChoice(v mediafiles.Verdict, rule, detail string) mediafiles.Verdict {
	v.Reasons = append(v.Reasons, mediafiles.Reason{Step: "copy", Rule: rule, Effect: "fail", Detail: detail})
	v.Passed, v.Outcome, v.HoldUntil, v.Rank = false, "refused", nil, 0
	return v
}

// copyChoiceVerdict adds the checks that depend on the Version a movie is chosen for. A wrong or unclear edition refuses the Release. A missing audio language only warns, because the media inspected at import decides.
func copyChoiceVerdict(c slotCtx, rel mediafiles.ReleaseFacts, v mediafiles.Verdict) mediafiles.Verdict {
	if c.ti.Kind != "" && c.ti.Kind != "movie" {
		return v
	}
	if wanted := strings.TrimSpace(c.ver.Edition); wanted != "" {
		editions := rel.Parse.Edition
		switch {
		case len(editions) == 0:
			return refuseCopyChoice(v, "edition_unknown", "edition is unknown; choose a Release that states the edition for this Version")
		case len(editions) > 1:
			return refuseCopyChoice(v, "edition_ambiguous", "edition is ambiguous for the selected Version")
		case importer.EditionKey(editions[0]) == importer.EditionKey(wanted):
			v.Reasons = append(v.Reasons, mediafiles.Reason{Step: "copy", Rule: "edition", Effect: "pass", Detail: "edition matches this Version"})
		default:
			return refuseCopyChoice(v, "edition_wrong", "edition does not match the selected Version")
		}
	}
	if len(c.ver.AudioLanguages) > 0 && len(rel.Parse.Languages) > 0 {
		missing := false
		for _, wanted := range c.ver.AudioLanguages {
			if !slices.Contains(rel.Parse.Languages, wanted) {
				missing = true
				break
			}
		}
		if missing {
			v.Reasons = append(v.Reasons, mediafiles.Reason{Step: "copy", Rule: "audio_language_hint", Effect: "warn",
				Detail: "the Release name does not claim every requested audio language; inspected media decides at import"})
		}
	}
	return v
}
