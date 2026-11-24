package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
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
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`  // nil: kept until emptied by hand
	Replaced     bool       `json:"replaced,omitempty"`    // an upgrade put a better copy in its place
	ReplacedBy   string     `json:"replaced_by,omitempty"` // the better copy's path, when known
}

// RestoreResult says what a restore did with one File.
type RestoreResult struct {
	ID     int64  `json:"id"`
	Path   string `json:"path,omitempty"`
	Result string `json:"result"` // restored, or why not
	// Taken: another file sits where it was; restore again with how
	// "both" (a free name beside it) or, when CanSwap, "swap" (the copy
	// there goes to Recently deleted in its place).
	Taken   bool `json:"taken,omitempty"`
	CanSwap bool `json:"can_swap,omitempty"`
}

// Restore choices when the place is taken.
const (
	RestoreSwap = "swap"
	RestoreBoth = "both"
)

type heldRow struct {
	HeldFile
	root, state string
	slots       []int64
	newID       int64 // the copy an upgrade put in its place (0: none known)
}

// heldFiles reads the deleted Files still on disk in a holding folder
// (id 0: all of them).
func (s *Service) heldFiles(ctx context.Context, id, titleID int64) ([]heldRow, error) {
	db := s.Store.Read()
	rules, err := importer.ReadFileRules(ctx, db)
	if err != nil {
		return nil, err
	}
	prefix := importer.HoldingDir + "/"
	rows, err := db.QueryContext(ctx, `SELECT f.id, f.state, f.relative_path, f.size, f.updated_at, f.slot_ids, lf.path, lf.label, t.id, t.name, t.kind, t.state
		FROM files f JOIN versions v ON v.id = f.version_id JOIN library_folders lf ON lf.id = v.library_folder_id JOIN titles t ON t.id = v.title_id
		WHERE f.state IN ('gone', 'replaced') AND substr(f.relative_path, 1, ?) = ? AND (? = 0 OR f.id = ?) AND (? = 0 OR t.id = ?) ORDER BY f.updated_at DESC, f.id DESC`,
		len(prefix), prefix, id, id, titleID, titleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []heldRow
	for rows.Next() {
		var h heldRow
		var at, slots, fstate string
		if err := rows.Scan(&h.ID, &fstate, &h.HeldPath, &h.Size, &at, &slots, &h.root, &h.Folder, &h.TitleID, &h.Title, &h.Kind, &h.state); err != nil {
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
		h.Replaced = fstate == "replaced"
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
		if out[i].Replaced {
			var by, was sql.NullString
			var newID sql.NullInt64
			_ = s.Store.ReadEvents().QueryRowContext(ctx, `SELECT json_extract(data, '$.new_file_id'), json_extract(data, '$.from') FROM events
				WHERE kind = 'file_replaced' AND json_extract(data, '$.old_file_id') = ? ORDER BY id DESC LIMIT 1`, out[i].ID).Scan(&newID, &was)
			if newID.Valid {
				_ = db.QueryRowContext(ctx, "SELECT relative_path FROM files WHERE id = ?", newID.Int64).Scan(&by)
			}
			out[i].ReplacedBy = by.String
			out[i].newID = newID.Int64
			// Where it was before the upgrade, as it was named (the held
			// name may carry a .1 the holding folder added).
			if was.Valid && was.String != "" && !strings.HasPrefix(was.String, importer.HoldingDir+"/") {
				out[i].Path = was.String
				continue
			}
		}
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

// HeldFiles lists the deleted Files that can still be restored (titleID 0:
// of every title).
func (s *Service) HeldFiles(ctx context.Context, titleID int64) ([]HeldFile, error) {
	rows, err := s.heldFiles(ctx, 0, titleID)
	out := make([]HeldFile, 0, len(rows))
	for _, h := range rows {
		out = append(out, h.HeldFile)
	}
	return out, err
}

// HeldCount counts the held Files (titleID 0: of every title) from the
// database alone, without looking at the disk: cheap enough for a badge.
// Days the cleanup has already emptied are left out.
func (s *Service) HeldCount(ctx context.Context, titleID int64) (int, error) {
	db := s.Store.Read()
	rules, err := importer.ReadFileRules(ctx, db)
	if err != nil {
		return 0, err
	}
	prefix := importer.HoldingDir + "/"
	cutoff := ""
	if rules.HoldingDays > 0 {
		cutoff = s.now().Truncate(24*time.Hour).AddDate(0, 0, -rules.HoldingDays).Format("2006-01-02")
	}
	// One look per held day: a day the cleanup (or a hand) already
	// emptied is not counted, even after the setting grows.
	rows, err := db.QueryContext(ctx, `SELECT lf.path, substr(f.relative_path, ?, 10), count(*) FROM files f JOIN versions v ON v.id = f.version_id
		JOIN library_folders lf ON lf.id = v.library_folder_id
		WHERE f.state IN ('gone', 'replaced') AND substr(f.relative_path, 1, ?) = ? AND substr(f.relative_path, ?, 10) >= ?
		AND (? = 0 OR v.title_id = ?) GROUP BY lf.path, substr(f.relative_path, ?, 10)`,
		len(prefix)+1, len(prefix), prefix, len(prefix)+1, cutoff, titleID, titleID, len(prefix)+1)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var root, day string
		var c int
		if err := rows.Scan(&root, &day, &c); err != nil {
			return 0, err
		}
		if fi, err := os.Lstat(filepath.Join(root, importer.HoldingDir, day)); err == nil && fi.IsDir() {
			n += c
		}
	}
	return n, rows.Err()
}

// RestoreHeld moves held Files back where they were. A place that is taken
// is never overwritten. A deleted movie, show or artist comes back to the
// Library with the File; it returns the titles that came back.
// how says what to do when the place is taken: "" stops and says so,
// RestoreBoth puts it back under a free name beside the other file, and
// RestoreSwap sends the tracked copy there to Recently deleted first.
func (s *Service) RestoreHeld(ctx context.Context, ids []int64, how string, by domain.Actor) ([]RestoreResult, []int64, error) {
	unlock := s.Store.LockFiles()
	defer unlock()
	var out []RestoreResult
	var back []int64
	for _, id := range ids {
		rows, err := s.heldFiles(ctx, id, 0)
		if err != nil {
			return out, back, err
		}
		if len(rows) == 0 {
			out = append(out, RestoreResult{ID: id, Result: "not in a holding folder"})
			continue
		}
		h := rows[0]
		res, err := s.restoreOne(ctx, &h, how, by)
		if err != nil {
			return out, back, err
		}
		why := res.Result
		if why == "" {
			why = "restored"
			if h.TitleDeleted && !containsID(back, h.TitleID) {
				back = append(back, h.TitleID)
			}
		}
		res.ID, res.Path, res.Result = id, h.Path, why
		out = append(out, res)
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

// restoreOne puts one held File back; a non-empty Result says why not.
func (s *Service) restoreOne(ctx context.Context, h *heldRow, how string, by domain.Actor) (RestoreResult, error) {
	why, taken, err := s.restoreFile(ctx, h, how, by)
	if taken != nil {
		return *taken, err
	}
	return RestoreResult{Result: why}, err
}

// fileAt is the tracked, present File of this title at rel in the Library
// folder root (0: none). Another title's file is never swapped out: it only
// gets "keep both".
func (s *Service) fileAt(ctx context.Context, root, rel string, titleID int64) int64 {
	var id int64
	_ = s.Store.Read().QueryRowContext(ctx, `SELECT f.id FROM files f JOIN versions v ON v.id = f.version_id JOIN library_folders lf ON lf.id = v.library_folder_id
		WHERE lf.path = ? AND f.relative_path = ? AND f.state = 'present' AND v.title_id = ? LIMIT 1`, root, rel, titleID).Scan(&id)
	return id
}

// freeName is rel with " (2)", " (3)" ... before its extension, the first
// that is free on disk.
func freeName(root, rel string) string {
	ext := filepath.Ext(rel)
	base := strings.TrimSuffix(rel, ext)
	for n := 2; ; n++ {
		c := base + " (" + strconv.Itoa(n) + ")" + ext
		if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(c))); errors.Is(err, os.ErrNotExist) {
			return c
		}
	}
}

// setAside sends the tracked copy in the way (id) to Recently deleted, as
// an upgrade does with the copy it replaces; it returns where it went.
func (s *Service) setAside(ctx context.Context, h *heldRow, id int64, at string, by domain.Actor) (string, error) {
	held, err := importer.HoldPath(h.root, at, s.now())
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(h.root, held)
	if err != nil {
		return "", err
	}
	rel = filepath.ToSlash(rel)
	if err := moveFile(filepath.Join(h.root, filepath.FromSlash(at)), held); err != nil {
		return "", err
	}
	ts, who := database.FormatTime(s.now()), ActorText(by)
	if err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE files SET state = 'replaced', relative_path = ?, updated_at = ?, updated_by = ? WHERE id = ?", rel, ts, who, id)
		return err
	}); err != nil {
		_ = moveFile(held, filepath.Join(h.root, filepath.FromSlash(at)))
		return "", err
	}
	return rel, nil
}

// putBack undoes setAside when the restore after it failed.
func (s *Service) putBack(ctx context.Context, h *heldRow, id int64, rel, at string) {
	if moveFile(filepath.Join(h.root, filepath.FromSlash(rel)), filepath.Join(h.root, filepath.FromSlash(at))) == nil {
		_ = s.Store.Write(ctx, func(tx *sql.Tx) error {
			_, err := tx.Exec("UPDATE files SET state = 'present', relative_path = ? WHERE id = ?", at, id)
			return err
		})
	}
}

func (s *Service) restoreFile(ctx context.Context, h *heldRow, how string, by domain.Actor) (string, *RestoreResult, error) {
	if h.TitleDeleted {
		var other int64
		_ = s.Store.Read().QueryRowContext(ctx, `SELECT o.id FROM titles o JOIN titles t ON t.id = ? WHERE o.id != t.id AND o.kind = t.kind
			AND o.state != 'removed' AND EXISTS (SELECT 1 FROM json_each(t.source_ids) a JOIN json_each(o.source_ids) b ON a.key = b.key AND a.value = b.value)`,
			h.TitleID).Scan(&other)
		if other != 0 {
			return "it was added to the Library again; restore it by hand", nil, nil
		}
	}
	from := filepath.Join(h.root, filepath.FromSlash(h.HeldPath))
	to := filepath.Join(h.root, filepath.FromSlash(h.Path))
	if !filepath.IsLocal(filepath.FromSlash(h.Path)) || strings.HasPrefix(h.Path, importer.HoldingDir+"/") ||
		!importer.InsideReal(h.root, filepath.Dir(from)) || !importer.InsideReal(h.root, filepath.Dir(to)) {
		return "its place is outside the Library folder", nil, nil
	}
	var swapped int64 // the copy sent to Recently deleted in its place
	swappedTo, swappedAt := "", ""
	_, lerr := os.Lstat(to)
	if lerr != nil && !errors.Is(lerr, os.ErrNotExist) {
		return lerr.Error(), nil, nil
	}
	occupied := lerr == nil
	inWay, inWayAt := int64(0), h.Path
	if occupied {
		inWay = s.fileAt(ctx, h.root, h.Path, h.TitleID)
	} else if h.Replaced && h.newID != 0 {
		// The better copy usually has another name: it still holds the
		// place, so restoring asks rather than quietly keeping both.
		_ = s.Store.Read().QueryRowContext(ctx, `SELECT f.relative_path FROM files f JOIN versions v ON v.id = f.version_id JOIN library_folders lf ON lf.id = v.library_folder_id
			WHERE f.id = ? AND f.state = 'present' AND lf.path = ?`, h.newID, h.root).Scan(&inWayAt)
		if inWayAt != h.Path {
			if _, err := os.Lstat(filepath.Join(h.root, filepath.FromSlash(inWayAt))); err == nil {
				inWay = h.newID
			}
		}
	}
	if occupied || inWay != 0 {
		switch {
		case how == RestoreBoth:
			if occupied {
				h.Path = freeName(h.root, h.Path)
				to = filepath.Join(h.root, filepath.FromSlash(h.Path))
			}
		case how == RestoreSwap && inWay != 0 && inWay != h.ID:
			rel, err := s.setAside(ctx, h, inWay, inWayAt, by)
			if err != nil {
				return err.Error(), nil, nil
			}
			swapped, swappedTo, swappedAt = inWay, rel, inWayAt
		default:
			return "", &RestoreResult{Result: "a file is already at " + inWayAt, Taken: true, CanSwap: inWay != 0 && inWay != h.ID}, nil
		}
	}
	if err := moveFile(from, to); err != nil {
		if swapped != 0 {
			s.putBack(ctx, h, swapped, swappedTo, swappedAt)
		}
		return err.Error(), nil, nil
	}
	if !importer.InsideReal(h.root, filepath.Dir(to)) {
		// A link in the way led it out: put it back where it waited.
		_ = moveFile(to, from)
		return "its place is outside the Library folder", nil, nil
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
		if swapped != 0 {
			// The swapped-out copy's Slots point at this one again.
			if _, err := tx.Exec(`UPDATE slots SET state = 'in_library', file_id = ?, last_reason = 'its earlier copy was restored', updated_at = ?, updated_by = ?
				WHERE file_id = ?`, h.ID, ts, who, swapped); err != nil {
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
		if swapped != 0 {
			s.putBack(ctx, h, swapped, swappedTo, swappedAt)
		}
		return "", nil, err
	}
	titleID := h.TitleID
	if swapped != 0 {
		s.Emit(ctx, &domain.Event{Actor: by, Kind: "file_replaced", TitleID: &titleID, Summary: "Swapped back to " + filepath.Base(h.Path),
			Data: map[string]any{"old_file_id": swapped, "new_file_id": h.ID, "from": swappedAt, "moved_to": swappedTo, "swapped": true}, GroupKey: GroupKey(titleID)})
	}
	s.Emit(ctx, &domain.Event{Actor: by, Kind: domain.EventFileBack, TitleID: &titleID, Summary: "Restored " + filepath.Base(h.Path) + " from the holding folder",
		Data: map[string]any{"file_id": h.ID, "from": h.HeldPath, "to": h.Path, "subtitles": subsBack, "title_back": h.TitleDeleted}, GroupKey: GroupKey(titleID)})
	return "", nil, nil
}
