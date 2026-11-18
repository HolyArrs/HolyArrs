package subtitles

import (
	"context"
	"os"
	"path/filepath"

	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/subtext"
)

// cleanupRun is the cleanup of one save: the text to write and what to say
// about it in the save's Event.
type cleanupRun struct {
	text string         // the cleaned text, or the given text when nothing changed
	note map[string]any // nil when cleanup had nothing to say
}

// cleanup runs the owner's cleanup steps for row's language over text before
// it is saved. Only SRT is cleaned; other formats are saved as they came. A
// forced subtitle keeps its sound lines, capitals and first and last cues,
// since signs are often in capitals and forced files have few cues. A wish
// for hearing-impaired subtitles keeps its sound lines too. When sound lines
// were removed, row is no longer hearing-impaired, so the file is named and
// recorded as a normal one.
func (s *Service) cleanup(ctx context.Context, fc *fileCtx, row *Subtitle, text, format string, set Settings) cleanupRun {
	o := set.CleanupFor(row.Language)
	if row.Forced {
		o.RemoveHI, o.FixUppercase, o.RemoveAds = false, false, false
	}
	if o.RemoveHI && s.wantsHIOnly(ctx, fc, row) {
		o.RemoveHI = false
	}
	if !o.Any() {
		return cleanupRun{text: text}
	}
	if format != subtext.SRT {
		return cleanupRun{text: text, note: map[string]any{"skipped": "only SRT subtitles are cleaned"}}
	}
	out, rep, err := subtext.CleanupSRT(text, o, row.Language)
	if err != nil {
		return cleanupRun{text: text, note: map[string]any{"skipped": err.Error()}}
	}
	if !rep.Any() {
		return cleanupRun{text: text}
	}
	if rep.Steps["remove_hi"] > 0 {
		row.HearingImpaired = false
	}
	return cleanupRun{text: out, note: map[string]any{"removed": rep.Removed, "changed": rep.Changed, "steps": rep.Steps}}
}

// wantsHIOnly reports whether the File's wish asks for a hearing-impaired
// subtitle in row's language. When the wish cannot be read, the sound lines
// are kept, since removing them cannot be undone by a later search.
func (s *Service) wantsHIOnly(ctx context.Context, fc *fileCtx, row *Subtitle) bool {
	wish, err := WishFor(ctx, s.Store.Read(), fc.title.ID, fc.version.ID, fc.item.ID)
	if err != nil {
		return true
	}
	for _, w := range wish.Wants {
		if w.HearingImpaired == HIOnly && w.Forced == row.Forced && (w.Language == row.Language || LangMatches(w.Language, row.Language)) {
			return true
		}
	}
	return false
}

// hold keeps orig, the uncleaned text, in the holding folder under rel before
// the cleaned text is saved there, so a cleanup can be undone later. It
// returns the text to save and the held file's full path. When orig cannot be
// kept, orig is saved instead: no cleanup is worth losing the original.
func (c *cleanupRun) hold(lib, rel, orig string) (string, string) {
	if c.text == orig {
		return orig, ""
	}
	to, err := writeHeld(lib, rel, orig)
	if err != nil {
		// The error carries full paths, which Events must not show.
		c.note = map[string]any{"skipped": "the original could not be kept in the holding folder"}
		return orig, ""
	}
	c.note["held"] = libRel(lib, to)
	return c.text, to
}

// unhold removes a held original after its save failed, so no stray copy is
// left for a subtitle that was never saved.
func unhold(held string) {
	if held != "" {
		_ = os.Remove(held)
	}
}

// writeHeld writes text to a free holding-folder name for rel with the same
// temporary file, sync and rename as a save, and returns its full path.
func writeHeld(lib, rel, text string) (string, error) {
	to, err := importer.HoldPath(lib, rel, now())
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o777); err != nil {
		return "", err
	}
	tmp, err := importer.CreateTemp(filepath.Dir(to), ".holyarrs-sub-", "")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name()) // already gone after the rename
	if _, err := tmp.WriteString(text); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	return to, os.Rename(tmp.Name(), to)
}
