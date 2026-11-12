package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
)

func (s *Service) handleScanLibrary(ctx context.Context, r *jobs.Run) error {
	if r.Job.Subject == nil || r.Job.Subject.Type != "library_folder" {
		return errors.New("scan_library_folder needs a library_folder subject")
	}
	after, _ := r.Job.Params["after"].(string)
	var lf mediafiles.LibraryFolder
	if err := database.Get(ctx, s.Store.Read(), "library_folders", r.Job.Subject.ID, &lf); err != nil {
		return ErrNoFolder
	}
	ctx = WithScanProgress(ctx, scanProgress(ctx, r, time.Now))
	var p AdoptPlan
	var err error
	switch lf.Kind {
	case "show":
		p, err = s.showParts().PlanAdopt(ctx, lf.ID, after)
	case "music":
		// A restarted scan goes on from the part it saved; each artist
		// folder's result is saved as it is done.
		var seed *AdoptPlan
		if old, ok := r.Job.Result["plan"]; ok {
			var sp AdoptPlan
			if raw, jerr := json.Marshal(old); jerr == nil && json.Unmarshal(raw, &sp) == nil && sp.Partial {
				seed = &sp
			}
		}
		p, err = planMusicAdopt(ctx, s.Store.Read(), s.Artists, lf.ID, after, seed, func(part AdoptPlan) error {
			return r.SaveResult(ctx, map[string]any{"dry_run": true, "plan": part})
		})
	default:
		p, err = PlanAdoptFrom(ctx, s.Store.Read(), s.Metadata, lf.ID, after)
	}
	if err != nil {
		return err
	}
	r.SetResult(map[string]any{"dry_run": true, "plan": p})
	s.Emit(ctx, &domain.Event{Actor: JobActor(r), Kind: domain.EventLibraryScanned, JobID: &r.Job.ID,
		Summary: fmt.Sprintf("Scanned %s: %d matched, %d unsure, %d unmatched", p.Path, p.Counts["matched"], p.Counts["unsure"], p.Counts["unmatched"]),
		Data:    map[string]any{"library_folder_id": p.LibraryFolderID, "counts": p.Counts}})
	return nil
}

// scanProgressEvery is the least time between two progress saves of a scan.
const scanProgressEvery = 2 * time.Second

// scanProgress saves a scan's progress as "done/total" on its Job, at most
// every scanProgressEvery and always when it ends, so the page shows a long
// scan moving (and keeps waiting for it) without a write per folder.
func scanProgress(ctx context.Context, r *jobs.Run, now func() time.Time) func(done, total int) {
	var last time.Time
	return func(done, total int) {
		if total <= 0 || (done < total && done > 0 && now().Sub(last) < scanProgressEvery) {
			return
		}
		last = now()
		_ = r.Progress(ctx, float64(done)/float64(total), strconv.Itoa(done)+"/"+strconv.Itoa(total))
	}
}

func (s *Service) handleAdopt(ctx context.Context, r *jobs.Run) error {
	if r.Job.Subject == nil || r.Job.Subject.Type != "library_folder" {
		return errors.New("adopt_library needs a library_folder subject")
	}
	raw, _ := r.Job.Params["choices"].([]any)
	var cs []AdoptChoice
	for _, x := range raw {
		m, _ := x.(map[string]any)
		f, _ := m["folder"].(string)
		t, _ := m["tmdb_id"].(string)
		id, _ := m["id"].(string)
		cs = append(cs, AdoptChoice{Folder: f, TMDB: t, ID: id})
	}
	res, err := s.Adopt(ctx, r.Job.Subject.ID, cs, JobActor(r))
	adopted := 0
	for _, x := range res {
		if x.TitleID != 0 {
			adopted++
		}
	}
	if follow, _ := r.Job.Params["follow"].(bool); follow {
		if ferr := s.followAdopted(ctx, r.Job.AskedBy, res); ferr != nil && err == nil {
			err = ferr
		}
	}
	r.SetResult(map[string]any{"adopted": adopted, "results": res})
	return err
}

// followAdopted makes the person who imported titles follow them, within a
// Kid's limit (titles above it are left out).
func (s *Service) followAdopted(ctx context.Context, by domain.Actor, res []AdoptResult) error {
	if by.Kind != "user" || by.ID == 0 {
		return nil
	}
	var max string
	if err := s.Store.Read().QueryRowContext(ctx, "SELECT max_rating FROM users WHERE id = ?", by.ID).Scan(&max); err != nil {
		return nil // no such person: nothing to follow
	}
	now := database.FormatTime(time.Now())
	return s.Store.Write(ctx, func(tx *sql.Tx) error {
		for _, x := range res {
			if x.TitleID == 0 {
				continue
			}
			if max != "" {
				var kind, rating string
				if err := tx.QueryRow("SELECT kind, coalesce(rating, '') FROM titles WHERE id = ?", x.TitleID).Scan(&kind, &rating); err != nil || !metadata.RatingAllows(max, kind, rating) {
					continue
				}
			}
			if _, err := tx.Exec("INSERT OR IGNORE INTO follows (user_id, title_id, created_at) VALUES (?, ?, ?)", by.ID, x.TitleID, now); err != nil {
				return err
			}
		}
		return nil
	})
}

// RescanResult counts what a rescan saw.
type RescanResult struct {
	Folders     int `json:"folders"`
	Unreachable int `json:"unreachable"`
	Missing     int `json:"missing"`
	Back        int `json:"back"`
	Found       int `json:"found"`
}

// Rescan looks at every movie and show Library folder: Files not on disk become
// missing, missing ones that are back become present, and unknown videos
// in a movie's folder are recorded as found (never touched). A Library
// folder that is gone or empty while it should hold files (a mount that
// dropped), or where every known file or most of many are gone at once (a
// leftover folder, a pool that lost a disk), is a warning, and none of its
// Files becomes missing.
func (s *Service) Rescan(ctx context.Context, by domain.Actor) (RescanResult, error) {
	unlock := s.Store.LockFiles()
	defer unlock()
	var res RescanResult
	backfillLeft := backfillPerRescan
	db := s.Store.Read()
	lfs, err := database.Select[mediafiles.LibraryFolder](ctx, db, "library_folders", "kind IN ('movie', 'show', 'music') ORDER BY id")
	if err != nil {
		return res, err
	}
	for _, lf := range lfs {
		res.Folders++
		if err := s.rescanFolder(ctx, lf, 0, &res, &backfillLeft, by); err != nil {
			return res, err
		}
	}
	s.Emit(ctx, &domain.Event{Actor: by, Kind: domain.EventLibraryRescanned,
		Summary: fmt.Sprintf("Rescanned %d Library folders: %d missing, %d back, %d found, %d not reachable", res.Folders, res.Missing, res.Back, res.Found, res.Unreachable),
		Data:    map[string]any{"folders": res.Folders, "missing": res.Missing, "back": res.Back, "found": res.Found, "unreachable": res.Unreachable}})
	return res, nil
}

// rescanFolder is one Library folder's part of a rescan, or only titleID's
// files in it when titleID is set. The unreachable and mass-gone guards
// hold the same way either way.
func (s *Service) rescanFolder(ctx context.Context, lf mediafiles.LibraryFolder, titleID int64, res *RescanResult, backfillLeft *int, by domain.Actor) error {
	db := s.Store.Read()
	files, err := database.Select[mediafiles.File](ctx, db, "files", `state IN ('present', 'missing') AND version_id IN
		(SELECT v.id FROM versions v JOIN titles t ON t.id = v.title_id WHERE v.library_folder_id = ? AND t.state != 'removed' AND (? = 0 OR t.id = ?)) ORDER BY id`, lf.ID, titleID, titleID)
	if err != nil {
		return err
	}
	if why := unreachable(lf.Path, len(files)); why != "" {
		res.Unreachable++
		s.Emit(ctx, &domain.Event{Actor: by, Kind: domain.EventFolderUnreachable,
			Summary: "Library folder " + lf.Label + " cannot be reached (" + why + "); no file was marked missing",
			Data:    map[string]any{"library_folder_id": lf.ID, "path": lf.Path, "reason": why}})
		return nil
	}
	knownPaths := map[string]bool{}
	type change struct {
		f  mediafiles.File
		to string
	}
	var changes []change
	present, gone := 0, 0
	for _, f := range files {
		knownPaths[strings.ToLower(f.RelativePath)] = true
		_, statErr := os.Stat(filepath.Join(lf.Path, filepath.FromSlash(f.RelativePath)))
		if lf.Kind != "music" && f.Role == "main" && statErr == nil {
			backfill := *backfillLeft > 0 && importer.NeedsFacts(f.MediaInfo)
			if info, changed := InspectLibraryVideo(lf.Path, f.RelativePath, f.MediaInfo, backfill); changed {
				if err := s.updateFileInspection(ctx, f.ID, info, by); err != nil {
					return err
				}
				if backfill {
					(*backfillLeft)--
				}
			}
		}
		if f.State == "present" {
			present++
		}
		switch {
		case f.State == "present" && errors.Is(statErr, os.ErrNotExist):
			gone++
			changes = append(changes, change{f, "missing"})
		case f.State == "missing" && statErr == nil:
			changes = append(changes, change{f, "present"})
		}
	}
	// A mount that dropped while leaving something in the folder, or a
	// pool that lost a disk: every known file, or most of many, gone at
	// once. None becomes missing; the person looks first.
	massGone := (gone >= 2 && gone == present) || (gone >= massGoneMin && gone*2 > present)
	if massGone {
		res.Unreachable++
		s.Emit(ctx, &domain.Event{Actor: by, Kind: domain.EventFolderUnreachable,
			Summary: fmt.Sprintf("Library folder %s: %d of %d files are gone at once; is a drive or share not mounted? No file was marked missing", lf.Label, gone, present),
			Data:    map[string]any{"library_folder_id": lf.ID, "path": lf.Path, "reason": "most files gone at once", "gone": gone, "known": present}})
	}
	for _, c := range changes {
		if c.to == "missing" && massGone {
			continue
		}
		if c.to == "missing" {
			res.Missing++
		} else {
			res.Back++
		}
		if err := s.setFileState(ctx, c.f, c.to, by); err != nil {
			return err
		}
	}
	n, err := s.recordFound(ctx, lf, titleID, knownPaths, by)
	if err != nil {
		return err
	}
	res.Found += n
	return nil
}

func inspectionSize(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case int:
		return int64(n), true
	case float64:
		return int64(n), n == float64(int64(n))
	case json.Number:
		i, err := n.Int64()
		return i, err == nil
	}
	return 0, false
}

// backfillPerRescan is how many unchanged files one rescan reads again
// because their stored facts predate the full header reader. Only headers
// are read; the cap keeps a large Library's first rescans light, and the
// rest follow on later rescans.
const backfillPerRescan = 300

// inspectLibraryVideo reads a regular movie/show file only when its real path
// remains inside the Library folder. Existing non-inspection facts, including a
// confirmed edition, are retained. An unchanged file is read again only when
// backfill is set, and a failed read of it (a network share timing out)
// keeps the old facts so a later rescan tries again.
func InspectLibraryVideo(root, relative string, old map[string]any, backfill bool) (map[string]any, bool) {
	rel := filepath.FromSlash(relative)
	if !filepath.IsLocal(rel) {
		return nil, false
	}
	full := filepath.Join(root, rel)
	fi, err := os.Lstat(full)
	if linked, ok := importer.LinkedFile(full); ok {
		fi = linked
	}
	if err != nil || !fi.Mode().IsRegular() || !importer.InsideRealFile(root, full) {
		return nil, false
	}
	size, ok := inspectionSize(old["inspection_size"])
	unchanged := ok && size == fi.Size() && fmt.Sprint(old["inspection_modified"]) == strconv.FormatInt(fi.ModTime().UnixNano(), 10)
	if unchanged && !backfill {
		return old, false
	}
	fresh := importer.InspectFacts(full)
	if unchanged && fresh["inspection_status"] == "error" {
		return old, false
	}
	merged := make(map[string]any, len(old)+10)
	for k, v := range old {
		merged[k] = v
	}
	// Header facts a new read may not give again must not linger.
	for _, k := range importer.HeaderFactKeys {
		delete(merged, k)
	}
	for k, v := range fresh {
		merged[k] = v
	}
	if _, set := merged["edition"]; !set {
		if edition := importer.ParsedEdition(path.Base(relative)); edition != "" {
			merged["edition"], merged["edition_source"] = edition, "name"
		}
	}
	return merged, true
}

func InspectedLanguages(info map[string]any) []string {
	langs, _ := info["audio_languages"].([]string)
	if langs == nil {
		return []string{}
	}
	return append([]string{}, langs...)
}

func (s *Service) updateFileInspection(ctx context.Context, fileID int64, info map[string]any, by domain.Actor) error {
	raw, err := json.Marshal(info)
	if err != nil {
		return err
	}
	languages, err := json.Marshal(InspectedLanguages(info))
	if err != nil {
		return err
	}
	return s.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE files SET media_info = json(?), languages = json(?), updated_at = ?, updated_by = json(?) WHERE id = ?",
			string(raw), string(languages), database.FormatTime(s.now()), ActorText(by), fileID)
		return err
	})
}

// massGoneMin is how many files must be gone at once, and more than half of
// those known, before a rescan takes it for a lost disk rather than deletes.
const massGoneMin = 10

// unreachable says why a Library folder looks like a dropped mount: it is
// gone, or empty while the database knows files in it.
func unreachable(dir string, known int) string {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return "the folder is not there"
	}
	if len(ents) == 0 && known > 0 {
		return "the folder is empty"
	}
	return ""
}

func (s *Service) setFileState(ctx context.Context, f mediafiles.File, to string, by domain.Actor) error {
	if err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE files SET state = ?, checked_at = ?, updated_at = ?, updated_by = ? WHERE id = ?",
			to, database.FormatTime(s.now()), database.FormatTime(s.now()), ActorText(by), f.ID)
		return err
	}); err != nil {
		return err
	}
	var titleID int64
	_ = s.Store.Read().QueryRowContext(ctx, "SELECT title_id FROM versions WHERE id = ?", f.VersionID).Scan(&titleID)
	kind, summary := domain.EventFileMissing, "File not on disk: "+f.RelativePath
	if to == "present" {
		kind, summary = domain.EventFileBack, "File is back: "+f.RelativePath
	}
	if len(f.SlotIDs) > 0 {
		// An episode file names its show and episodes: "Copper Valley
		// S01E02: file not on disk (...)".
		var name, kind string
		_ = s.Store.Read().QueryRowContext(ctx, "SELECT t.name, t.kind FROM titles t JOIN versions v ON v.title_id = t.id WHERE v.id = ? AND t.kind IN ('show', 'artist')",
			f.VersionID).Scan(&name, &kind)
		if name != "" {
			labels := make([]string, 0, len(f.SlotIDs))
			for _, sid := range f.SlotIDs {
				if kind == "artist" {
					// "Lab Band – Old Days: track 03 Song: file not on disk"
					labels = append(labels, "– "+trackLabelOf(ctx, s.Store.Read(), sid))
					continue
				}
				labels = append(labels, EpisodeLabel(ctx, s.Store.Read(), sid))
			}
			what := "file not on disk"
			if to == "present" {
				what = "file is back"
			}
			summary = name + " " + strings.Join(labels, ", ") + ": " + what + " (" + f.RelativePath + ")"
		}
	}
	s.Emit(ctx, &domain.Event{Actor: by, Kind: kind, TitleID: &titleID, Summary: summary,
		Data: map[string]any{"file_id": f.ID, "path": f.RelativePath}, GroupKey: GroupKey(titleID)})
	return nil
}

// recordFound records unknown videos in the movies' own folders as found
// Files: shown, never renamed, moved or deleted.
func (s *Service) recordFound(ctx context.Context, lf mediafiles.LibraryFolder, titleID int64, known map[string]bool, by domain.Actor) (int, error) {
	vs, err := database.Select[mediafiles.Version](ctx, s.Store.Read(), "versions",
		"library_folder_id = ? AND title_id IN (SELECT id FROM titles WHERE state != 'removed') AND (? = 0 OR title_id = ?) ORDER BY id", lf.ID, titleID, titleID)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, v := range vs {
		if v.FolderName == "" || !filepath.IsLocal(v.FolderName) {
			continue
		}
		if lf.Kind == "music" {
			m, err := s.recordFoundTracks(ctx, lf, v, known, by)
			n += m
			if err != nil {
				return n, err
			}
			continue
		}
		if lf.Kind == "show" {
			m, err := s.showParts().RecordFound(ctx, lf, v, known, by)
			n += m
			if err != nil {
				return n, err
			}
			continue
		}
		for _, vid := range listVideos(lf.Path, v.FolderName) {
			if known[strings.ToLower(vid.Path)] {
				continue
			}
			var exists int
			_ = s.Store.Read().QueryRowContext(ctx, "SELECT count(*) FROM files WHERE version_id = ? AND relative_path = ?", v.ID, vid.Path).Scan(&exists)
			if exists > 0 {
				continue
			}
			release := strings.TrimSuffix(path.Base(vid.Path), path.Ext(vid.Path))
			info, _ := InspectLibraryVideo(lf.Path, vid.Path, nil, false)
			if info == nil {
				info = map[string]any{}
			}
			f := &mediafiles.File{SlotIDs: []int64{}, VersionID: v.ID, RelativePath: vid.Path, Size: vid.Size, AddedAt: s.now(), Languages: []string{},
				MediaInfo: info, ReleaseName: &release, Origin: "found", PlacedBy: "none", Role: "main", State: "present"}
			f.Languages = InspectedLanguages(info)
			if err := s.Store.Write(ctx, func(tx *sql.Tx) error { _, err := database.Insert(tx, "files", f, by); return err }); err != nil {
				return n, err
			}
			n++
			known[strings.ToLower(vid.Path)] = true
		}
		if err := s.linkFoundMovie(ctx, v, by); err != nil {
			return n, err
		}
	}
	return n, nil
}

// linkFoundMovie links the one found video in a movie Version's folder to
// its slot when the slot has no file and no running download, as Radarr does
// with a file another app placed: once the file is there the movie is no
// longer wanted. A file that does not fit (edition, audio) stays a note.
func (s *Service) linkFoundMovie(ctx context.Context, v mediafiles.Version, by domain.Actor) error {
	notes, err := database.Select[mediafiles.File](ctx, s.Store.Read(), "files", "version_id = ? AND origin = 'found' AND state = 'present'", v.ID)
	if err != nil {
		return err
	}
	var only []mediafiles.File
	for _, f := range notes {
		if len(f.SlotIDs) == 0 && !f.Protected {
			only = append(only, f)
		}
	}
	if len(only) > 1 {
		return s.markLeftovers(ctx, only, "movie_more_than_one")
	}
	if len(only) != 1 {
		return nil
	}
	_, err = s.adoptMovieCopy(ctx, v.TitleID, v.ID, CopyAdoption{RelativePath: only[0].RelativePath}, true, by)
	if errors.Is(err, ErrCopyChoice) {
		reason := "movie_copy_choice"
		if strings.Contains(err.Error(), "already has a copy") {
			reason = "movie_has_file"
		}
		return s.markLeftovers(ctx, only, reason)
	}
	return err
}

// markLeftovers stores why found files stay notes.
func (s *Service) markLeftovers(ctx context.Context, fs []mediafiles.File, reason string) error {
	return s.Store.Write(ctx, func(tx *sql.Tx) error {
		for _, f := range fs {
			if err := mediafiles.SetLeftoverReason(ctx, tx, f.ID, reason); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Service) handleRescan(ctx context.Context, r *jobs.Run) error {
	res, err := s.Rescan(ctx, JobActor(r))
	r.SetResult(map[string]any{"folders": res.Folders, "unreachable": res.Unreachable, "missing": res.Missing, "back": res.Back, "found": res.Found})
	return err
}
