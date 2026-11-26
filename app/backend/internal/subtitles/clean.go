package subtitles

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/subtext"
)

// EventCleaned is a subtitle cleaned of ads or tidied.
const EventCleaned = "subtitle_cleaned"

// KindClean cleans every subtitle beside the videos in the Library again.
const KindClean = "subtitle_clean"

// stepsFor returns the cleanup steps for one subtitle. A hearing-impaired
// subtitle was wished for with its sound notes, and a forced one carries only
// signs and foreign lines, so neither loses notes or names.
func stepsFor(set Settings, row *Subtitle) subtext.Tidy {
	t := set.CleanFor(row.Language)
	if row.HearingImpaired || row.Forced {
		t.HI = false
	}
	if row.Forced {
		t.Caps = false
	}
	return t
}

// cleanForSave runs the cleanup on text about to be saved. It returns the
// text to write and, when something changed, the report.
func cleanForSave(set Settings, row *Subtitle, text, format string) (string, *subtext.TidyReport) {
	t := stepsFor(set, row)
	out, rep, err := subtext.TidyText(text, format, t)
	if err != nil || !rep.Did() {
		return text, nil
	}
	return out, &rep
}

// holdOriginal keeps the text from before the cleanup in the dated holding
// folder, which is emptied after the holding days like every replaced file.
// It returns the held path relative to the Library folder.
func holdOriginal(lib, rel, text string) string {
	to, err := importer.HoldPath(lib, rel, now())
	if err != nil {
		return ""
	}
	if os.MkdirAll(filepath.Dir(to), 0o777) != nil || os.WriteFile(to, []byte(text), 0o666) != nil {
		return ""
	}
	r, err := filepath.Rel(lib, to)
	if err != nil {
		return ""
	}
	return filepath.ToSlash(r)
}

// cleaned adds the History line for a cleanup.
func (s *Service) cleaned(ctx context.Context, fc *fileCtx, row *Subtitle, rel, held string, steps subtext.Tidy, rep subtext.TidyReport, by domain.Actor) {
	titleID, slotID := fc.title.ID, fc.file.SlotIDs[0]
	data := map[string]any{"file_id": fc.file.ID, "subtitle_id": row.ID, "language": row.Language, "path": rel, "removed": rep.Removed,
		"ads": rep.Ads, "changed": rep.Changed, "steps": steps.Steps()}
	if held != "" {
		data["original"] = held
	}
	if len(rep.Skipped) > 0 {
		data["skipped"] = rep.Skipped
	}
	s.event(ctx, &domain.Event{Actor: by, Kind: EventCleaned, TitleID: &titleID, SlotID: &slotID,
		Summary: fmt.Sprintf("Cleaned %s: %d ad lines removed, %d lines tidied", path.Base(rel), rep.Ads, rep.Changed),
		Data:    data, GroupKey: "subtitles:" + itoa(fc.file.ID)})
}

// saveClean is save with the cleanup: the cleaned text is written, the text
// as fetched waits in the holding folder, and History notes what changed.
func (s *Service) saveClean(ctx context.Context, fc *fileCtx, row *Subtitle, text, format, rel string, set Settings, old *Subtitle, by domain.Actor) (string, error) {
	out, rep := cleanForSave(set, row, text, format)
	p, err := s.writeAt(ctx, fc, row, out, rel, old, by)
	if err == nil && rep != nil {
		s.cleaned(ctx, fc, row, p, holdOriginal(fc.lib, p, text), stepsFor(set, row), *rep, by)
	}
	return p, err
}

// CleanAgain runs the cleanup on a subtitle beside the video now. The text
// before the cleanup moves to the holding folder, as with a timing fix, so it
// can be put back. It reports false when there was nothing to clean.
func (s *Service) CleanAgain(ctx context.Context, id int64, by domain.Actor) (bool, error) {
	row, fc, err := s.besideRow(ctx, id)
	if err != nil {
		return false, err
	}
	if row.Origin == "found" {
		return false, ErrNotOurs
	}
	rel := *row.RelativePath
	if path.Ext(rel) != "."+subtext.SRT {
		return false, nil
	}
	full := filepath.Join(fc.lib, filepath.FromSlash(rel))
	if !importer.InsideReal(fc.lib, full) {
		return false, fmt.Errorf("%s is outside the Library folder", rel)
	}
	fi, err := os.Lstat(full)
	if err != nil {
		return false, err
	}
	if !fi.Mode().IsRegular() || fi.Size() > subtext.MaxText {
		return false, errors.New("the subtitle file is not a regular file under 5 MB")
	}
	b, err := os.ReadFile(full)
	if err != nil {
		return false, err
	}
	text, err := subtext.Clean(b, subtext.SRT)
	if err != nil {
		return false, err
	}
	set := ReadSettings(ctx, s.Store.Read())
	out, rep := cleanForSave(set, &row, text, subtext.SRT)
	if rep == nil {
		return false, nil
	}
	nr := row
	nr.ID = 0
	if _, err := s.writeAt(ctx, fc, &nr, out, rel, &row, by); err != nil {
		return false, err
	}
	s.cleaned(ctx, fc, &nr, rel, "", stepsFor(set, &row), *rep, by)
	return true, nil
}

// cleanRows cleans each subtitle in ids and counts the ones that changed.
// One that fails is skipped so the rest still run.
func (s *Service) cleanRows(ctx context.Context, ids []int64, by domain.Actor) int {
	n := 0
	for _, id := range ids {
		if ctx.Err() != nil {
			break
		}
		if ok, _ := s.CleanAgain(ctx, id, by); ok {
			n++
		}
	}
	return n
}

// CleanTitle cleans every subtitle the app saved beside the videos of one
// title, in every Version, and returns how many changed.
func (s *Service) CleanTitle(ctx context.Context, titleID int64, by domain.Actor) (int, error) {
	files, err := FilesOf(ctx, s.Store.Read(), titleID, 0)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, f := range files {
		ids, err := s.cleanIDs(ctx, f)
		if err != nil {
			return n, err
		}
		n += s.cleanRows(ctx, ids, by)
	}
	return n, nil
}

// QueueCleanAll queues a cleanup of every subtitle the app saved in the Library.
func (s *Service) QueueCleanAll(ctx context.Context) error {
	if s.Enqueue == nil {
		return errors.New("the job queue is not running")
	}
	return s.Enqueue(ctx, KindClean, nil, nil, KindClean)
}

func (s *Service) handleClean(ctx context.Context, r *jobs.Run) error {
	ids, err := s.cleanIDs(ctx, 0)
	if err != nil {
		return err
	}
	n := s.cleanRows(ctx, ids, jobActor(r))
	s.event(ctx, &domain.Event{Actor: jobActor(r), Kind: EventCleaned,
		Summary: fmt.Sprintf("Cleaned subtitles in the Library: %d of %d changed", n, len(ids)),
		Data:    map[string]any{"changed": n, "checked": len(ids)}})
	return ctx.Err()
}
