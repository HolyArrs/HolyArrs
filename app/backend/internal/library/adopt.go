package library

import (
	"context"
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

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
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
// NasSystemDir reports whether name is a folder a NAS or OS keeps for
// itself (thumbnails, recycle bins, snapshots). Folder walks skip them:
// they hold no media of the person's, and reading them wakes disks.
func NasSystemDir(name string) bool {
	switch strings.ToLower(name) {
	case "@eadir", "#recycle", "@recycle", "#snapshot", "lost+found", "$recycle.bin", "system volume information":
		return true
	}
	return false
}

func listVideos(lib, folder string) []AdoptVideo { return ListVideosN(lib, folder, maxScanVideos) }

// listVideosN is listVideos reading at most limit videos.
func ListVideosN(lib, folder string, limit int) []AdoptVideo {
	root := filepath.Join(lib, folder)
	var out []AdoptVideo
	_ = filepath.WalkDir(root, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		if e.IsDir() {
			lower := strings.ToLower(e.Name())
			if p != root && (strings.HasPrefix(e.Name(), ".") || NasSystemDir(e.Name()) || lower == "sample" || lower == "samples" || strings.Count(rel, string(filepath.Separator)) >= maxScanDepth) {
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
	return NameScore(name, year, m.Name, m.Year)
}

// nameScore says how well a found name and year fit a folder's (0..1).
func NameScore(name string, year int, mName string, mYear int) float64 {
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

var ReFolderYear = regexp.MustCompile(`^(.+?)[\s._-]*[(\[]?((?:18|19|20)\d{2})[)\]]?\s*$`)

// folderName reads "Name (Year)" and its usual variants from a folder
// name; a release-style name goes through the release parser.
func FolderName(s string) (string, int) {
	if m := ReFolderYear.FindStringSubmatch(s); m != nil {
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
	name, year := FolderName(strings.TrimSpace(reTMDBTag.ReplaceAllString(folder, "")))
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
func PlanAdopt(ctx context.Context, db Reader, md metadata.Provider, folderID int64) (AdoptPlan, error) {
	return PlanAdoptFrom(ctx, db, md, folderID, "")
}

// PlanAdoptFrom is PlanAdopt for the folders whose names sort after after,
// at most the scan limit of them.
func PlanAdoptFrom(ctx context.Context, db Reader, md metadata.Provider, folderID int64, after string) (AdoptPlan, error) {
	var lf mediafiles.LibraryFolder
	if err := database.Get(ctx, db, "library_folders", folderID, &lf); err != nil {
		return AdoptPlan{}, ErrNoFolder
	}
	if lf.Kind != "movie" {
		return AdoptPlan{}, ErrNotMovieFolder
	}
	ns, err := mediafiles.ReadNaming(ctx, db)
	if err != nil {
		return AdoptPlan{}, err
	}
	p := AdoptPlan{Kind: "movie", LibraryFolderID: lf.ID, Path: lf.Path, Folders: []AdoptCandidate{}, Counts: map[string]int{}, Limit: ns.AdoptScanLimit}
	ents, err := os.ReadDir(lf.Path)
	if err != nil {
		return p, fmt.Errorf("the Library folder cannot be read: %w", err)
	}
	known := map[string]int64{}
	vs, err := database.Select[mediafiles.Version](ctx, db, "versions", "library_folder_id = ? AND title_id IN (SELECT id FROM titles WHERE state != 'removed')", lf.ID)
	if err != nil {
		return p, err
	}
	for _, v := range vs {
		known[strings.ToLower(v.FolderName)] = v.TitleID
	}
	p.Warning = MountWarning(ctx, db, lf, len(ents))
	for _, e := range ents {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") || NasSystemDir(e.Name()) || (after != "" && e.Name() <= after) {
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
	var lf mediafiles.LibraryFolder
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
			r, err := s.showParts().AdoptOne(ctx, lf, c, by)
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

func (s *Service) adoptOne(ctx context.Context, lf mediafiles.LibraryFolder, c AdoptChoice, by domain.Actor) (int64, string, error) {
	if c.Folder == "" || strings.ContainsAny(c.Folder, `/\`) || !filepath.IsLocal(c.Folder) || strings.HasPrefix(c.Folder, ".") {
		return 0, "", errors.New("the folder must be one folder directly in the Library folder")
	}
	if fi, err := os.Lstat(filepath.Join(lf.Path, c.Folder)); err != nil || !fi.IsDir() {
		return 0, "", errors.New("no such folder in the Library folder")
	}
	if taken, err := FolderTaken(ctx, s.Store.Read(), lf, c.Folder); err != nil {
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
		ReleaseGate: c.ReleaseGate, GateDelayDays: c.GateDelayDays}, s.DownloadFolders(ctx), s.now(), by)
	if err != nil {
		return 0, "", err
	}
	s.Emit(ctx, &domain.Event{Actor: by, Kind: domain.EventAdopted, TitleID: &id, Summary: "Adopted " + m.Name + " from " + c.Folder,
		Data: map[string]any{"library_folder_id": lf.ID, "folder": c.Folder, "file": a.File, "tmdb_id": c.TMDB}, GroupKey: GroupKey(id)})
	return id, a.File, nil
}

// ScanFolder queues the dry-run scan of a Library folder; the plan is the
// Job's result.
// after, when set, scans only the folders after it (the rest of a scan
// that reached the limit).
func (s *Service) ScanFolder(ctx context.Context, folderID int64, after string, by domain.Actor) (int64, error) {
	var lf mediafiles.LibraryFolder
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
	var lf mediafiles.LibraryFolder
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
