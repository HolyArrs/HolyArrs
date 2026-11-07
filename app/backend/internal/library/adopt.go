package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
	"github.com/HolyArrs/holyarrs/app/backend/internal/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/naming"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
	"github.com/HolyArrs/holyarrs/app/backend/internal/ratings"
)

// Job kinds for an existing library.
const (
	KindScanLibrary = "scan_library_folder" // plan adopting a Library folder (dry run; the plan is the Job result)
	KindAdopt       = "adopt_library"       // adopt chosen movie folders
	KindRescan      = "rescan_library"      // look at every movie Library folder for missing, returned and found files
)

// Confidence a folder match needs to count as matched; below MatchUnsure
// it is unmatched.
const (
	MatchSure   = 0.9
	MatchUnsure = 0.5
)

// AdoptMatch is one movie or show a folder may be.
type AdoptMatch struct {
	// ID is a show's "<source>:<id>" (tvdb:81189); movies use TMDB.
	ID    string  `json:"id,omitempty"`
	TMDB  string  `json:"tmdb_id"`
	Name  string  `json:"name"`
	Year  int     `json:"year"`
	Score float64 `json:"score"`
}

// AdoptVideo is a video file found in a movie folder.
type AdoptVideo struct {
	Path string `json:"path"` // relative to the Library folder
	Size int64  `json:"size"`
}

// AdoptCandidate is one folder in a Library folder and what it may be.
type AdoptCandidate struct {
	Folder     string       `json:"folder"`
	ParsedName string       `json:"parsed_name"`
	ParsedYear int          `json:"parsed_year,omitempty"`
	Videos     []AdoptVideo `json:"videos"`
	Main       string       `json:"main,omitempty"` // the biggest video, which becomes the File
	Match      *AdoptMatch  `json:"match,omitempty"`
	Others     []AdoptMatch `json:"others"`
	// Status: matched, unsure, unmatched, no_video, in_library, never_add.
	Status  string `json:"status"`
	Why     string `json:"why"`
	TitleID int64  `json:"title_id,omitempty"` // set when in_library
	// A show folder: each video and the episodes its name gives, how many
	// episodes are found and how many files give none.
	Files []AdoptEpisodeFile `json:"files,omitempty"`
	// Cut says Files lists only the first files (music: loose files).
	Cut      bool `json:"cut,omitempty"`
	Episodes int  `json:"episodes,omitempty"`
	Unknown  int  `json:"unknown,omitempty"`
	// An artist folder: each album folder with the album it may be, and how
	// many files hold a track.
	Albums []AdoptAlbum `json:"albums,omitempty"`
	Tracks int          `json:"tracks,omitempty"`
}

// AdoptPlan is the dry run of adopting a Library folder.
type AdoptPlan struct {
	Kind            string           `json:"kind"` // movie, show or music
	LibraryFolderID int64            `json:"library_folder_id"`
	Path            string           `json:"path"`
	Folders         []AdoptCandidate `json:"folders"`
	Counts          map[string]int   `json:"counts"`
	// Limit is the most folders one scan reads. More says folders were
	// left; scanning again from Next (the last folder read) reads the rest.
	Limit int    `json:"limit"`
	More  bool   `json:"more"`
	Next  string `json:"next,omitempty"`
	// Warning says the Library folder looks like a dropped mount.
	Warning string `json:"warning,omitempty"`
	// Partial says the scan stopped (cancelled, or still running) after
	// Next; scanning again from Next reads the rest.
	Partial bool `json:"partial,omitempty"`
}

var reTMDBTag = regexp.MustCompile(`(?i)[\[{(]?tmdb(?:id)?[-_=: ]?(\d+)[\]})]?`)

// Limits on what a scan reads.
const (
	maxScanDepth  = 4
	maxScanVideos = 50
)

// listVideos finds the video files in one movie folder, samples left out,
// biggest first. Symlinks are never followed.
// nasSystemDir reports whether name is a folder a NAS or OS keeps for
// itself (thumbnails, recycle bins, snapshots). Folder walks skip them:
// they hold no media of the person's, and reading them wakes disks.
func nasSystemDir(name string) bool {
	switch strings.ToLower(name) {
	case "@eadir", "#recycle", "@recycle", "#snapshot", "lost+found", "$recycle.bin", "system volume information":
		return true
	}
	return false
}

func listVideos(lib, folder string) []AdoptVideo { return listVideosN(lib, folder, maxScanVideos) }

// listVideosN is listVideos reading at most limit videos.
func listVideosN(lib, folder string, limit int) []AdoptVideo {
	root := filepath.Join(lib, folder)
	var out []AdoptVideo
	_ = filepath.WalkDir(root, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		if e.IsDir() {
			lower := strings.ToLower(e.Name())
			if p != root && (strings.HasPrefix(e.Name(), ".") || nasSystemDir(e.Name()) || lower == "sample" || lower == "samples" || strings.Count(rel, string(filepath.Separator)) >= maxScanDepth) {
				return filepath.SkipDir
			}
			return nil
		}
		if e.Type()&fs.ModeSymlink != 0 || !e.Type().IsRegular() || !videoExt[strings.ToLower(filepath.Ext(p))] || len(out) >= limit {
			return nil
		}
		base := strings.ToLower(strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())))
		for _, w := range strings.FieldsFunc(base, func(r rune) bool { return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9') }) {
			if w == "sample" {
				return nil
			}
		}
		fi, err := e.Info()
		if err != nil {
			return nil
		}
		out = append(out, AdoptVideo{Path: path.Join(folder, filepath.ToSlash(rel)), Size: fi.Size()})
		return nil
	})
	sort.SliceStable(out, func(i, j int) bool { return out[i].Size > out[j].Size })
	return out
}

// score says how well a movie fits a folder's parsed name and year (0..1).
func score(name string, year int, m metadata.Movie) float64 {
	return nameScore(name, year, m.Name, m.Year)
}

// nameScore says how well a found name and year fit a folder's (0..1).
func nameScore(name string, year int, mName string, mYear int) float64 {
	s := wordScore(name, mName)
	switch {
	case year == 0:
		s *= 0.85
	case mYear == year:
	case mYear == year-1 || mYear == year+1:
		s *= 0.9
	default:
		s *= 0.6
	}
	return math.Round(s*100) / 100
}

// wordScore says how alike two names are by their words (0..1): 1 when
// they clean to the same words, else the share of words both hold.
func wordScore(name, other string) float64 {
	a, b := releasename.CleanName(name), releasename.CleanName(other)
	switch {
	case a == "" || b == "":
		return 0
	case a == b:
		return 1
	}
	wa, wb := strings.Fields(a), strings.Fields(b)
	set := map[string]bool{}
	for _, w := range wa {
		set[w] = true
	}
	both := 0
	for _, w := range wb {
		if set[w] {
			both++
		}
	}
	return float64(both) / float64(max(len(wa), len(wb)))
}

var reFolderYear = regexp.MustCompile(`^(.+?)[\s._-]*[(\[]?((?:18|19|20)\d{2})[)\]]?\s*$`)

// folderName reads "Name (Year)" and its usual variants from a folder
// name; a release-style name goes through the release parser.
func folderName(s string) (string, int) {
	if m := reFolderYear.FindStringSubmatch(s); m != nil {
		y, _ := strconv.Atoi(m[2])
		return strings.Trim(m[1], " -._(["), y
	}
	p := releasename.Parse(s, releasename.Hints{})
	if p.Title.Value != "" && p.Year.Value != 0 {
		return p.Title.Value, p.Year.Value
	}
	return strings.Trim(s, " -._"), 0
}

// matchFolder looks a folder up at the metadata source.
func matchFolder(ctx context.Context, md metadata.Provider, folder string) (string, int, []AdoptMatch, error) {
	if m := reTMDBTag.FindStringSubmatch(folder); m != nil {
		mv, err := md.Movie(ctx, "tmdb:"+m[1])
		if err == nil {
			return mv.Name, mv.Year, []AdoptMatch{{TMDB: mv.IDs["tmdb"], Name: mv.Name, Year: mv.Year, Score: 1}}, nil
		}
		if !errors.Is(err, metadata.ErrNotFound) {
			return "", 0, nil, err
		}
	}
	name, year := folderName(strings.TrimSpace(reTMDBTag.ReplaceAllString(folder, "")))
	ms, err := md.Search(ctx, name, year)
	if err == nil && len(ms) == 0 && year != 0 {
		ms, err = md.Search(ctx, name, 0)
	}
	if err != nil {
		return name, year, nil, err
	}
	var out []AdoptMatch
	for _, m := range ms {
		if m.IDs["tmdb"] == "" {
			continue
		}
		out = append(out, AdoptMatch{TMDB: m.IDs["tmdb"], Name: m.Name, Year: m.Year, Score: score(name, year, m)})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if len(out) > 4 {
		out = out[:4]
	}
	return name, year, out, nil
}

// PlanAdopt scans a Library folder: each folder in it is matched to a
// movie with a confidence score. Nothing is written anywhere.
func PlanAdopt(ctx context.Context, db reader, md metadata.Provider, folderID int64) (AdoptPlan, error) {
	return PlanAdoptFrom(ctx, db, md, folderID, "")
}

// PlanAdoptFrom is PlanAdopt for the folders whose names sort after after,
// at most the scan limit of them.
func PlanAdoptFrom(ctx context.Context, db reader, md metadata.Provider, folderID int64, after string) (AdoptPlan, error) {
	var lf domain.LibraryFolder
	if err := database.Get(ctx, db, "library_folders", folderID, &lf); err != nil {
		return AdoptPlan{}, ErrNoFolder
	}
	if lf.Kind != "movie" {
		return AdoptPlan{}, ErrNotMovieFolder
	}
	ns, err := naming.Read(ctx, db)
	if err != nil {
		return AdoptPlan{}, err
	}
	p := AdoptPlan{Kind: "movie", LibraryFolderID: lf.ID, Path: lf.Path, Folders: []AdoptCandidate{}, Counts: map[string]int{}, Limit: ns.AdoptScanLimit}
	ents, err := os.ReadDir(lf.Path)
	if err != nil {
		return p, fmt.Errorf("the Library folder cannot be read: %w", err)
	}
	known := map[string]int64{}
	vs, err := database.Select[domain.Version](ctx, db, "versions", "library_folder_id = ? AND title_id IN (SELECT id FROM titles WHERE state != 'removed')", lf.ID)
	if err != nil {
		return p, err
	}
	for _, v := range vs {
		known[strings.ToLower(v.FolderName)] = v.TitleID
	}
	p.Warning = mountWarning(ctx, db, lf, len(ents))
	for _, e := range ents {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") || nasSystemDir(e.Name()) || (after != "" && e.Name() <= after) {
			continue
		}
		if len(p.Folders) >= p.Limit {
			p.More, p.Next = true, p.Folders[len(p.Folders)-1].Folder
			break
		}
		c := AdoptCandidate{Folder: e.Name(), Videos: listVideos(lf.Path, e.Name()), Others: []AdoptMatch{}}
		if len(c.Videos) > 0 {
			c.Main = c.Videos[0].Path
		}
		if id, ok := known[strings.ToLower(e.Name())]; ok {
			c.Status, c.Why, c.TitleID = "in_library", "this folder already belongs to a movie in the Library", id
			p.Folders = append(p.Folders, c)
			p.Counts[c.Status]++
			continue
		}
		name, year, ms, err := matchFolder(ctx, md, e.Name())
		if err != nil {
			if ctx.Err() != nil {
				return p, ctx.Err()
			}
			// One failed lookup does not stop the scan: this folder is
			// unmatched with the reason and can be looked up by hand.
			c.ParsedName, c.ParsedYear = name, year
			c.Status, c.Why = "unmatched", "the TMDB lookup failed ("+err.Error()+"); look it up by hand or scan again later"
			p.Folders = append(p.Folders, c)
			p.Counts[c.Status]++
			continue
		}
		c.ParsedName, c.ParsedYear = name, year
		if len(ms) > 0 && ms[0].Score >= MatchUnsure {
			c.Match = &ms[0]
			c.Others = append(c.Others, ms[1:]...)
		} else {
			c.Others = append(c.Others, ms...)
		}
		switch {
		case c.Match == nil:
			c.Status, c.Why = "unmatched", "no movie at TMDB fits this folder name; pick one by hand"
		default:
			id, err := TitleByTMDB(ctx, db, c.Match.TMDB)
			if err != nil {
				return p, err
			}
			never, err := OnNeverAdd(ctx, db, c.Match.TMDB)
			if err != nil {
				return p, err
			}
			close := len(ms) > 1 && ms[1].Score > c.Match.Score-0.1
			switch {
			case id != 0:
				c.Status, c.Why, c.TitleID = "in_library", "this movie is already in the Library in another folder", id
			case never:
				c.Status, c.Why = "never_add", "this movie is on the Never-add list"
			case len(c.Videos) == 0:
				c.Status, c.Why = "no_video", "no video file in the folder; adopting adds the movie as missing"
			case c.Match.Score < MatchSure || close:
				c.Status, c.Why = "unsure", fmt.Sprintf("best match %s (%d) at %.0f%%; check it", c.Match.Name, c.Match.Year, c.Match.Score*100)
			default:
				c.Status, c.Why = "matched", fmt.Sprintf("%s (%d) at %.0f%%", c.Match.Name, c.Match.Year, c.Match.Score*100)
			}
		}
		p.Folders = append(p.Folders, c)
		p.Counts[c.Status]++
	}
	return p, nil
}

// AdoptChoice is one folder to adopt as one movie.
type AdoptChoice struct {
	Folder string `json:"folder"`
	TMDB   string `json:"tmdb_id,omitempty"`
	// ID is the show to adopt a show folder as: "<source>:<id>".
	ID string `json:"id,omitempty"`
	// ReleaseGate and GateDelayDays set a movie's Release gate ("" and
	// nil: this server's defaults); the move brings them from Radarr.
	ReleaseGate   string `json:"release_gate,omitempty"`
	GateDelayDays *int   `json:"gate_delay_days,omitempty"`
}

// AdoptResult says what happened to one choice.
type AdoptResult struct {
	Folder  string `json:"folder"`
	TitleID int64  `json:"title_id,omitempty"`
	File    string `json:"file,omitempty"`
	Error   string `json:"error,omitempty"`
	// A show folder: the episodes its files filled and the files left alone.
	Episodes int      `json:"episodes,omitempty"`
	Unknown  []string `json:"unknown,omitempty"`
}

// Adopt adds each chosen folder as a movie with its folder as it is; its
// biggest video becomes an adopted File. No file is moved, renamed or
// written. One bad choice does not stop the others.
func (s *Service) Adopt(ctx context.Context, folderID int64, choices []AdoptChoice, by domain.Actor) ([]AdoptResult, error) {
	var lf domain.LibraryFolder
	if err := database.Get(ctx, s.Store.Read(), "library_folders", folderID, &lf); err != nil {
		return nil, ErrNoFolder
	}
	out := make([]AdoptResult, 0, len(choices))
	for _, c := range choices {
		if lf.Kind == "music" {
			r, err := s.adoptArtistOne(ctx, lf, c, by)
			if err != nil {
				r.Error = err.Error()
			}
			out = append(out, r)
			continue
		}
		if lf.Kind == "show" {
			r, err := s.adoptShowOne(ctx, lf, c, by)
			if err != nil {
				r.Error = err.Error()
			}
			out = append(out, r)
			continue
		}
		r := AdoptResult{Folder: c.Folder}
		id, file, err := s.adoptOne(ctx, lf, c, by)
		r.TitleID, r.File = id, file
		if err != nil {
			r.Error = err.Error()
		}
		out = append(out, r)
	}
	return out, nil
}

func (s *Service) adoptOne(ctx context.Context, lf domain.LibraryFolder, c AdoptChoice, by domain.Actor) (int64, string, error) {
	if c.Folder == "" || strings.ContainsAny(c.Folder, `/\`) || !filepath.IsLocal(c.Folder) || strings.HasPrefix(c.Folder, ".") {
		return 0, "", errors.New("the folder must be one folder directly in the Library folder")
	}
	if fi, err := os.Lstat(filepath.Join(lf.Path, c.Folder)); err != nil || !fi.IsDir() {
		return 0, "", errors.New("no such folder in the Library folder")
	}
	if taken, err := folderTaken(ctx, s.Store.Read(), lf, c.Folder); err != nil {
		return 0, "", err
	} else if taken {
		return 0, "", errors.New("this folder already belongs to a movie in the Library")
	}
	if s.Metadata == nil {
		return 0, "", errors.New("no metadata source")
	}
	m, err := s.Metadata.Movie(ctx, "tmdb:"+c.TMDB)
	if err != nil {
		return 0, "", fmt.Errorf("TMDB %s: %w", c.TMDB, err)
	}
	a := &Adoption{FolderName: c.Folder}
	if vids := listVideos(lf.Path, c.Folder); len(vids) > 0 {
		a.File, a.Size = vids[0].Path, vids[0].Size
	}
	id, err := AddMovie(ctx, s.Store, m, AddOptions{LibraryFolderID: lf.ID, WhyAdded: "import", Adopt: a,
		ReleaseGate: c.ReleaseGate, GateDelayDays: c.GateDelayDays}, s.downloadPaths(ctx), s.now(), by)
	if err != nil {
		return 0, "", err
	}
	s.event(ctx, &domain.Event{Actor: by, Kind: domain.EventAdopted, TitleID: &id, Summary: "Adopted " + m.Name + " from " + c.Folder,
		Data: map[string]any{"library_folder_id": lf.ID, "folder": c.Folder, "file": a.File, "tmdb_id": c.TMDB}, GroupKey: groupKey(id)})
	return id, a.File, nil
}

// ScanFolder queues the dry-run scan of a Library folder; the plan is the
// Job's result.
// after, when set, scans only the folders after it (the rest of a scan
// that reached the limit).
func (s *Service) ScanFolder(ctx context.Context, folderID int64, after string, by domain.Actor) (int64, error) {
	var lf domain.LibraryFolder
	if err := database.Get(ctx, s.Store.Read(), "library_folders", folderID, &lf); err != nil {
		return 0, ErrNoFolder
	}
	if lf.Kind != "movie" && lf.Kind != "show" && lf.Kind != "music" {
		return 0, ErrNotMovieFolder
	}
	id, _, err := s.Queue.Enqueue(ctx, KindScanLibrary, &domain.Subject{Type: "library_folder", ID: folderID}, map[string]any{"after": after},
		KindScanLibrary+":"+strconv.FormatInt(folderID, 10)+":"+after, by)
	return id, err
}

// AdoptLater queues adopting the chosen folders.
// With follow, the person who started it follows every title brought in.
func (s *Service) AdoptLater(ctx context.Context, folderID int64, choices []AdoptChoice, follow bool, by domain.Actor) (int64, error) {
	var lf domain.LibraryFolder
	if err := database.Get(ctx, s.Store.Read(), "library_folders", folderID, &lf); err != nil {
		return 0, ErrNoFolder
	}
	raw := make([]any, 0, len(choices))
	for _, c := range choices {
		raw = append(raw, map[string]any{"folder": c.Folder, "tmdb_id": c.TMDB, "id": c.ID})
	}
	id, _, err := s.Queue.Enqueue(ctx, KindAdopt, &domain.Subject{Type: "library_folder", ID: folderID}, map[string]any{"choices": raw, "follow": follow}, "", by)
	return id, err
}

func (s *Service) handleScanLibrary(ctx context.Context, r *jobs.Run) error {
	if r.Job.Subject == nil || r.Job.Subject.Type != "library_folder" {
		return errors.New("scan_library_folder needs a library_folder subject")
	}
	after, _ := r.Job.Params["after"].(string)
	var lf domain.LibraryFolder
	if err := database.Get(ctx, s.Store.Read(), "library_folders", r.Job.Subject.ID, &lf); err != nil {
		return ErrNoFolder
	}
	var p AdoptPlan
	var err error
	switch lf.Kind {
	case "show":
		p, err = PlanShowAdoptFrom(ctx, s.Store.Read(), s.Shows, lf.ID, after)
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
	s.event(ctx, &domain.Event{Actor: jobActor(r), Kind: domain.EventLibraryScanned, JobID: &r.Job.ID,
		Summary: fmt.Sprintf("Scanned %s: %d matched, %d unsure, %d unmatched", p.Path, p.Counts["matched"], p.Counts["unsure"], p.Counts["unmatched"]),
		Data:    map[string]any{"library_folder_id": p.LibraryFolderID, "counts": p.Counts}})
	return nil
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
	res, err := s.Adopt(ctx, r.Job.Subject.ID, cs, jobActor(r))
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
				if err := tx.QueryRow("SELECT kind, coalesce(rating, '') FROM titles WHERE id = ?", x.TitleID).Scan(&kind, &rating); err != nil || !ratings.Allows(max, kind, rating) {
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
	lfs, err := database.Select[domain.LibraryFolder](ctx, db, "library_folders", "kind IN ('movie', 'show', 'music') ORDER BY id")
	if err != nil {
		return res, err
	}
	for _, lf := range lfs {
		res.Folders++
		files, err := database.Select[domain.File](ctx, db, "files", `state IN ('present', 'missing') AND version_id IN
			(SELECT v.id FROM versions v JOIN titles t ON t.id = v.title_id WHERE v.library_folder_id = ? AND t.state != 'removed') ORDER BY id`, lf.ID)
		if err != nil {
			return res, err
		}
		if why := unreachable(lf.Path, len(files)); why != "" {
			res.Unreachable++
			s.event(ctx, &domain.Event{Actor: by, Kind: domain.EventFolderUnreachable,
				Summary: "Library folder " + lf.Label + " cannot be reached (" + why + "); no file was marked missing",
				Data:    map[string]any{"library_folder_id": lf.ID, "path": lf.Path, "reason": why}})
			continue
		}
		knownPaths := map[string]bool{}
		type change struct {
			f  domain.File
			to string
		}
		var changes []change
		present, gone := 0, 0
		for _, f := range files {
			knownPaths[strings.ToLower(f.RelativePath)] = true
			_, statErr := os.Stat(filepath.Join(lf.Path, filepath.FromSlash(f.RelativePath)))
			if lf.Kind != "music" && f.Role == "main" && statErr == nil {
				backfill := backfillLeft > 0 && importer.NeedsFacts(f.MediaInfo)
				if info, changed := inspectLibraryVideo(lf.Path, f.RelativePath, f.MediaInfo, backfill); changed {
					if err := s.updateFileInspection(ctx, f.ID, info, by); err != nil {
						return res, err
					}
					if backfill {
						backfillLeft--
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
			s.event(ctx, &domain.Event{Actor: by, Kind: domain.EventFolderUnreachable,
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
				return res, err
			}
		}
		n, err := s.recordFound(ctx, lf, knownPaths, by)
		if err != nil {
			return res, err
		}
		res.Found += n
	}
	s.event(ctx, &domain.Event{Actor: by, Kind: domain.EventLibraryRescanned,
		Summary: fmt.Sprintf("Rescanned %d Library folders: %d missing, %d back, %d found, %d not reachable", res.Folders, res.Missing, res.Back, res.Found, res.Unreachable),
		Data:    map[string]any{"folders": res.Folders, "missing": res.Missing, "back": res.Back, "found": res.Found, "unreachable": res.Unreachable}})
	return res, nil
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
func inspectLibraryVideo(root, relative string, old map[string]any, backfill bool) (map[string]any, bool) {
	rel := filepath.FromSlash(relative)
	if !filepath.IsLocal(rel) {
		return nil, false
	}
	full := filepath.Join(root, rel)
	fi, err := os.Lstat(full)
	if err != nil || !fi.Mode().IsRegular() || !importer.InsideReal(root, full) {
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

func inspectedLanguages(info map[string]any) []string {
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
	languages, err := json.Marshal(inspectedLanguages(info))
	if err != nil {
		return err
	}
	return s.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE files SET media_info = json(?), languages = json(?), updated_at = ?, updated_by = json(?) WHERE id = ?",
			string(raw), string(languages), database.FormatTime(s.now()), actorText(by), fileID)
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

func (s *Service) setFileState(ctx context.Context, f domain.File, to string, by domain.Actor) error {
	if err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE files SET state = ?, checked_at = ?, updated_at = ?, updated_by = ? WHERE id = ?",
			to, database.FormatTime(s.now()), database.FormatTime(s.now()), actorText(by), f.ID)
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
				labels = append(labels, episodeLabel(ctx, s.Store.Read(), sid))
			}
			what := "file not on disk"
			if to == "present" {
				what = "file is back"
			}
			summary = name + " " + strings.Join(labels, ", ") + ": " + what + " (" + f.RelativePath + ")"
		}
	}
	s.event(ctx, &domain.Event{Actor: by, Kind: kind, TitleID: &titleID, Summary: summary,
		Data: map[string]any{"file_id": f.ID, "path": f.RelativePath}, GroupKey: groupKey(titleID)})
	return nil
}

// recordFound records unknown videos in the movies' own folders as found
// Files: shown, never renamed, moved or deleted.
func (s *Service) recordFound(ctx context.Context, lf domain.LibraryFolder, known map[string]bool, by domain.Actor) (int, error) {
	vs, err := database.Select[domain.Version](ctx, s.Store.Read(), "versions",
		"library_folder_id = ? AND title_id IN (SELECT id FROM titles WHERE state != 'removed') ORDER BY id", lf.ID)
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
			m, err := s.recordFoundEpisodes(ctx, lf, v, known, by)
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
			info, _ := inspectLibraryVideo(lf.Path, vid.Path, nil, false)
			if info == nil {
				info = map[string]any{}
			}
			f := &domain.File{SlotIDs: []int64{}, VersionID: v.ID, RelativePath: vid.Path, Size: vid.Size, AddedAt: s.now(), Languages: []string{},
				MediaInfo: info, ReleaseName: &release, Origin: "found", PlacedBy: "none", Role: "main", State: "present"}
			f.Languages = inspectedLanguages(info)
			if err := s.Store.Write(ctx, func(tx *sql.Tx) error { _, err := database.Insert(tx, "files", f, by); return err }); err != nil {
				return n, err
			}
			n++
			known[strings.ToLower(vid.Path)] = true
		}
	}
	return n, nil
}

func (s *Service) handleRescan(ctx context.Context, r *jobs.Run) error {
	res, err := s.Rescan(ctx, jobActor(r))
	r.SetResult(map[string]any{"folders": res.Folders, "unreachable": res.Unreachable, "missing": res.Missing, "back": res.Back, "found": res.Found})
	return err
}
