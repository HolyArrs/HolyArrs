package subtitles

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/importer"
	"github.com/CoderLuii/holyarrs/internal/store"
	"github.com/CoderLuii/holyarrs/internal/subtitles/subtext"
)

// ErrNotBeside refuses work on subtitles that are not a present file
// beside the video.
var ErrNotBeside = errors.New("only a subtitle file beside the video can be changed")

func (s *Service) besideRow(ctx context.Context, id int64) (Subtitle, *fileCtx, error) {
	var row Subtitle
	if err := store.Get(ctx, s.Store.Read(), "subtitles", id, &row); err != nil {
		return row, nil, ErrNotFound
	}
	if row.Place != "beside" || row.State != "present" || row.RelativePath == nil {
		return row, nil, ErrNotBeside
	}
	fc, err := s.load(ctx, row.FileID)
	return row, fc, err
}

// Sync moves the timing of a subtitle beside the video (offset and
// frame-rate fix) and writes it under the same name through the safe
// write; the old text goes to the holding folder.
func (s *Service) Sync(ctx context.Context, id int64, fix subtext.Fix, by domain.Actor) (Subtitle, error) {
	if err := fix.Check(); err != nil {
		return Subtitle{}, err
	}
	row, fc, err := s.besideRow(ctx, id)
	if err != nil {
		return row, err
	}
	rel := *row.RelativePath
	full := filepath.Join(fc.lib, filepath.FromSlash(rel))
	if !importer.InsideReal(fc.lib, full) {
		return row, fmt.Errorf("%s is outside the Library folder", rel)
	}
	fi, err := os.Lstat(full)
	if err != nil {
		return row, err
	}
	if !fi.Mode().IsRegular() || fi.Size() > subtext.MaxText {
		return row, errors.New("the subtitle file is not a regular file under 5 MB")
	}
	b, err := os.ReadFile(full)
	if err != nil {
		return row, err
	}
	format := path.Ext(rel)
	if format != "" {
		format = format[1:]
	}
	text, err := subtext.Clean(b, format)
	if err != nil {
		return row, err
	}
	out, err := subtext.Retime(text, format, fix)
	if err != nil {
		return row, err
	}
	nr := row
	nr.ID = 0
	nr.Sync = map[string]any{"offset_ms": fix.OffsetMS, "from_fps": fix.FromFPS, "to_fps": fix.ToFPS, "from_subtitle_id": row.ID}
	if row.Origin == "found" {
		nr.Origin = "by_hand" // HolyArrs wrote this one
	}
	if _, err := s.writeAt(ctx, fc, &nr, out, rel, &row, by); err != nil {
		return row, err
	}
	titleID, slotID := fc.title.ID, fc.file.SlotIDs[0]
	s.event(ctx, &domain.Event{Actor: by, Kind: EventSynced, TitleID: &titleID, SlotID: &slotID,
		Summary:  fmt.Sprintf("Moved the timing of %s", path.Base(rel)),
		Data:     map[string]any{"file_id": row.FileID, "subtitle_id": nr.ID, "offset_ms": fix.OffsetMS, "from_fps": fix.FromFPS, "to_fps": fix.ToFPS},
		GroupKey: "subtitles:" + itoa(row.FileID)})
	return nr, nil
}

// TurnDown moves a fetched subtitle into the holding folder and never
// takes that site's subtitle for this File again; a new search follows.
func (s *Service) TurnDown(ctx context.Context, id int64, reason string, by domain.Actor) error {
	row, fc, err := s.besideRow(ctx, id)
	if err != nil {
		return err
	}
	if row.SiteName == nil || row.SiteKey == nil {
		return errors.New("only a subtitle HolyArrs fetched from a site can be turned down")
	}
	if len(reason) > 500 {
		reason = reason[:500]
	}
	to, err := importer.HoldPath(fc.lib, *row.RelativePath, now())
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	if err := os.Rename(filepath.Join(fc.lib, filepath.FromSlash(*row.RelativePath)), to); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	err = s.Store.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "UPDATE subtitles SET state = 'replaced', updated_at = ? WHERE id = ?", store.FormatTime(now()), row.ID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO subtitle_turned_down (created_at, created_by, updated_at, updated_by, file_id, site_name, site_key, reason)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT (file_id, site_name, site_key) DO NOTHING`,
			store.FormatTime(now()), actorJSON(by), store.FormatTime(now()), actorJSON(by), row.FileID, *row.SiteName, *row.SiteKey, reason)
		return err
	})
	if err != nil {
		return err
	}
	titleID, slotID := fc.title.ID, fc.file.SlotIDs[0]
	s.event(ctx, &domain.Event{Actor: by, Kind: EventTurnedDown, TitleID: &titleID, SlotID: &slotID,
		Summary: fmt.Sprintf("Turned down %s from %s", path.Base(*row.RelativePath), *row.SiteName),
		Data:    map[string]any{"file_id": row.FileID, "site": *row.SiteName, "reason": reason, "moved_to": libRel(fc.lib, to)}, GroupKey: "subtitles:" + itoa(row.FileID)})
	if s.Enqueue != nil {
		_ = s.Queue(ctx, row.FileID, nil)
	}
	return nil
}

// Choose saves a candidate a person picked from a shown search, at any
// score (the Pass mark is for searches on their own); it replaces a
// subtitle HolyArrs saved for that language.
func (s *Service) Choose(ctx context.Context, fileID int64, siteName, key string, by domain.Actor) (string, error) {
	res, err := s.Search(ctx, fileID, SearchOptions{Show: true}, by)
	if err != nil {
		return "", err
	}
	fc, err := s.load(ctx, fileID)
	if err != nil {
		return "", err
	}
	rows, err := Sites(ctx, s.Store.Read())
	if err != nil {
		return "", err
	}
	set := ReadSettings(ctx, s.Store.Read())
	for _, o := range res.Outcomes {
		for _, c := range o.Choices {
			if c.Site != siteName || c.Key != key {
				continue
			}
			if c.Score.Refused != "" {
				return "", fmt.Errorf("that subtitle cannot be used: %s", c.Score.Refused)
			}
			for _, r := range rows {
				if r.Name != siteName {
					continue
				}
				t := target{want: o.Want, beat: -1}
				old, _ := store.Select[Subtitle](ctx, s.Store.Read(), "subtitles",
					"file_id = ? AND place = 'beside' AND state = 'present' AND origin IN ('fetched', 'by_hand') AND language = ? AND forced = ?",
					fileID, o.Want.Language, o.Want.Forced)
				if len(old) > 0 {
					t.old = &old[0]
				}
				p, err := s.fetchAndSave(ctx, fc, t, c, r, set, by)
				if err != nil {
					return "", err
				}
				_ = s.Store.Write(ctx, func(tx *sql.Tx) error {
					_, err := tx.ExecContext(ctx, "UPDATE subtitles SET origin = 'by_hand' WHERE file_id = ? AND relative_path = ? AND state = 'present'", fileID, p)
					return err
				})
				out := Outcome{Want: o.Want, Result: "saved", Path: p, Score: c.Score.Total, Site: c.Site}
				s.afterTarget(ctx, fc, t, &out, set, by)
				return p, nil
			}
		}
	}
	return "", ErrNotFound
}
