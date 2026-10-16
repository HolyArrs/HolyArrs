package source

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/importer"
	"github.com/CoderLuii/holyarrs/internal/judge"
)

func protectedChoiceErr() error {
	return fmt.Errorf("%w: the copy in this Slot is protected", ErrBusy)
}

// choiceStateTx re-reads the durable copy guard in the same write that will
// choose a Release. A search result or review loaded before protection changed
// cannot replace the protected copy.
func choiceStateTx(tx *sql.Tx, slotID int64) (domain.SlotState, error) {
	var slotState, titleState string
	var protected bool
	err := tx.QueryRow(`SELECT s.state, t.state, COALESCE(f.protected, 0)
		FROM slots s JOIN items i ON i.id = s.item_id JOIN titles t ON t.id = i.title_id
		LEFT JOIN files f ON f.id = s.file_id WHERE s.id = ?`, slotID).Scan(&slotState, &titleState, &protected)
	if err != nil {
		return "", err
	}
	if titleState != "active" {
		return domain.SlotState(slotState), fmt.Errorf("%w: the Title is %s", ErrBusy, strings.ReplaceAll(titleState, "_", " "))
	}
	if protected {
		return domain.SlotState(slotState), protectedChoiceErr()
	}
	return domain.SlotState(slotState), nil
}

func slotHasProtectedCopy(ctx context.Context, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, slotID int64) bool {
	var protected bool
	err := db.QueryRowContext(ctx, `SELECT COALESCE(f.protected, 0) FROM slots s
		LEFT JOIN files f ON f.id = s.file_id WHERE s.id = ?`, slotID).Scan(&protected)
	return err == nil && protected
}

func refuseCopyChoice(v judge.Verdict, rule, detail string) judge.Verdict {
	v.Reasons = append(v.Reasons, judge.Reason{Step: "copy", Rule: rule, Effect: "fail", Detail: detail})
	v.Passed, v.Outcome, v.HoldUntil, v.Rank = false, "refused", nil, 0
	return v
}

// copyChoiceVerdict applies Version-specific copy identity after the shared
// quality judge. Filename languages are hints only; inspected media headers
// remain the import-time authority.
func copyChoiceVerdict(c slotCtx, rel judge.Release, v judge.Verdict) judge.Verdict {
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
			v.Reasons = append(v.Reasons, judge.Reason{Step: "copy", Rule: "edition", Effect: "pass", Detail: "edition matches this Version"})
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
			v.Reasons = append(v.Reasons, judge.Reason{Step: "copy", Rule: "audio_language_hint", Effect: "warn",
				Detail: "the Release name does not claim every requested audio language; inspected media decides at import"})
		}
	}
	return v
}
