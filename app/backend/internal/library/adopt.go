package library

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
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
	// Poster is the art the lookup already returned; no extra call.
	Poster string `json:"poster,omitempty"`
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
	// Loose says Folder is a video file directly in the Library folder, a
	// movie kept without a folder of its own; Videos are its cuts.
	Loose bool `json:"loose,omitempty"`
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
	// Total is how many folders the Library folder holds in all; Offset is
	// how many of them sort before this part.
	Total  int `json:"total"`
	Offset int `json:"offset"`
	// Warning says the Library folder looks like a dropped mount.
	Warning string `json:"warning,omitempty"`
	// Partial says the scan stopped (cancelled, or still running) after
	// Next; scanning again from Next reads the rest.
	Partial bool `json:"partial,omitempty"`
	// LooseMovies is how many movies sit as loose files (in a genre
	// folder or the Library folder itself) and were left out; LooseFolders
	// names the folders that hold them ("" is the Library folder). Tidy my
	// folders gives each a folder of its own.
	LooseMovies  int      `json:"loose_movies,omitempty"`
	LooseFolders []string `json:"loose_folders,omitempty"`
	// Flat says the Library folder keeps its movies as files directly in
	// it. SuggestFlat says it is not set so but looks that way.
	Flat bool `json:"flat,omitempty"`
	// LooseTop is how many movies sit as loose files directly in a folder
	// that is not flat: offered as they are, Tidy my folders optional.
	LooseTop    int  `json:"loose_top,omitempty"`
	SuggestFlat bool `json:"suggest_flat,omitempty"`
}

// reTMDBTag reads the id tags Radarr reads in folder names: {tmdb-123},
// [tmdbid-123], {imdb-tt0113277} and [imdbid-tt0113277].
var reTMDBTag = regexp.MustCompile(`(?i)[\[{(]?(tmdb|imdb)(?:id)?[-_=: ]?(tt\d+|\d+)[\]})]?`)

// TagID reads the first id tag of a movie or show folder name as
// "<source>:<id>"; an IMDb number without "tt" gets it.
func TagID(m []string) string {
	kind, v := strings.ToLower(m[1]), strings.ToLower(m[2])
	if kind == "imdb" {
		return "imdb:tt" + strings.TrimPrefix(v, "tt")
	}
	if strings.HasPrefix(v, "tt") {
		return ""
	}
	return kind + ":" + v
}

// Limits on what a scan reads.
const (
	maxScanDepth  = 4
	maxScanVideos = 50
)

// listVideos finds the video files in one movie folder, samples left out,
// biggest first. Symlinks are followed only to plain files, and only when importer.FollowLinks is on.
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
	return listVideosWhere(lib, folder, limit, nil)
}

// listVideosWhere is ListVideosN keeping only the videos keep accepts, by
// their path inside the folder with forward slashes; nil keeps all. Only
// kept videos count toward limit.
func listVideosWhere(lib, folder string, limit int, keep func(rel string) bool) []AdoptVideo {
	root := filepath.Join(lib, folder)
	// Never nil: the list goes to the page as JSON, and a folder with no
	// video must read as an empty list there, not as null.
	out := []AdoptVideo{}
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
		linked, isLink := importer.LinkedFile(p)
		if (e.Type()&fs.ModeSymlink != 0 && !isLink) || (!isLink && !e.Type().IsRegular()) || !videoExt[strings.ToLower(filepath.Ext(p))] || len(out) >= limit {
			return nil
		}
		base := strings.ToLower(strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())))
		for _, w := range strings.FieldsFunc(base, func(r rune) bool { return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9') }) {
			if w == "sample" {
				return nil
			}
		}
		if keep != nil && !keep(filepath.ToSlash(rel)) {
			return nil
		}
		fi, err := e.Info()
		if isLink {
			fi, err = linked, nil
		}
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
	case a == b || releasename.SameName(other, a):
		return 1
	}
	wa, wb := strings.Fields(a), strings.Fields(b)
	// Count each word as often as it appears, so "Lisa Lisa" shares one
	// word with "Lisa Knapp", not two.
	left := map[string]int{}
	for _, w := range wa {
		left[w]++
	}
	both := 0
	for _, w := range wb {
		if left[w] > 0 {
			left[w]--
			both++
		}
	}
	return float64(both) / float64(max(len(wa), len(wb)))
}

var ReFolderYear = regexp.MustCompile(`^(.+?)[\s._-]*[(\[]?((?:18|19|20)\d{2})[)\]]?\s*$`)

// matchFolder looks a folder up at the metadata source.
func matchFolder(ctx context.Context, md metadata.Provider, folder string) (string, int, []AdoptMatch, error) {
	if m := reTMDBTag.FindStringSubmatch(folder); m != nil {
		mv, err := metadata.Movie{}, metadata.ErrNotFound
		if id := TagID(m); id != "" {
			mv, err = md.Movie(ctx, id)
		}
		if err == nil && mv.IDs["tmdb"] != "" {
			return mv.Name, mv.Year, []AdoptMatch{{TMDB: mv.IDs["tmdb"], Name: mv.Name, Year: mv.Year, Score: 1, Poster: mv.Poster}}, nil
		}
		if err != nil && !errors.Is(err, metadata.ErrNotFound) {
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
		out = append(out, AdoptMatch{TMDB: m.IDs["tmdb"], Name: m.Name, Year: m.Year, Score: score(name, year, m), Poster: m.Poster})
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
	fold := FoldsCase(lf.Path)
	shared := map[string]bool{}
	for _, v := range vs {
		switch {
		case v.SharedFolder:
			shared[v.FolderName] = true
		case v.FolderName != "":
			known[FolderKey(v.FolderName, fold)] = v.TitleID
		}
	}
	ownedFiles, err := mediafiles.AdoptedFileOwners(ctx, db, lf.ID, fold, "")
	if err != nil {
		return p, err
	}
	for rel, owner := range ownedFiles {
		known[rel] = owner.TitleID
	}
	// Loose videos already in the Library, by their path.
	rootKnown, err := looseKnown(ctx, db, lf.ID, fold)
	if err != nil {
		return p, err
	}
	p.Warning = MountWarning(ctx, db, lf, len(ents))
	p.Flat = lf.Flat
	// Movie folders may sit in letter, genre or collection folders.
	all, loose := movieCandidatesLoose(lf.Path, ents, shared)
	looseAt := looseGroups(lf.Path, ents)
	if after == "" {
		p.SuggestFlat = !lf.Flat && SuggestFlat(len(looseAt), len(all))
	}
	for name := range looseAt {
		all = append(all, name)
	}
	sort.Strings(all)
	// Loose files directly in the Library folder are offered as they are.
	// Not in a flat folder, Tidy my folders is offered for them too, as an
	// option; loose files in a genre folder are still left out.
	if after == "" {
		for name, vids := range looseAt {
			if _, ok := looseOwner(rootKnown, AdoptCandidate{Videos: vids}, fold); !ok && !lf.Flat {
				if _, year := FolderName(scanName(name)); year != 0 {
					p.LooseTop++
				}
			}
		}
	}
	inGroups := slices.DeleteFunc(slices.Clone(loose), func(f string) bool { return f == "" })
	if len(inGroups) > 0 && after == "" {
		p.LooseMovies = LooseMovieCount(lf.Path, inGroups)
	}
	if (p.LooseMovies > 0 || p.LooseTop > 0) && after == "" {
		p.LooseFolders = loose
		if lf.Flat {
			p.LooseFolders = inGroups
		}
	}
	todo := afterIn(all, after)
	p.Offset, p.Total = len(all)-len(todo), len(all)
	total := min(len(todo), p.Limit)
	defer func() { ReportScan(ctx, len(p.Folders), total) }()
	ahead := startAhead(ctx, md, scanLookups(todo, p.Limit, known, fold))
	defer ahead.close()
	evidenceBudget := scanEvidenceRequests
	for _, name := range todo {
		if len(p.Folders) >= p.Limit {
			p.More, p.Next = true, p.Folders[len(p.Folders)-1].Folder
			break
		}
		ReportScan(ctx, len(p.Folders), total)
		c := AdoptCandidate{Folder: name, Others: []AdoptMatch{}}
		vids, isLoose := looseAt[name]
		if isLoose {
			c.Videos, c.Loose = vids, true
		} else {
			c.Videos = candidateVideos(lf.Path, name)
		}
		if v, ok := mainCandidateVideo(lf.Path, name, c.Videos, isLoose); ok {
			c.Main = v.Path
		}
		if id, ok := looseOwner(rootKnown, c, fold); isLoose && ok {
			c.Status, c.Why, c.TitleID = "in_library", "this file already belongs to a movie in the Library", id
			p.Folders = append(p.Folders, c)
			p.Counts[c.Status]++
			continue
		}
		if id, ok := known[FolderKey(name, fold)]; ok && !isLoose {
			c.Status, c.Why, c.TitleID = "in_library", "this folder already belongs to a movie in the Library", id
			p.Folders = append(p.Folders, c)
			p.Counts[c.Status]++
			continue
		}
		if collectionName(path.Base(name)) && len(c.Videos) == 0 {
			c.ParsedName = path.Base(name)
			c.Status, c.Why = "no_video", "No movie video found in this collection; samples and subtitles do not count."
			p.Folders = append(p.Folders, c)
			p.Counts[c.Status]++
			continue
		}
		if possibleCollectionPack(name, c.Videos) {
			c.Main = ""
			c.ParsedName = path.Base(name)
			c.Status, c.Why = "unmatched", "Possible collection pack: review its contents and import individual movie files."
			p.Folders = append(p.Folders, c)
			p.Counts[c.Status]++
			continue
		}
		ahead.wait(ctx, name)
		name, year, ms, err := matchCandidateWithBudget(ctx, md, name, &evidenceBudget)
		if err != nil {
			if ctx.Err() != nil {
				return p, ctx.Err()
			}
			// One failed lookup does not stop the scan: this folder is
			// unmatched with the reason and can be looked up by hand.
			c.ParsedName, c.ParsedYear = name, year
			c.Status, c.Why = "unmatched", "the TMDB lookup failed ("+err.Error()+"); look it up by hand or scan again later"
			if MetadataDown(err) {
				c.Status, c.Why, p.Warning = NotLookedUp, MetadataNotAnswering, cmp.Or(p.Warning, MetadataNotAnsweringScan)
			}
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
			case c.Main == "": // no video, or only extras
				c.Status, c.Why = "no_video", "no video file in the folder; adopting adds the movie as missing"
			case close && c.Match.Score >= MatchSure:
				// The name fits; what is unsure is which of two titles it is, so no percent is shown as if that were the doubt.
				c.Status, c.Why = "unsure", fmt.Sprintf("%s (%d) and %s (%d) both fit this folder name; check which one it is", c.Match.Name, c.Match.Year, ms[1].Name, ms[1].Year)
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

// ScanWants says whether a scan reads the Library folder entry e: a
// folder, not hidden or kept by a NAS, sorting after after.
func ScanWants(e os.DirEntry, after string) bool {
	return e.IsDir() && !strings.HasPrefix(e.Name(), ".") && !NasSystemDir(e.Name()) && (after == "" || e.Name() > after)
}

// ScanTotal is how many folders one scan reads: those it wants, at most limit.
func ScanTotal(ents []os.DirEntry, after string, limit int) int {
	n := 0
	for _, e := range ents {
		if ScanWants(e, after) {
			n++
		}
	}
	return min(n, limit)
}

// ScanPlace is how many folders a scan from the start reads in all, and
// how many of them sort at or before after.
func ScanPlace(ents []os.DirEntry, after string) (before, total int) {
	for _, e := range ents {
		if ScanWants(e, "") {
			total++
			if after != "" && e.Name() <= after {
				before++
			}
		}
	}
	return before, total
}

type scanProgressKey struct{}

// WithScanProgress makes the folder scans run under ctx tell f how many
// folders are read so far out of how many, so a long scan shows progress.
func WithScanProgress(ctx context.Context, f func(done, total int)) context.Context {
	return context.WithValue(ctx, scanProgressKey{}, f)
}

// ReportScan tells the scan's progress hook, if any, how far it is.
func ReportScan(ctx context.Context, done, total int) {
	if f, ok := ctx.Value(scanProgressKey{}).(func(done, total int)); ok {
		f(done, total)
	}
}

// AdoptChoice is a movie folder or an individual video in a collection.
type AdoptChoice struct {
	Folder string `json:"folder"`
	TMDB   string `json:"tmdb_id,omitempty"`
	// ID is the show to adopt a show folder as: "<source>:<id>".
	ID string `json:"id,omitempty"`
	// ReleaseGate and GateDelayDays set a movie's Release gate ("" and
	// nil: this server's defaults); the move brings them from Radarr.
	ReleaseGate   string `json:"release_gate,omitempty"`
	GateDelayDays *int   `json:"gate_delay_days,omitempty"`
	// SeasonsDiffer leaves every file of a show folder for the person to
	// place: the old app numbers the show's seasons differently.
	SeasonsDiffer bool `json:"-"`
	// File names the movie's video in Folder that an old app recorded, by
	// its path there with forward slashes (RecordedMovieVideo); it becomes
	// the File instead of the biggest video when it is there. Only the move
	// from other apps sets it.
	File string `json:"-"`
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

// Adopt adds each chosen folder or collection video as a movie in place.
// A folder's biggest video becomes its File. No file is moved, renamed or
// written. One bad choice does not stop the others.
func (s *Service) Adopt(ctx context.Context, folderID int64, choices []AdoptChoice, by domain.Actor) ([]AdoptResult, error) {
	var lf mediafiles.LibraryFolder
	if err := database.Get(ctx, s.Store.Read(), "library_folders", folderID, &lf); err != nil {
		return nil, ErrNoFolder
	}
	// One show can carry hundreds of episodes in one save.
	ctx = database.WithLongWrite(ctx)
	out := make([]AdoptResult, len(choices))
	errs := make([]error, len(choices))
	defer func() { ReportScan(ctx, len(out), len(choices)) }()
	for i, c := range choices {
		ReportScan(ctx, i, len(choices))
		out[i], errs[i] = s.adoptChoice(ctx, lf, c, by)
	}
	s.retryWaiting(ctx, lf, choices, out, errs, by)
	for i, err := range errs {
		if err != nil {
			s.adoptFailed(ctx, lf, &out[i], err)
		}
	}
	return out, nil
}

// adoptChoice brings in one folder of any kind.
func (s *Service) adoptChoice(ctx context.Context, lf mediafiles.LibraryFolder, c AdoptChoice, by domain.Actor) (AdoptResult, error) {
	switch lf.Kind {
	case "music":
		return s.adoptArtistOne(ctx, lf, c, by)
	case "show":
		return s.showParts().AdoptOne(ctx, lf, c, by)
	}
	r := AdoptResult{Folder: c.Folder}
	id, file, err := s.adoptOne(ctx, lf, c, by)
	r.TitleID, r.File = id, file
	return r, err
}

func (s *Service) adoptOne(ctx context.Context, lf mediafiles.LibraryFolder, c AdoptChoice, by domain.Actor) (int64, string, error) {
	if looseChoice(lf.Path, c.Folder) {
		return s.adoptLoose(ctx, lf, c, by)
	}
	if !validFolder(c.Folder) {
		return 0, "", errors.New("the folder must be one folder directly in the Library folder")
	}
	fi, err := os.Lstat(filepath.Join(lf.Path, c.Folder))
	if err != nil || !importer.InsideRealFile(lf.Path, filepath.Join(lf.Path, c.Folder)) {
		return 0, "", errors.New("no such folder in the Library folder")
	}
	fileOnly := fi.Mode().IsRegular() && videoExt[strings.ToLower(path.Ext(c.Folder))] && !sampleVideo(path.Base(c.Folder))
	if !fi.IsDir() && !fileOnly {
		return 0, "", errors.New("choose a movie folder or a video file")
	}
	if fileOnly {
		fold := FoldsCase(lf.Path)
		owners, err := mediafiles.AdoptedFileOwners(ctx, s.Store.Read(), lf.ID, fold, c.Folder)
		if err != nil {
			return 0, "", err
		}
		if owner := owners[FolderKey(c.Folder, fold)]; owner.TitleID != 0 {
			if err := checkAdoptedOwner(ctx, s.Store.Read(), owner, c.TMDB); err != nil {
				return 0, "", err
			}
			return owner.TitleID, c.Folder, nil
		}
	} else if len(collectionFiles(filepath.Join(lf.Path, c.Folder))) > 0 {
		return 0, "", errors.New("this folder holds several movies; scan again and choose the individual files")
	} else if _, grouped := groupOf(filepath.Join(lf.Path, c.Folder), path.Base(c.Folder), 0); grouped {
		return 0, "", errors.New("this folder groups movies; scan again and choose its contents")
	}
	if !fileOnly && collectionName(path.Base(c.Folder)) && possibleCollectionPack(c.Folder, candidateVideos(lf.Path, c.Folder)) {
		return 0, "", errors.New("possible collection pack; review its contents and import individual movie files")
	}
	// A folder already in the Library (an import run again after a restart)
	// counts as brought in, so its title is followed too.
	if !fileOnly {
		if id, err := FolderTaken(ctx, s.Store.Read(), lf, c.Folder); err != nil || id != 0 {
			return id, "", err
		}
	}
	if s.Metadata == nil {
		return 0, "", errors.New("no metadata source")
	}
	m, err := s.Metadata.Movie(ctx, "tmdb:"+c.TMDB)
	if err != nil {
		return 0, "", fmt.Errorf("TMDB %s: %w", c.TMDB, err)
	}
	a := &Adoption{FolderName: c.Folder, SharedFolder: fileOnly}
	if fileOnly {
		a.FolderName = path.Dir(c.Folder)
	}
	if v, ok := mainCandidateVideo(lf.Path, c.Folder, nil, false); ok {
		a.File, a.Size = v.Path, v.Size
	}
	if v, ok := RecordedMovieVideo(lf.Path, c.Folder, c.File); ok && !fileOnly {
		a.File, a.Size = v.Path, v.Size
	}
	return s.adoptAs(ctx, lf, c, m, a, by)
}

// looseChoice says an adopt choice names a video file directly in the
// Library folder: a movie kept without a folder of its own.
func looseChoice(root, name string) bool {
	if name == "" || strings.ContainsAny(name, `/\`) || !validFolder(name) || !videoExt[strings.ToLower(filepath.Ext(name))] {
		return false
	}
	fi, err := os.Lstat(filepath.Join(root, name))
	if err != nil {
		return false
	}
	if _, isLink := importer.LinkedFile(filepath.Join(root, name)); isLink {
		return true
	}
	return fi.Mode().IsRegular()
}

// adoptLoose adds a loose video directly in the Library folder as a movie
// with no folder of its own (folder_name ”); the biggest cut of it is the
// File. Nothing on disk changes.
func (s *Service) adoptLoose(ctx context.Context, lf mediafiles.LibraryFolder, c AdoptChoice, by domain.Actor) (int64, string, error) {
	ents, err := os.ReadDir(lf.Path)
	if err != nil {
		return 0, "", fmt.Errorf("the Library folder cannot be read: %w", err)
	}
	var vids []AdoptVideo
	for _, g := range looseGroups(lf.Path, ents) {
		if slices.ContainsFunc(g, func(v AdoptVideo) bool { return v.Path == c.Folder }) {
			vids = g
		}
	}
	if len(vids) == 0 {
		return 0, "", errors.New("no such video in the Library folder")
	}
	fold := FoldsCase(lf.Path)
	known, err := looseKnown(ctx, s.Store.Read(), lf.ID, fold)
	if err != nil {
		return 0, "", err
	}
	if id, ok := looseOwner(known, AdoptCandidate{Videos: vids}, fold); ok {
		return id, "", nil // brought in before (an import run again)
	}
	if s.Metadata == nil {
		return 0, "", errors.New("no metadata source")
	}
	m, err := s.Metadata.Movie(ctx, "tmdb:"+c.TMDB)
	if err != nil {
		return 0, "", fmt.Errorf("TMDB %s: %w", c.TMDB, err)
	}
	return s.adoptAs(ctx, lf, c, m, &Adoption{FolderName: "", File: vids[0].Path, Size: vids[0].Size}, by)
}

// adoptAs adds the movie m with the found files a, and stores its poster.
func (s *Service) adoptAs(ctx context.Context, lf mediafiles.LibraryFolder, c AdoptChoice, m metadata.Movie, a *Adoption, by domain.Actor) (int64, string, error) {
	id, err := AddMovie(ctx, s.Store, m, AddOptions{LibraryFolderID: lf.ID, WhyAdded: "import", Adopt: a,
		ReleaseGate: c.ReleaseGate, GateDelayDays: c.GateDelayDays}, s.DownloadFolders(ctx), s.now(), by)
	if err != nil {
		return 0, "", err
	}
	// The details came with the lookup; the poster is stored now, as a
	// show's is when added, so the movie has one without a refresh.
	s.CachePoster(ctx, id, m.Poster, by)
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

// looseKnown is the files directly in a Library folder that belong to a
// movie there, by their folded path: the movies kept flat.
func looseKnown(ctx context.Context, db Reader, folderID int64, fold bool) (map[string]int64, error) {
	rows, err := db.QueryContext(ctx, `SELECT f.relative_path, v.title_id FROM files f JOIN versions v ON v.id = f.version_id
		JOIN titles t ON t.id = v.title_id WHERE v.library_folder_id = ? AND v.folder_name = '' AND t.state != 'removed' AND f.state = 'present'`, folderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var rel string
		var id int64
		if err := rows.Scan(&rel, &id); err != nil {
			return nil, err
		}
		out[FolderKey(rel, fold)] = id
	}
	return out, rows.Err()
}
