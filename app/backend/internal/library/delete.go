package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
)

// KindCleanHolding empties old days out of the holding folders.
const KindCleanHolding = "clean_holding"

// ErrAlreadyRemoved says the movie or show was already deleted.
var ErrAlreadyRemoved = errors.New("this was already deleted from the Library")

var ErrProtectedCopy = errors.New("a protected copy is retained; the whole title stays in the Library")

// RemoveChoice is what a delete does besides taking the movie out of the
// Library. Both are off by default: files stay on disk, and lists may add
// the movie again.
type RemoveChoice struct {
	DeleteFiles           bool `json:"delete_files"`
	NeverAdd              bool `json:"never_add"`
	AcknowledgeProtection bool `json:"acknowledge_protection"`
}

// RemoveFile is one File in a delete. Paths are relative to the Library
// folder; To is where a held file goes in the holding folder.
type RemoveFile struct {
	FileID          int64  `json:"file_id"`
	LibraryFolderID int64  `json:"library_folder_id"`
	Origin          string `json:"origin"`
	Protected       bool   `json:"protected"`
	Path            string `json:"path"`
	To              string `json:"to,omitempty"`
	Why             string `json:"why"`
	// Episodes the file holds (a show), such as S01E02, or its track (an
	// artist), such as "Old Days: track 03 Song".
	Episodes []string `json:"episodes,omitempty"`
	// Subtitles are the subtitle files beside a held video; they go to the
	// holding folder with it.
	Subtitles []SubtitleMove `json:"subtitles,omitempty"`
	root      string
}

// Root is the Library folder path the file lives under.
func (f RemoveFile) Root() string { return f.root }

// holdSubtitles plans the subtitle files beside a video that is held: each
// goes to its own free place in the same day of the holding folder. The
// two files of a VobSub pair get the same free name (name.N.idx and
// name.N.sub when name is taken), so they still match once held.
func holdSubtitles(root, rel string, now time.Time) []SubtitleMove {
	var out []SubtitleMove
	for _, group := range besideSubtitles(root, rel) {
		ms := make([]SubtitleMove, len(group))
		for i, sub := range group {
			ms[i] = SubtitleMove{From: sub, pair: i > 0}
		}
		for n := 0; n < 1000; n++ {
			free, skip := true, ""
			for i, sub := range group {
				want := sub
				if n > 0 {
					want = strings.TrimSuffix(sub, path.Ext(sub)) + "." + strconv.Itoa(n) + path.Ext(sub)
				}
				to, err := importer.HoldPath(root, want, now)
				if err != nil {
					skip = err.Error()
					break
				}
				r, err := filepath.Rel(root, to)
				if err != nil {
					skip = err.Error()
					break
				}
				if filepath.ToSlash(r) != path.Join(importer.HoldingDir, now.UTC().Format("2006-01-02"), want) {
					free = false // HoldPath had to add its own suffix: taken
				}
				ms[i].To = filepath.ToSlash(r)
			}
			if skip != "" {
				for i := range ms {
					ms[i].To, ms[i].Skip = "", skip
				}
				break
			}
			if free || len(group) == 1 {
				break
			}
		}
		out = append(out, ms...)
	}
	return out
}

// moveSubtitle moves one held subtitle; tests make it fail.
var moveSubtitle = moveFile

// RemovePlan says what deleting a movie does.
type RemovePlan struct {
	TitleID int64  `json:"title_id"`
	Name    string `json:"name"`
	// Scope is what is deleted: title (the whole movie or show), season or
	// episodes; Season and Episodes name them.
	Scope    string   `json:"scope"`
	Season   *int     `json:"season,omitempty"`
	Episodes []string `json:"episodes,omitempty"`
	// An album or tracks of an artist: the album and the tracks deleted.
	AlbumID     *int64       `json:"album_id,omitempty"`
	Album       string       `json:"album,omitempty"`
	Tracks      []string     `json:"tracks,omitempty"`
	Choice      RemoveChoice `json:"choice"`
	Applied     bool         `json:"applied"`
	Held        []RemoveFile `json:"held"`
	Kept        []RemoveFile `json:"kept"`
	HoldingDays int          `json:"holding_days"`
	Protected   []RemoveFile `json:"protected"`
	Skipped     bool         `json:"skipped"`
	// Downloads still running for the movie are left in their Downloader.
	ActiveDownloads int `json:"active_downloads"`
	itemIDs         []int64
}

// ItemIDs are the episode or track Items a partial removal ends.
func (p RemovePlan) ItemIDs() []int64 { return p.itemIDs }

// planRemove works out a delete. Nothing changes.
func planRemove(ctx context.Context, db Reader, titleID int64, c RemoveChoice, now time.Time) (RemovePlan, mediafiles.Title, error) {
	var t mediafiles.Title
	if err := database.Get(ctx, db, "titles", titleID, &t); err != nil {
		return RemovePlan{}, t, err
	}
	if t.State == "removed" {
		return RemovePlan{}, t, ErrAlreadyRemoved
	}
	rules, err := importer.ReadFileRules(ctx, db)
	if err != nil {
		return RemovePlan{}, t, err
	}
	p := RemovePlan{TitleID: t.ID, Name: t.Name, Scope: "title", Choice: c, Held: []RemoveFile{}, Kept: []RemoveFile{}, HoldingDays: rules.HoldingDays}
	files, err := database.Select[mediafiles.File](ctx, db, "files",
		"state IN ('present', 'missing') AND version_id IN (SELECT id FROM versions WHERE title_id = ?) ORDER BY id", t.ID)
	if err != nil {
		return p, t, err
	}
	for _, f := range files {
		var v mediafiles.Version
		var lf mediafiles.LibraryFolder
		if err := database.Get(ctx, db, "versions", f.VersionID, &v); err != nil {
			return p, t, err
		}
		if err := database.Get(ctx, db, "library_folders", v.LibraryFolderID, &lf); err != nil {
			return p, t, err
		}
		rf := RemoveFile{FileID: f.ID, LibraryFolderID: lf.ID, Origin: f.Origin, Path: f.RelativePath, Protected: f.Protected, root: lf.Path}
		if f.Protected {
			p.Protected = append(p.Protected, rf)
			if !c.AcknowledgeProtection || !c.DeleteFiles || f.Origin != "imported" && f.Origin != "adopted" {
				p.Skipped = true
			}
		}
		for _, sid := range f.SlotIDs {
			switch t.Kind {
			case "show":
				rf.Episodes = append(rf.Episodes, EpisodeLabel(ctx, db, sid))
			case "artist":
				rf.Episodes = append(rf.Episodes, trackLabelOf(ctx, db, sid))
			}
		}
		switch {
		case !c.DeleteFiles:
			rf.Why = "kept: files stay on disk unless you choose to delete them"
		case f.Origin != "imported" && f.Origin != "adopted":
			rf.Why = "kept: found on disk, not imported or adopted; HolyArrs never deletes it"
		case f.State == "missing":
			rf.Why = "kept: the file is not on disk"
		default:
			to, err := importer.HoldPath(lf.Path, f.RelativePath, now)
			if err != nil {
				rf.Why = "kept: " + err.Error()
				break
			}
			rel, _ := filepath.Rel(lf.Path, to)
			rf.To = filepath.ToSlash(rel)
			rf.Why = fmt.Sprintf("goes to the holding folder for %d days", rules.HoldingDays)
			if rules.HoldingDays == 0 {
				rf.Why = "goes to the holding folder until you empty it"
			}
			if t.Kind != "artist" {
				rf.Subtitles = holdSubtitles(lf.Path, f.RelativePath, now)
			}
			p.Held = append(p.Held, rf)
			continue
		}
		p.Kept = append(p.Kept, rf)
	}
	if p.Skipped {
		for _, h := range p.Held {
			h.To = ""
			h.Why = ErrProtectedCopy.Error()
			p.Kept = append(p.Kept, h)
		}
		p.Held = []RemoveFile{}
	}
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM downloads WHERE state IN ('sent', 'queued', 'downloading', 'stalled', 'paused')
		AND EXISTS (SELECT 1 FROM json_each(downloads.slot_ids) j JOIN slots s ON s.id = j.value JOIN items i ON i.id = s.item_id WHERE i.title_id = ?)`, t.ID).Scan(&p.ActiveDownloads)
	return p, t, nil
}

// Remove deletes a movie from the Library (apply false: only the plan).
// Its Slots end, Keep up to date goes off, and with the choices its
// imported and adopted files go to the holding folder and it joins the
// Never-add list. Files HolyArrs did not import or adopt are never touched.
func (s *Service) Remove(ctx context.Context, titleID int64, c RemoveChoice, apply bool, by domain.Actor) (RemovePlan, error) {
	unlock := s.Store.LockFiles()
	defer unlock()
	return s.RemoveLocked(ctx, titleID, c, apply, by)
}

// RemoveLocked is also used by a collection confirmation that already holds
// placement coordination while validating its membership review.
func (s *Service) RemoveLocked(ctx context.Context, titleID int64, c RemoveChoice, apply bool, by domain.Actor) (RemovePlan, error) {
	p, t, err := planRemove(ctx, s.Store.Read(), titleID, c, s.now())
	if err != nil || !apply {
		return p, err
	}
	if p.Skipped {
		return p, ErrProtectedCopy
	}
	if err := s.holdFiles(ctx, &p, t.ID, by); err != nil {
		return p, err
	}
	what := "movie"
	switch t.Kind {
	case "show":
		what = "show"
	case "artist":
		what = "artist"
	}
	err = s.Store.Write(ctx, func(tx *sql.Tx) error {
		ts, who := database.FormatTime(s.now()), ActorText(by)
		if _, err := tx.Exec("UPDATE titles SET state = 'removed', keep_up_to_date = 0, updated_at = ?, updated_by = ? WHERE id = ?", ts, who, t.ID); err != nil {
			return err
		}
		// A deleted title's Slots end whatever state they were in.
		if _, err := tx.Exec(`UPDATE slots SET state = 'removed', last_reason = ?, held_release_id = NULL, active_download_id = NULL,
			updated_at = ?, updated_by = ? WHERE item_id IN (SELECT id FROM items WHERE title_id = ?)`, "the "+what+" was deleted", ts, who, t.ID); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE versions SET keep_up_to_date = 0 WHERE title_id = ?", t.ID); err != nil {
			return err
		}
		if c.NeverAdd && t.Kind == "show" {
			if on, err := showOnNeverAdd(ctx, tx, t.SourceIDs); err != nil {
				return err
			} else if !on {
				ids := map[string]string{}
				for k, v := range t.SourceIDs {
					ids[k] = v
				}
				n := mediafiles.NeverAdd{Kind: "show", SourceIDs: ids, Name: t.Name, Reason: "deleted from the Library"}
				if _, err := database.Insert(tx, "never_add", &n, by); err != nil {
					return err
				}
			}
		}
		if c.NeverAdd && t.Kind == "artist" && t.SourceIDs["musicbrainz"] != "" {
			if on, err := ArtistOnNeverAdd(ctx, tx, t.SourceIDs["musicbrainz"]); err != nil {
				return err
			} else if !on {
				n := mediafiles.NeverAdd{Kind: "artist", SourceIDs: map[string]string{"musicbrainz": t.SourceIDs["musicbrainz"]}, Name: t.Name, Reason: "deleted from the Library"}
				if _, err := database.Insert(tx, "never_add", &n, by); err != nil {
					return err
				}
			}
		}
		if c.NeverAdd && t.Kind == "movie" && t.SourceIDs["tmdb"] != "" {
			if on, err := OnNeverAdd(ctx, tx, t.SourceIDs["tmdb"]); err != nil {
				return err
			} else if !on {
				n := mediafiles.NeverAdd{Kind: "movie", SourceIDs: map[string]string{"tmdb": t.SourceIDs["tmdb"]}, Name: t.Name, Reason: "deleted from the Library"}
				if _, err := database.Insert(tx, "never_add", &n, by); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return p, err
	}
	if c.DeleteFiles {
		s.removeEmptyFolders(ctx, t.ID, p.Held, true)
	}
	p.Applied = true
	s.Emit(ctx, &domain.Event{Actor: by, Kind: domain.EventTitleRemoved, TitleID: &t.ID, Summary: "Deleted " + t.Name + " from the Library",
		Data: map[string]any{"delete_files": c.DeleteFiles, "never_add": c.NeverAdd, "protection_acknowledged": c.AcknowledgeProtection, "held": len(p.Held), "kept": len(p.Kept)}, GroupKey: GroupKey(t.ID)})
	s.Publish("title", map[string]any{"id": t.ID, "state": "removed"})
	return p, nil
}

// removeEmptyFolders removes the folders a delete with files left empty:
// season folders (named by the season folder pattern, the default "Season
// N" or the specials folder) and the folders a held file came from, each
// with any empty folders nested in it; then, for a whole title (showFolder
// true), the Version folder if it is empty. A person's own folders that
// were empty before (extras, .actors) stay, and any folder holding a file
// or a link stays.
func (s *Service) removeEmptyFolders(ctx context.Context, titleID int64, held []RemoveFile, showFolder bool) {
	db := s.Store.Read()
	vs, _ := database.Select[mediafiles.Version](ctx, db, "versions", "title_id = ?", titleID)
	names := map[string]bool{}
	if st, err := mediafiles.ReadNaming(ctx, db); err == nil {
		rows, err := db.QueryContext(ctx, "SELECT DISTINCT json_extract(numbers, '$.season') FROM items WHERE title_id = ? AND json_extract(numbers, '$.season') IS NOT NULL", titleID)
		if err == nil {
			for rows.Next() {
				var n int
				if rows.Scan(&n) == nil {
					for _, name := range []string{mediafiles.SeasonFolder(st, n), fmt.Sprintf("Season %d", n), fmt.Sprintf("Season %02d", n)} {
						if name != "" {
							names[strings.ToLower(name)] = true
						}
					}
				}
			}
			_ = rows.Close()
		}
	}
	from := map[string]bool{} // folders a held file came from
	for _, h := range held {
		from[filepath.Clean(filepath.Dir(filepath.Join(h.root, filepath.FromSlash(h.Path))))] = true
	}
	for _, v := range vs {
		dir, _, err := versionFolder(ctx, db, v)
		if err != nil || v.FolderName == "" {
			continue
		}
		dir = filepath.Clean(dir)
		// Folders a held file came from deeper than a season or album
		// folder (Season 1/Part A, Album/CD1): remove each empty one up to
		// its child of dir, and that child too when it is left empty.
		for f := range from {
			for d := f; d != dir && strings.HasPrefix(d, dir+string(filepath.Separator)); d = filepath.Dir(d) {
				if filepath.Dir(d) == dir {
					from[d] = true // a child of the show or artist folder: handled below
					break
				}
				removeEmptyTree(d)
			}
		}
		if ents, err := os.ReadDir(dir); err == nil {
			for _, e := range ents {
				p := filepath.Join(dir, e.Name())
				if e.IsDir() && e.Type()&os.ModeSymlink == 0 && (names[strings.ToLower(e.Name())] || from[p]) {
					removeEmptyTree(p)
				}
			}
		}
		if showFolder {
			_ = os.Remove(dir) // only an empty one goes
		}
	}
}

// removeEmptyTree removes dir and the folders in it when none of them holds
// a file or a link; anything else leaves the whole tree as it is.
func removeEmptyTree(dir string) {
	var dirs []string
	empty := true
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() || d.Type()&os.ModeSymlink != 0 {
			empty = false
			return filepath.SkipAll
		}
		dirs = append(dirs, p)
		return nil
	})
	if !empty {
		return
	}
	for i := len(dirs) - 1; i >= 0; i-- { // deepest first
		_ = os.Remove(dirs[i])
	}
}

// holdFiles moves each held File of a plan to the holding folder. Each
// row moves first: a stop between the two leaves the file where it was,
// never lost. On a failure the rest stay and are listed as kept.
func (s *Service) holdFiles(ctx context.Context, p *RemovePlan, titleID int64, by domain.Actor) error {
	for i, h := range p.Held {
		err := s.Store.Write(ctx, func(tx *sql.Tx) error {
			_, err := tx.Exec("UPDATE files SET state = 'gone', relative_path = ?, updated_at = ?, updated_by = ? WHERE id = ?",
				h.To, database.FormatTime(s.now()), ActorText(by), h.FileID)
			if err == nil {
				// Built-in tracks leave with the video; a beside subtitle's
				// row changes only once that subtitle has moved (below).
				_, err = tx.Exec("UPDATE subtitles SET state = 'gone', updated_at = ? WHERE file_id = ? AND state = 'present' AND place <> 'beside'", database.FormatTime(s.now()), h.FileID)
			}
			return err
		})
		if err != nil {
			return err
		}
		if err := moveFile(filepath.Join(h.root, filepath.FromSlash(h.Path)), filepath.Join(h.root, filepath.FromSlash(h.To))); err != nil {
			_ = s.Store.Write(ctx, func(tx *sql.Tx) error {
				_, err := tx.Exec("UPDATE files SET state = 'present', relative_path = ? WHERE id = ?", h.Path, h.FileID)
				return err
			})
			// The rest stay; nothing is deleted half-way.
			p.Held, p.Kept = p.Held[:i], append(p.Kept, p.Held[i:]...)
			return fmt.Errorf("moving %s to the holding folder: %w", filepath.Base(h.Path), err)
		}
		// Only a successfully held, explicitly acknowledged copy loses its
		// protection, so the holding retention policy can eventually clear it.
		if h.Protected {
			if err := s.Store.Write(ctx, func(tx *sql.Tx) error {
				_, err := tx.Exec("UPDATE files SET protected = 0 WHERE id = ? AND state = 'gone' AND relative_path = ?", h.FileID, h.To)
				return err
			}); err != nil {
				return err
			}
		}
		// The subtitles beside it follow; one that cannot move stays where
		// it is and says why. The two files of a VobSub pair (.idx, then its
		// .sub) move together or not at all.
		ms := p.Held[i].Subtitles
		stay := map[string]bool{} // subtitles left in place: their rows stay
		for j, m := range ms {
			if m.Skip != "" || m.To == "" {
				stay[m.From] = true
				continue
			}
			if m.pair && ms[j-1].Result != "held" {
				ms[j].Result = "failed: its other half did not move"
				stay[m.From] = true
				continue
			}
			from, to := filepath.Join(h.root, filepath.FromSlash(m.From)), filepath.Join(h.root, filepath.FromSlash(m.To))
			err := error(nil)
			if !importer.InsideReal(h.root, filepath.Dir(from)) || !importer.InsideReal(h.root, filepath.Dir(to)) {
				err = errors.New("a link leads it outside the Library folder")
			} else {
				err = moveSubtitle(from, to)
			}
			if err != nil {
				ms[j].Result = "failed: " + err.Error()
				stay[m.From] = true
				if m.pair { // put its .idx back so the pair stays whole
					prev := ms[j-1]
					if moveFile(filepath.Join(h.root, filepath.FromSlash(prev.To)), filepath.Join(h.root, filepath.FromSlash(prev.From))) == nil {
						ms[j-1].Result = "failed: its other half did not move"
						stay[prev.From] = true
					}
				}
				continue
			}
			ms[j].Result = "held"
		}
		var subs []string
		for _, m := range ms {
			if m.Result == "held" {
				subs = append(subs, m.To)
			}
		}
		// Each held subtitle's row is gone now; a row whose subtitle stayed
		// keeps saying where it is.
		_ = s.Store.Write(ctx, func(tx *sql.Tx) error {
			rows, err := tx.Query("SELECT id, relative_path FROM subtitles WHERE file_id = ? AND state = 'present' AND place = 'beside'", h.FileID)
			if err != nil {
				return err
			}
			var gone []int64
			for rows.Next() {
				var id int64
				var rel sql.NullString
				if rows.Scan(&id, &rel) == nil && !stay[rel.String] {
					gone = append(gone, id)
				}
			}
			rows.Close()
			for _, id := range gone {
				if _, err := tx.Exec("UPDATE subtitles SET state = 'gone', updated_at = ? WHERE id = ?", database.FormatTime(s.now()), id); err != nil {
					return err
				}
			}
			return nil
		})
		s.Emit(ctx, &domain.Event{Actor: by, Kind: domain.EventFileHeld, TitleID: &titleID, Summary: "Moved " + filepath.Base(h.Path) + " to the holding folder",
			Data: map[string]any{"file_id": h.FileID, "from": h.Path, "to": h.To, "episodes": h.Episodes, "subtitles": subs}, GroupKey: GroupKey(titleID)})
	}
	return nil
}

// CleanHolding empties holding folder days older than the holding setting
// (0 keeps everything). Only folders named as a day (2 January 2006) directly
// in a Library folder's holding folder are removed; it returns how many.
func (s *Service) CleanHolding(ctx context.Context, by domain.Actor) (int, error) {
	unlock := s.Store.LockFiles()
	defer unlock()
	db := s.Store.Read()
	rules, err := importer.ReadFileRules(ctx, db)
	if err != nil || rules.HoldingDays == 0 {
		return 0, err
	}
	today := s.now().Truncate(24 * time.Hour)
	cutoff := today.AddDate(0, 0, -rules.HoldingDays)
	lfs, err := database.Select[mediafiles.LibraryFolder](ctx, db, "library_folders", "1=1 ORDER BY id")
	if err != nil {
		return 0, err
	}
	n := 0
	for _, lf := range lfs {
		hold := filepath.Join(lf.Path, importer.HoldingDir)
		if fi, err := os.Lstat(hold); err != nil || !fi.IsDir() {
			continue
		}
		ents, err := os.ReadDir(hold)
		if err != nil {
			continue
		}
		for _, e := range ents {
			day, err := time.Parse("2006-01-02", e.Name())
			if err != nil || !e.IsDir() || e.Type()&os.ModeSymlink != 0 || !day.Before(cutoff) {
				continue
			}
			prefix := importer.HoldingDir + "/" + e.Name() + "/"
			var protected int
			if err := db.QueryRowContext(ctx, `SELECT count(*) FROM files WHERE protected = 1 AND substr(relative_path, 1, ?) = ?
				AND version_id IN (SELECT id FROM versions WHERE library_folder_id = ?)`, len(prefix), prefix, lf.ID).Scan(&protected); err != nil {
				return n, err
			}
			if protected > 0 {
				continue
			}
			if err := os.RemoveAll(filepath.Join(hold, e.Name())); err != nil {
				return n, err
			}
			n++
			if err := s.Store.Write(ctx, func(tx *sql.Tx) error {
				_, err := tx.Exec(`UPDATE files SET state = 'gone', updated_at = ?, updated_by = ? WHERE state IN ('replaced', 'gone')
					AND substr(relative_path, 1, ?) = ? AND version_id IN (SELECT id FROM versions WHERE library_folder_id = ?)`,
					database.FormatTime(s.now()), ActorText(by), len(prefix), prefix, lf.ID)
				return err
			}); err != nil {
				return n, err
			}
			s.Emit(ctx, &domain.Event{Actor: by, Kind: domain.EventHoldingCleaned,
				Summary: fmt.Sprintf("Emptied %s from the holding folder of %s", e.Name(), lf.Label),
				Data:    map[string]any{"library_folder_id": lf.ID, "day": e.Name(), "kept_days": rules.HoldingDays}})
		}
	}
	return n, nil
}

func (s *Service) handleCleanHolding(ctx context.Context, r *jobs.Run) error {
	n, err := s.CleanHolding(ctx, JobActor(r))
	r.SetResult(map[string]any{"days_emptied": n})
	return err
}
