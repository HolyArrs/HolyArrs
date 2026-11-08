package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
)

// HeldFile is one deleted file still waiting in a holding folder.
type HeldFile struct {
	ID           int64      `json:"id"`        // the File's id
	Path         string     `json:"path"`      // where it was, in its Library folder
	HeldPath     string     `json:"held_path"` // where it waits now
	Folder       string     `json:"folder"`    // the Library folder's label
	TitleID      int64      `json:"title_id"`
	Title        string     `json:"title"`
	Kind         string     `json:"kind"`
	TitleDeleted bool       `json:"title_deleted"`
	Size         int64      `json:"size"`
	DeletedAt    time.Time  `json:"deleted_at"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"` // nil: kept until emptied by hand
}

// RestoreResult says what a restore did with one File.
type RestoreResult struct {
	ID     int64  `json:"id"`
	Path   string `json:"path,omitempty"`
	Result string `json:"result"` // restored, or why not
}

type heldRow struct {
	HeldFile
	root, state string
	slots       []int64
}

// heldFiles reads the deleted Files still on disk in a holding folder
// (id 0: all of them).
func (s *Service) heldFiles(ctx context.Context, id int64) ([]heldRow, error) {
	db := s.Store.Read()
	rules, err := importer.ReadFileRules(ctx, db)
	if err != nil {
		return nil, err
	}
	prefix := importer.HoldingDir + "/"
	rows, err := db.QueryContext(ctx, `SELECT f.id, f.relative_path, f.size, f.updated_at, f.slot_ids, lf.path, lf.label, t.id, t.name, t.kind, t.state
		FROM files f JOIN versions v ON v.id = f.version_id JOIN library_folders lf ON lf.id = v.library_folder_id JOIN titles t ON t.id = v.title_id
		WHERE f.state = 'gone' AND substr(f.relative_path, 1, ?) = ? AND (? = 0 OR f.id = ?) ORDER BY f.updated_at DESC, f.id DESC`,
		len(prefix), prefix, id, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []heldRow
	for rows.Next() {
		var h heldRow
		var at, slots string
		if err := rows.Scan(&h.ID, &h.HeldPath, &h.Size, &at, &slots, &h.root, &h.Folder, &h.TitleID, &h.Title, &h.Kind, &h.state); err != nil {
			return nil, err
		}
		day, rest, ok := strings.Cut(strings.TrimPrefix(h.HeldPath, prefix), "/")
		d, err := time.Parse("2006-01-02", day)
		if !ok || err != nil {
			continue
		}
		if fi, err := os.Lstat(filepath.Join(h.root, filepath.FromSlash(h.HeldPath))); err != nil || !fi.Mode().IsRegular() {
			continue // emptied already, or not a plain file
		}
		h.Path = rest
		h.TitleDeleted = h.state == "removed"
		h.DeletedAt, _ = time.Parse(database.TimeLayout, at)
		if h.DeletedAt.IsZero() {
			h.DeletedAt = d
		}
		if rules.HoldingDays > 0 {
			// CleanHolding empties a day once it is older than the setting.
			e := d.AddDate(0, 0, rules.HoldingDays+1)
			h.ExpiresAt = &e
		}
		_ = json.Unmarshal([]byte(slots), &h.slots)
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// The exact place it came from is in its file_held event; a name the
	// holding folder had to change (name.1) comes back as it was.
	for i := range out {
		var from sql.NullString
		_ = s.Store.ReadEvents().QueryRowContext(ctx, `SELECT json_extract(data, '$.from') FROM events
			WHERE kind = ? AND json_extract(data, '$.file_id') = ? AND json_extract(data, '$.to') = ? ORDER BY id DESC LIMIT 1`,
			domain.EventFileHeld, out[i].ID, out[i].HeldPath).Scan(&from)
		if from.Valid && from.String != "" {
			out[i].Path = from.String
		}
	}
	return out, nil
}

// HeldFiles lists the deleted Files that can still be restored.
func (s *Service) HeldFiles(ctx context.Context) ([]HeldFile, error) {
	rows, err := s.heldFiles(ctx, 0)
	out := make([]HeldFile, 0, len(rows))
	for _, h := range rows {
		out = append(out, h.HeldFile)
	}
	return out, err
}

// RestoreHeld moves held Files back where they were. A place that is taken
// is never overwritten. A deleted movie, show or artist comes back to the
// Library with the File; it returns the titles that came back.
func (s *Service) RestoreHeld(ctx context.Context, ids []int64, by domain.Actor) ([]RestoreResult, []int64, error) {
	unlock := s.Store.LockFiles()
	defer unlock()
	var out []RestoreResult
	var back []int64
	for _, id := range ids {
		rows, err := s.heldFiles(ctx, id)
		if err != nil {
			return out, back, err
		}
		if len(rows) == 0 {
			out = append(out, RestoreResult{ID: id, Result: "not in a holding folder"})
			continue
		}
		h := rows[0]
		why, err := s.restoreOne(ctx, h, by)
		if err != nil {
			return out, back, err
		}
		if why == "" {
			why = "restored"
			if h.TitleDeleted && !containsID(back, h.TitleID) {
				back = append(back, h.TitleID)
			}
		}
		out = append(out, RestoreResult{ID: id, Path: h.Path, Result: why})
	}
	return out, back, nil
}

func containsID(ids []int64, id int64) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

// restoreOne puts one held File back; a non-empty answer says why not.
func (s *Service) restoreOne(ctx context.Context, h heldRow, by domain.Actor) (string, error) {
	if h.TitleDeleted {
		var other int64
		_ = s.Store.Read().QueryRowContext(ctx, `SELECT o.id FROM titles o JOIN titles t ON t.id = ? WHERE o.id != t.id AND o.kind = t.kind
			AND o.state != 'removed' AND EXISTS (SELECT 1 FROM json_each(t.source_ids) a JOIN json_each(o.source_ids) b ON a.key = b.key AND a.value = b.value)`,
			h.TitleID).Scan(&other)
		if other != 0 {
			return "it was added to the Library again; restore it by hand", nil
		}
	}
	from := filepath.Join(h.root, filepath.FromSlash(h.HeldPath))
	to := filepath.Join(h.root, filepath.FromSlash(h.Path))
	if !filepath.IsLocal(filepath.FromSlash(h.Path)) || strings.HasPrefix(h.Path, importer.HoldingDir+"/") ||
		!importer.InsideReal(h.root, filepath.Dir(from)) || !importer.InsideReal(h.root, filepath.Dir(to)) {
		return "its place is outside the Library folder", nil
	}
	if _, err := os.Lstat(to); err == nil {
		return "a file is already at " + h.Path, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err.Error(), nil
	}
	if err := moveFile(from, to); err != nil {
		return err.Error(), nil
	}
	if !importer.InsideReal(h.root, filepath.Dir(to)) {
		// A link in the way led it out: put it back where it waited.
		_ = moveFile(to, from)
		return "its place is outside the Library folder", nil
	}
	// The subtitles held beside it come back too when their place is free.
	var subs []string
	var raw sql.NullString
	_ = s.Store.ReadEvents().QueryRowContext(ctx, `SELECT json_extract(data, '$.subtitles') FROM events
		WHERE kind = ? AND json_extract(data, '$.file_id') = ? AND json_extract(data, '$.to') = ? ORDER BY id DESC LIMIT 1`,
		domain.EventFileHeld, h.ID, h.HeldPath).Scan(&raw)
	var subsBack []string
	if raw.Valid {
		_ = json.Unmarshal([]byte(raw.String), &subs)
	}
	for _, sub := range subs {
		_, orig, ok := strings.Cut(strings.TrimPrefix(sub, importer.HoldingDir+"/"), "/")
		if !ok || !filepath.IsLocal(filepath.FromSlash(orig)) {
			continue
		}
		sf, st := filepath.Join(h.root, filepath.FromSlash(sub)), filepath.Join(h.root, filepath.FromSlash(orig))
		if _, err := os.Lstat(st); err == nil || !importer.InsideReal(h.root, filepath.Dir(sf)) || !importer.InsideReal(h.root, filepath.Dir(st)) {
			continue
		}
		if moveFile(sf, st) == nil {
			if !importer.InsideReal(h.root, filepath.Dir(st)) {
				_ = moveFile(st, sf)
				continue
			}
			subsBack = append(subsBack, orig)
		}
	}
	ts, who := database.FormatTime(s.now()), ActorText(by)
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE files SET state = 'present', relative_path = ?, updated_at = ?, updated_by = ? WHERE id = ?", h.Path, ts, who, h.ID); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE subtitles SET state = 'present', updated_at = ? WHERE file_id = ? AND state = 'gone' AND place <> 'beside'", ts, h.ID); err != nil {
			return err
		}
		for _, sub := range subsBack {
			if _, err := tx.Exec("UPDATE subtitles SET state = 'present', updated_at = ? WHERE file_id = ? AND state = 'gone' AND place = 'beside' AND relative_path = ?", ts, h.ID, sub); err != nil {
				return err
			}
		}
		if h.TitleDeleted {
			if _, err := tx.Exec("UPDATE titles SET state = 'active', updated_at = ?, updated_by = ? WHERE id = ? AND state = 'removed'", ts, who, h.TitleID); err != nil {
				return err
			}
			// Its other Slots have no file yet: wanted, as after a part delete.
			if _, err := tx.Exec(`UPDATE slots SET state = 'wanted', file_id = NULL, last_reason = 'its file was deleted', updated_at = ?, updated_by = ?
				WHERE state = 'removed' AND item_id IN (SELECT id FROM items WHERE title_id = ?)`, ts, who, h.TitleID); err != nil {
				return err
			}
		}
		for _, sid := range h.slots {
			if _, err := tx.Exec(`UPDATE slots SET state = 'in_library', file_id = ?, last_reason = 'its file was restored', updated_at = ?, updated_by = ?
				WHERE id = ? AND state IN ('wanted', 'removed')`, h.ID, ts, who, sid); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		_ = moveFile(to, from)
		return "", err
	}
	titleID := h.TitleID
	s.Emit(ctx, &domain.Event{Actor: by, Kind: domain.EventFileBack, TitleID: &titleID, Summary: "Restored " + filepath.Base(h.Path) + " from the holding folder",
		Data: map[string]any{"file_id": h.ID, "from": h.HeldPath, "to": h.Path, "subtitles": subsBack, "title_back": h.TitleDeleted}, GroupKey: GroupKey(titleID)})
	return "", nil
}
