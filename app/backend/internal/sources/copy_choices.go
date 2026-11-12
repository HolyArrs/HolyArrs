package sources

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
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

// copyChoiceVerdict adds the checks that depend on the Version a movie or show is chosen for. A wrong or unclear edition refuses a movie Release. A name that clearly lacks an audio language the Version needs is refused; an unclear one only warns, because the media inspected at import decides.
func copyChoiceVerdict(ti mediafiles.Title, ver mediafiles.Version, rel mediafiles.ReleaseFacts, v mediafiles.Verdict) mediafiles.Verdict {
	movie := ti.Kind == "" || ti.Kind == "movie"
	if !movie && ti.Kind != "show" {
		return v
	}
	if wanted := strings.TrimSpace(ver.Edition); wanted != "" && movie {
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
	original := ""
	if ti.OriginalLanguage != nil {
		original = *ti.OriginalLanguage
	}
	return audioChoiceVerdict(ver.AudioLanguages, original, rel.Parse, v)
}

// audioChoiceVerdict checks the audio languages a Version needs against the languages a Release name claims. A name with no language is read as the original language. MULTI, or a DUAL name that names no language, may hold any language, so a gap there only warns.
func audioChoiceVerdict(need []string, original string, p releasename.Result, v mediafiles.Verdict) mediafiles.Verdict {
	if len(need) == 0 {
		return v
	}
	have := slices.Clone(p.Languages)
	unclear := p.Has("multi") || p.Has("dual") && len(have) == 0
	if (len(have) == 0 || p.Has("dual")) && original != "" {
		have = append(have, original)
	}
	// A language names never show (the reader does not know it) cannot be clearly missing.
	missing, unread := "", ""
	for _, l := range need {
		if slices.Contains(have, l) {
			continue
		}
		if releasename.ReadsLanguage(l) {
			missing = l
			break
		}
		if unread == "" {
			unread = l
		}
	}
	switch {
	case missing == "" && unread == "":
		return v
	case missing == "":
		v.Reasons = append(v.Reasons, mediafiles.Reason{Step: "copy", Rule: "audio_language_hint", Effect: "warn",
			Detail: "Release names do not show " + mediafiles.LanguageName(unread) + " audio; inspected media decides at import"})
		return v
	case len(have) == 0:
		v.Reasons = append(v.Reasons, mediafiles.Reason{Step: "copy", Rule: "audio_language_hint", Effect: "warn",
			Detail: "no language in the name and the original language is unknown; inspected media decides at import"})
		return v
	case unclear:
		v.Reasons = append(v.Reasons, mediafiles.Reason{Step: "copy", Rule: "audio_language_hint", Effect: "warn",
			Detail: "MULTI or DUAL may hold " + mediafiles.LanguageName(missing) + " audio; inspected media decides at import"})
		return v
	}
	return refuseCopyChoice(v, "audio_language_missing", "the name has no "+mediafiles.LanguageName(missing)+" audio, which this Version needs")
}

// withVersionAudio returns the rules body with the Version's audio languages added to the wanted languages, so a French Version of an English title passes French names. The list is copied: the shared body is never written.
func withVersionAudio(body mediafiles.PickRulesBody, audio []string) mediafiles.PickRulesBody {
	if len(audio) == 0 {
		return body
	}
	wanted := slices.Clip(slices.Clone(body.Language.Wanted))
	for _, l := range audio {
		if !slices.Contains(wanted, l) {
			wanted = append(wanted, l)
		}
	}
	body.Language.Wanted = wanted
	return body
}
