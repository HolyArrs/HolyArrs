package importer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// KindImport is the job kind of an import.
const KindImport = "import_download"

// Candidate is a media file found in a download.
type Candidate struct {
	Path string
	Size int64
}

// Target describes the movie a download is placed into.
type Target struct {
	LibraryPath string
	FolderName  string
	TitleID     int64
	TitleName   string
	Year        int
	SlotIDs     []int64
	Edition     string
	IDs         map[string]string
	Naming      *mediafiles.NamingSettings
}

func (t Target) fileName(p releasename.Result, source string) string {
	ns := mediafiles.NamingDefaults()
	if t.Naming != nil {
		ns = *t.Naming
	}
	ext := filepath.Ext(source)
	release := strings.TrimSuffix(filepath.Base(source), ext)
	if !ns.RenameOnImport {
		return EditionFileName(mediafiles.CleanWith(ns, release)+strings.ToLower(ext), t.Edition)
	}
	facts := mediafiles.FactsFrom(t.TitleName, t.Year, t.IDs, p, release, ext)
	if t.Edition != "" {
		facts.Edition = t.Edition
	}
	name := mediafiles.FileName(ns, facts)
	return EditionFileName(name, t.Edition)
}

var videoExt = map[string]bool{".mkv": true, ".mp4": true, ".avi": true, ".m4v": true, ".ts": true, ".m2ts": true, ".wmv": true,
	".mov": true, ".webm": true, ".mpg": true, ".mpeg": true}

var discExt = map[string]bool{".iso": true, ".img": true, ".vob": true, ".ifo": true, ".bdmv": true, ".mts": true}

// IsVideo reports whether ext is a known video extension.
func IsVideo(ext string) bool { return videoExt[strings.ToLower(ext)] }

// IsMedia reports whether ext is a video or audio extension.
func IsMedia(ext string) bool {
	ext = strings.ToLower(ext)
	return videoExt[ext] || audioExt[ext] || discExt[ext]
}

var programExt = map[string]bool{".exe": true, ".scr": true, ".bat": true, ".cmd": true, ".msi": true, ".lnk": true,
	".js": true, ".vbs": true, ".ps1": true, ".com": true, ".sh": true,
	".dll": true, ".jar": true, ".apk": true}

func safeName(s string) string { return mediafiles.CleanName(s) }

// FolderName builds the library folder name for a title and year.
func FolderName(title string, year int) string {
	return mediafiles.Folder(mediafiles.NamingDefaults(), mediafiles.NameFacts{Title: title, Year: year})
}

// FileName builds the library file name for a title, year, step and extension.
func FileName(title string, year int, step, ext string) string {
	return mediafiles.FileName(mediafiles.NamingDefaults(), mediafiles.NameFacts{Title: title, Year: year, Step: step, Ext: ext})
}

// Scan lists the media files under root.
func Scan(root string) ([]Candidate, error) {
	var out []Candidate
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&fs.ModeSymlink != 0 || d.IsDir() {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		out = append(out, Candidate{Path: p, Size: fi.Size()})
		return nil
	})
	return out, err
}

func isSample(path string, p releasename.Result) bool {
	if p.Has("sample") {
		return true
	}
	for _, part := range strings.Split(filepath.ToSlash(strings.ToLower(path)), "/") {
		if part == "sample" || part == "samples" {
			return true
		}
	}
	base := strings.ToLower(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	for _, w := range strings.FieldsFunc(base, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if w == "sample" {
			return true
		}
	}
	return false
}

func sampleByLength(facts map[string]any, minutes int) bool {
	secs := factInt(facts, "runtime_seconds")
	if secs <= 0 {
		return false
	}
	if secs < 15 {
		return true
	}
	if minutes <= 0 {
		return false
	}
	least := 15
	switch {
	case minutes > 30:
		least = 10 * 60
	case minutes > 10:
		least = 5 * 60
	case minutes > 3:
		least = 90
	}
	return secs < least
}

func lengthWords(secs int) string {
	if secs < 120 {
		return strconv.Itoa(secs) + " seconds"
	}
	return strconv.Itoa(secs/60) + " minutes"
}

func confidence(c releasename.Conf) float64 {
	switch c {
	case releasename.Sure:
		return 1
	case releasename.Likely:
		return 0.7
	case releasename.Guess:
		return 0.4
	}
	return 0
}

// MinConfidence is the lowest match score accepted without a human check.
const MinConfidence = 0.7

// Plan turns candidate files into copy steps for a movie.
func Plan(cands []Candidate, t Target) []mediafiles.PlanStep {
	ref := mediafiles.TitleRef{ID: t.TitleID, Name: t.TitleName, Year: t.Year}
	var steps []mediafiles.PlanStep
	var mains []int
	for _, c := range cands {
		ext := strings.ToLower(filepath.Ext(c.Path))
		s := mediafiles.PlanStep{SourceFile: c.Path, TargetSlotIDs: []int64{}, Why: []string{}}
		p := releasename.Parse(filepath.Base(c.Path), releasename.Hints{Size: c.Size})
		if p.Title.Value == "" {
			p = releasename.Parse(filepath.Base(filepath.Dir(c.Path)), releasename.Hints{Size: c.Size})
		}
		switch {
		case programExt[ext] || p.Has("executable") || p.Has("double_extension"):
			s.Method, s.Why = "refuse", []string{"a program file, not a video"}
		case !videoExt[ext]:
			s.Method, s.Why = "skip", []string{"not a video file (" + ext + ")"}
		case isSample(c.Path, p):
			s.Method, s.Why = "refuse", []string{"a sample, not the full video"}
		default:
			m := mediafiles.MatchTitle(p, []mediafiles.TitleRef{ref})
			if m.Title == nil && p.Year.Value == 0 {
				if fp := releasename.Parse(filepath.Base(filepath.Dir(c.Path)), releasename.Hints{Size: c.Size}); fp.Title.Value != "" {
					if fm := mediafiles.MatchTitle(fp, []mediafiles.TitleRef{ref}); fm.Title != nil {
						m = fm
					}
				}
			}
			s.Confidence = confidence(m.Conf)
			if m.Title == nil {
				s.Method, s.Why = "refuse", []string{"does not match the Title: " + m.Detail}
				break
			}
			s.Method = "hardlink"
			s.TargetSlotIDs = t.SlotIDs
			s.TargetPath = filepath.Join(t.LibraryPath, t.FolderName, t.fileName(p, c.Path))
			s.Why = []string{m.Detail}
			if s.Confidence < MinConfidence {
				s.Method = "refuse"
				s.Why = append(s.Why, "too unsure to import without you")
			}
			mains = append(mains, len(steps))
		}
		steps = append(steps, s)
	}
	if len(mains) > 1 {
		best := mains[0]
		for _, i := range mains[1:] {
			if sizeOf(cands, steps[i].SourceFile) > sizeOf(cands, steps[best].SourceFile) {
				best = i
			}
		}
		for _, i := range mains {
			if i != best {
				steps[i].Method, steps[i].TargetPath = "skip", ""
				steps[i].Why = append(steps[i].Why, "a bigger video in the same download is the main file")
			}
		}
	}
	return steps
}

func sizeOf(cands []Candidate, path string) int64 {
	for _, c := range cands {
		if c.Path == path {
			return c.Size
		}
	}
	return 0
}

// Ops holds the file operations an import runs, so tests can swap them.
type Ops struct {
	Link func(oldname, newname string) error
	Copy func(src, dst string) error
}

var activeParts sync.Map

func sweepStaleParts(dir string) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range ents {
		n := e.Name()
		if !e.Type().IsRegular() || !strings.HasPrefix(n, ".holyarrs-") || !strings.HasSuffix(n, ".part") {
			continue
		}
		p := filepath.Join(dir, n)
		if _, live := activeParts.Load(p); live {
			continue
		}
		_ = os.Remove(p)
	}
}

// MinFreeSpace is the default free space, in bytes, an import needs.
const MinFreeSpace = 100 << 20

var freeSpaceOf = FreeSpace

// CheckSpace fails when dir cannot hold size more bytes plus the default margin.
func CheckSpace(dir string, size int64) error { return CheckSpaceMin(dir, size, MinFreeSpace/mib) }

// CopyNoReplace copies src to dst and never overwrites an existing dst.
func CopyNoReplace(src, dst string) error { return copyFile(src, dst) }

var beforeFinish func()

func copyFile(src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := CreateTemp(filepath.Dir(dst), ".holyarrs-", ".part")
	if err != nil {
		return err
	}
	tmp := out.Name()
	activeParts.Store(tmp, true)
	defer activeParts.Delete(tmp)
	defer os.Remove(tmp)
	if _, err = io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err = out.Sync(); err != nil {
		_ = out.Close()
		return err
	}
	if err = out.Close(); err != nil {
		return err
	}
	if _, err = os.Lstat(dst); err == nil {
		return fmt.Errorf("%s appeared while copying", dst)
	}
	if beforeFinish != nil {
		beforeFinish()
	}
	link := os.Link
	if linkHook != nil {
		link = linkHook
	}
	if err = link(tmp, dst); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("%s appeared while copying", dst)
		}
		if linkUnsupported(err) {
			return copyDirect(src, dst)
		}
		return err
	}
	return nil
}

// CreateTemp makes a new temporary file in dir.
func CreateTemp(dir, prefix, suffix string) (*os.File, error) {
	for range 10000 {
		name := filepath.Join(dir, prefix+strconv.FormatUint(uint64(rand.Uint32()), 10)+suffix)
		f, err := os.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o666)
		if !errors.Is(err, fs.ErrExist) {
			return f, err
		}
	}
	return nil, &fs.PathError{Op: "createtemp", Path: filepath.Join(dir, prefix+"*"+suffix), Err: fs.ErrExist}
}

var linkHook func(oldname, newname string) error

func copyDirect(src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = out.Close()
			_ = os.Remove(dst)
		}
	}()
	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	if err = out.Sync(); err != nil {
		return err
	}
	return out.Close()
}

// SameContent reports whether two files hold the same bytes.
func SameContent(a, b string, size int64) bool { return sameContent(a, b, size) }

func sameContent(a, b string, size int64) bool {
	sum := func(p string) []byte {
		f, err := os.Open(p)
		if err != nil {
			return nil
		}
		defer f.Close()
		fi, err := f.Stat()
		if err != nil || fi.Size() != size {
			return nil
		}
		h := sha256.New()
		const mib = 1 << 20
		if _, err := io.CopyN(h, f, min(size, mib)); err != nil {
			return nil
		}
		if size > mib {
			if _, err := f.Seek(max(size-mib, mib), io.SeekStart); err != nil {
				return nil
			}
			if _, err := io.Copy(h, f); err != nil {
				return nil
			}
		}
		return h.Sum(nil)
	}
	x, y := sum(a), sum(b)
	return x != nil && bytes.Equal(x, y)
}

// Execute runs the steps with ops and returns one result per step.
func Execute(steps []mediafiles.PlanStep, ops Ops) []mediafiles.PlaceResult {
	return execute(steps, ops, nil)
}

func execute(steps []mediafiles.PlanStep, ops Ops, resumable map[string]time.Time) []mediafiles.PlaceResult {
	if ops.Link == nil {
		ops.Link = os.Link
	}
	if ops.Copy == nil {
		ops.Copy = copyFile
	}
	out := make([]mediafiles.PlaceResult, 0, len(steps))
	for _, s := range steps {
		r := mediafiles.PlaceResult{SourceFile: s.SourceFile, PlacedBy: "none"}
		switch s.Method {
		case "skip":
			r.Outcome = "skipped"
		case "hardlink":
			r = place(s, ops, resumable[s.SourceFile])
		default:
			r.Outcome = "refused"
			r.Error = strings.Join(s.Why, "; ")
		}
		out = append(out, r)
	}
	return out
}

func place(s mediafiles.PlanStep, ops Ops, resumeFrom time.Time) mediafiles.PlaceResult {
	r := mediafiles.PlaceResult{SourceFile: s.SourceFile, PlacedBy: "none"}
	sfi, err := os.Lstat(s.SourceFile)
	if err != nil {
		r.Outcome, r.Error = "failed", err.Error()
		return r
	}
	if !sfi.Mode().IsRegular() {
		r.Outcome, r.Error = "refused", "the source is not a plain file (a symlink or something else)"
		return r
	}
	if tfi, err := os.Lstat(s.TargetPath); err == nil {
		if os.SameFile(sfi, tfi) {
			r.Outcome, r.PlacedBy = "placed", "hardlink"
			return r
		}
		if !resumeFrom.IsZero() && tfi.Mode().IsRegular() && !tfi.ModTime().Before(resumeFrom.Add(-2*time.Second)) && sameContent(s.SourceFile, s.TargetPath, sfi.Size()) {
			r.Outcome, r.PlacedBy, r.FellBack = "placed", "copy", true
			return r
		}
		r.Outcome, r.Error = "refused", "a file HolyArrs did not place is already at the target"
		return r
	}
	if err := os.MkdirAll(filepath.Dir(s.TargetPath), 0o777); err != nil {
		r.Outcome, r.Error = "failed", err.Error()
		return r
	}
	linkErr := ops.Link(s.SourceFile, s.TargetPath)
	if linkErr == nil {
		r.Outcome, r.PlacedBy = "placed", "hardlink"
		return r
	}
	if errors.Is(linkErr, fs.ErrExist) {
		r.Outcome, r.Error = "refused", "a file appeared at the target"
		return r
	}
	if err := CheckSpace(filepath.Dir(s.TargetPath), sfi.Size()); err != nil {
		r.Outcome, r.Error = "refused", err.Error()
		return r
	}
	if err := ops.Copy(s.SourceFile, s.TargetPath); err != nil {
		r.Outcome, r.Error = "failed", fmt.Sprintf("hardlink failed (%v), copy failed (%v)", linkErr, err)
		return r
	}
	r.Outcome, r.PlacedBy, r.FellBack = "placed", "copy", true
	r.Error = "hardlink not possible: " + linkErr.Error()
	return r
}

func under(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func insideReal(root, p string) bool {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return true
	}
	for q := p; ; {
		if real, err := filepath.EvalSymlinks(q); err == nil {
			return real == realRoot || under(realRoot, real)
		}
		up := filepath.Dir(q)
		if up == q {
			return false
		}
		q = up
	}
}

func candidates(root string, files []domain.SeenFile) ([]Candidate, []mediafiles.PlanStep) {
	var cands []Candidate
	var bad []mediafiles.PlanStep
	realRoot, rootErr := filepath.EvalSymlinks(root)
	for _, f := range files {
		name := strings.ReplaceAll(f.Name, `\`, "/")
		ok := name != "" && !strings.HasPrefix(name, "/") && !filepath.IsAbs(name) && filepath.VolumeName(filepath.FromSlash(name)) == ""
		for _, part := range strings.Split(name, "/") {
			if part == ".." {
				ok = false
			}
		}
		cand := filepath.Join(root, filepath.FromSlash(name))
		if ok {
			ok = under(root, cand)
		}
		if ok && rootErr == nil {
			if real, err := filepath.EvalSymlinks(cand); err == nil && !under(realRoot, real) {
				ok = false
			}
		}
		if !ok {
			bad = append(bad, mediafiles.PlanStep{SourceFile: f.Name, TargetSlotIDs: []int64{}, Method: "refuse",
				Why: []string{"the Downloader reported a file outside the download folder"}})
			continue
		}
		cands = append(cands, Candidate{Path: cand, Size: f.Size})
	}
	return cands, bad
}

// Publisher announces import events.
type Publisher interface {
	Publish(topic string, data any) uint64
}

// Importer places finished downloads into the library.
type Importer struct {
	Store        *database.Store
	Live         Publisher
	Ops          Ops
	SavePath     string
	OnRejected   func(ctx context.Context, slotID int64)
	OnImported   func(ctx context.Context, downloadID int64)
	OnLowSpace   func(ctx context.Context, dir string, free, need int64)
	NoSizeFloors bool

	runs sync.Map

	spaceMu sync.Mutex
	booked  map[string]int64
}

func (im *Importer) publish(topic string, data any) {
	if im.Live != nil {
		im.Live.Publish(topic, data)
	}
}

// ErrEpisodeImport is returned when placing episodes is turned off.
var ErrEpisodeImport = errors.New("placing episodes is turned off; the download stays in the Downloader")

// ErrAlbumImport is returned when placing albums is turned off.
var ErrAlbumImport = errors.New("placing albums is turned off; the download stays in the Downloader")

var canImportAlbums = true

var canImportEpisodes = true

// RequeueStranded queues again the downloads whose import never finished.
func (im *Importer) RequeueStranded(ctx context.Context, enqueue func(ctx context.Context, downloadID int64)) (int, error) {
	if !canImportEpisodes {
		return 0, nil
	}
	rows, err := im.Store.Read().QueryContext(ctx, `SELECT DISTINCT d.id FROM downloads d, json_each(d.slot_ids) j
		JOIN slots s ON s.id = j.value JOIN items i ON i.id = s.item_id JOIN titles t ON t.id = i.title_id
		WHERE d.state = 'finished' AND s.state = 'downloading' AND (t.kind = 'show' OR t.kind = 'artist' AND ?)
		AND NOT EXISTS (SELECT 1 FROM imports x WHERE x.download_id = d.id AND x.state IN ('approved', 'placing', 'done', 'partly_done', 'waiting_for_you'))
		ORDER BY d.id`, canImportAlbums)
	if err != nil {
		return 0, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	n := 0
	for _, id := range ids {
		if wait, err := im.importBackoff(ctx, id); err != nil {
			return n, err
		} else if wait {
			continue
		}
		enqueue(ctx, id)
		n++
	}
	return n, nil
}

func (im *Importer) importBackoff(ctx context.Context, downloadID int64) (bool, error) {
	var failed int
	var last sql.NullString
	err := im.Store.Read().QueryRowContext(ctx, `SELECT count(*), max(ended_at) FROM jobs WHERE kind = ? AND state = 'failed'
		AND json_extract(subject, '$.type') = 'download' AND json_extract(subject, '$.id') = ?`, KindImport, downloadID).Scan(&failed, &last)
	if err != nil || failed == 0 || !last.Valid {
		return false, err
	}
	end, err := time.Parse(time.RFC3339Nano, last.String)
	if err != nil {
		return false, nil
	}
	// The wait doubles per failure from ten minutes and stops at one day.
	wait := 24 * time.Hour
	if failed <= 8 {
		wait = min(wait, 10*time.Minute<<(failed-1))
	}
	return time.Since(end) < wait, nil
}

// HandleJob runs an import job.
func (im *Importer) HandleJob(ctx context.Context, r *jobs.Run) error {
	if r.Job.Subject == nil || r.Job.Subject.Type != "download" {
		return errors.New("import needs a download subject")
	}
	dry, _ := r.Job.Params["dry_run"].(bool)
	imp, err := im.Import(ctx, r.Job.Subject.ID, dry, domain.Actor{Kind: "job", ID: r.Job.ID})
	if errors.Is(err, ErrEpisodeImport) || errors.Is(err, ErrAlbumImport) {
		r.SetResult(map[string]any{"waiting": err.Error()})
		return nil
	}
	if err != nil {
		return err
	}
	r.SetResult(map[string]any{"import_id": imp.ID, "state": imp.State})
	return nil
}

func mapPath(p string, maps []domain.PathMap) string {
	out, _ := domain.MapPath(p, maps)
	return out
}

// Import places one download into the library, or only plans it when dryRun is set.
func (im *Importer) Import(ctx context.Context, downloadID int64, dryRun bool, by domain.Actor) (*mediafiles.Import, error) {
	unlock := im.Store.LockFiles()
	defer unlock()
	db := im.Store.Read()
	var d domain.Download
	if err := database.Get(ctx, db, "downloads", downloadID, &d); err != nil {
		return nil, fmt.Errorf("download %d: %w", downloadID, err)
	}
	if d.State != "finished" && d.State != "seeding" {
		return nil, fmt.Errorf("download %d is %s, not finished", downloadID, d.State)
	}
	if len(d.SlotIDs) == 0 {
		return nil, fmt.Errorf("download %d has no Slot", downloadID)
	}
	var dr domain.Downloader
	if err := database.Get(ctx, db, "downloaders", d.DownloaderID, &dr); err != nil {
		return nil, err
	}
	var sl mediafiles.Slot
	var ver mediafiles.Version
	var lf mediafiles.LibraryFolder
	var it mediafiles.Item
	var ti mediafiles.Title
	if err := database.Get(ctx, db, "slots", d.SlotIDs[0], &sl); err != nil {
		return nil, err
	}
	var kind, titleState string
	if err := db.QueryRowContext(ctx, "SELECT t.kind, t.state FROM items i JOIN titles t ON t.id = i.title_id WHERE i.id = ?", sl.ItemID).Scan(&kind, &titleState); err != nil {
		return nil, err
	}
	if titleState == "removed" || sl.State == mediafiles.SlotRemoved {
		return nil, errors.New("the title or item was removed; import stopped")
	}
	if kind == "artist" {
		if !canImportAlbums {
			return nil, ErrAlbumImport
		}
		return im.importShow(ctx, d, dr, dryRun, by)
	}
	if kind == "show" {
		if !canImportEpisodes {
			return nil, ErrEpisodeImport
		}
		return im.importShow(ctx, d, dr, dryRun, by)
	}
	if !dryRun && sl.State != mediafiles.SlotDownloading && sl.State != mediafiles.SlotImporting {
		return nil, fmt.Errorf("slot %d is %s; nothing to import", sl.ID, sl.State)
	}
	for _, g := range []struct {
		table string
		id    int64
		dst   any
	}{{"versions", sl.VersionID, &ver}, {"items", sl.ItemID, &it}} {
		if err := database.Get(ctx, db, g.table, g.id, g.dst); err != nil {
			return nil, err
		}
	}
	if err := database.Get(ctx, db, "library_folders", ver.LibraryFolderID, &lf); err != nil {
		return nil, err
	}
	if err := database.Get(ctx, db, "titles", it.TitleID, &ti); err != nil {
		return nil, err
	}
	year := 0
	if ti.Year != nil {
		year = *ti.Year
	}
	root, rootOK := domain.MapPath(d.SavePath, dr.PathMap)
	cands, bad := candidates(root, d.FilesSeen)
	if !rootOK {
		cands, bad = nil, []mediafiles.PlanStep{unpackStep(d.SavePath, "the Downloader reported a folder with \"..\" in it")}
	}
	folder := safeName(ver.FolderName)
	if folder == "" || folder == "." || folder == ".." {
		folder = FolderName(ti.Name, year)
	}
	ns, err := mediafiles.ReadNaming(ctx, db)
	if err != nil {
		return nil, err
	}
	rules, err := ReadFileRules(ctx, db)
	if err != nil {
		return nil, err
	}
	unpacked, cleanup, unpackWhy := unpackRars(ctx, d.ID, root, cands, rules, dryRun)
	defer cleanup()
	cands = append(cands, unpacked...)
	t := Target{LibraryPath: lf.Path, FolderName: folder, TitleID: ti.ID, TitleName: ti.Name, Year: year, SlotIDs: d.SlotIDs, IDs: ti.SourceIDs, Naming: &ns, Edition: ver.Edition}
	plan := append(Plan(cands, t), bad...)
	if unpackWhy != "" {
		plan = append(plan, unpackStep(root, unpackWhy))
	}
	lowSpace, release := im.guard(plan, lf.Path, cands, rules)
	defer release()
	minutes := 0
	if it.Runtime != nil {
		minutes = *it.Runtime
	}
	up, err := checkUpgrade(ctx, db, sl, ver, d, plan, im.refuseFakes(ctx, db), minutes)
	if err != nil {
		return nil, err
	}
	imp := &mediafiles.Import{DownloadID: &d.ID, FromPath: root, Kind: "movie", Plan: plan, Result: []mediafiles.PlaceResult{}, Mode: "dry_run", State: "planned"}
	if dryRun {
		return imp, nil
	}
	imp.Mode = "auto"
	if lowSpace != nil {
		im.lowSpace(ctx, lowSpace, sl.ID, ti.ID, d.ID, by)
	}
	if up.lie != "" && !hasMain(plan) {
		slotTo, err := im.reject(ctx, imp, sl, d, up.lie, by, up.old != nil, ver.KeepUpToDate && it.KeepUpToDate)
		if err != nil {
			return imp, err
		}
		slotID, dlID, titleID := sl.ID, d.ID, ti.ID
		e := &domain.Event{Actor: by, Kind: "import_rejected", TitleID: &titleID, SlotID: &slotID, DownloadID: &dlID, Summary: "Import rejected: " + up.lie,
			Data: map[string]any{"import_id": imp.ID, "release_id": d.ReleaseID, "reason": up.lie}, GroupKey: "slot:" + strconv.FormatInt(slotID, 10)}
		if im.Store.AddEvent(ctx, e) == nil {
			im.publish("event", e)
		}
		im.publish("slot", map[string]any{"id": sl.ID, "state": slotTo, "reason": up.lie})
		im.publish("import", imp)
		if im.OnRejected != nil {
			im.OnRejected(ctx, sl.ID)
		}
		return imp, nil
	}
	prior, err := database.Select[mediafiles.Import](ctx, db, "imports", "download_id = ? AND state = 'placing' ORDER BY id DESC LIMIT 1", d.ID)
	if err != nil {
		return nil, err
	}
	resumable := map[string]time.Time{}
	recorded := ""
	if len(prior) == 1 {
		imp.Meta = prior[0].Meta
		for _, r := range prior[0].Result {
			if r.Outcome == "placing" || r.Outcome == "placed" {
				resumable[r.SourceFile] = prior[0].CreatedAt
			}
			if r.HeldAside != "" {
				recorded = r.HeldAside
			}
		}
	}
	for _, st := range plan {
		r := mediafiles.PlaceResult{SourceFile: st.SourceFile, Outcome: "pending", PlacedBy: "none"}
		if st.Method == "hardlink" {
			r.Outcome = "placing"
		}
		imp.Result = append(imp.Result, r)
	}
	imp.State = "placing"
	moved := false
	if err := im.Store.Write(ctx, func(tx *sql.Tx) error {
		if sl.State == mediafiles.SlotDownloading {
			if err := mediafiles.MoveSlotTx(tx, sl.ID, mediafiles.SlotImporting, "importing", by); err != nil {
				return err
			}
			moved = true
		}
		if imp.ID > 0 {
			return database.Update(tx, "imports", imp, by)
		}
		_, err := database.Insert(tx, "imports", imp, by)
		return err
	}); err != nil {
		return nil, err
	}
	if moved {
		im.publish("slot", map[string]any{"id": sl.ID, "state": mediafiles.SlotImporting})
	}

	swept := map[string]bool{}
	for _, st := range plan {
		if st.Method == "hardlink" && !swept[filepath.Dir(st.TargetPath)] {
			swept[filepath.Dir(st.TargetPath)] = true
			sweepStaleParts(filepath.Dir(st.TargetPath))
		}
	}
	heldEarly, sameIdx, oldAbs := "", -1, ""
	if up.old != nil {
		oldAbs = filepath.Join(lf.Path, filepath.FromSlash(up.old.RelativePath))
		for i, st := range plan {
			if st.Method == "hardlink" && filepath.Clean(st.TargetPath) == filepath.Clean(oldAbs) {
				sameIdx = i
			}
		}
	}
	if sameIdx >= 0 {
		if _, err := os.Lstat(recorded); recorded != "" && err == nil {
			heldEarly = recorded
		} else if _, err := os.Lstat(oldAbs); err == nil {
			to := recorded
			if to == "" {
				if to, err = holdPath(lf.Path, up.old.RelativePath, time.Now()); err != nil {
					return imp, fmt.Errorf("moving the old copy aside: %w", err)
				}
			}
			imp.Result[sameIdx].HeldAside = to
			if err := im.Store.Write(ctx, func(tx *sql.Tx) error { return database.Update(tx, "imports", imp, by) }); err != nil {
				return imp, err
			}
			if err := moveFile(oldAbs, to); err != nil {
				return imp, fmt.Errorf("moving the old copy aside: %w", err)
			}
			heldEarly = to
		}
		if crashAt != nil {
			if err := crashAt("held"); err != nil {
				return imp, err
			}
		}
	}
	imp.Result = execute(plan, im.Ops, resumable)
	if heldEarly != "" {
		imp.Result[sameIdx].HeldAside = heldEarly
	}
	if crashAt != nil {
		if err := crashAt("placed"); err != nil {
			return imp, err
		}
	}
	replacedTo := heldEarly
	if up.old != nil {
		placedAny := placedSome(imp.Result)
		switch {
		case !placedAny && heldEarly != "":
			if err := moveFile(heldEarly, oldAbs); err != nil {
				log.Printf("importer: putting the old copy back from %s: %v", heldEarly, err)
			} else {
				replacedTo = ""
				imp.Result[sameIdx].HeldAside = ""
			}
		case placedAny && heldEarly == "" && sameIdx < 0:
			to, err := recorded, error(nil)
			if _, lerr := os.Lstat(oldAbs); lerr != nil {
				if _, rerr := os.Lstat(recorded); recorded == "" || rerr != nil {
					to = ""
				}
			} else {
				if to == "" {
					to, err = holdPath(lf.Path, up.old.RelativePath, time.Now())
				}
				if err == nil {
					for i := range imp.Result {
						if imp.Result[i].Outcome == "placed" {
							imp.Result[i].HeldAside = to
							break
						}
					}
					err = im.Store.Write(ctx, func(tx *sql.Tx) error { return database.Update(tx, "imports", imp, by) })
				}
				if err == nil {
					err = moveFile(oldAbs, to)
				}
				if err == nil && crashAt != nil {
					if cerr := crashAt("held_late"); cerr != nil {
						return imp, cerr
					}
				}
			}
			if err != nil {
				to = ""
				log.Printf("importer: moving the replaced copy aside: %v", err)
				e := &domain.Event{Actor: by, Kind: "file_hold_failed", SlotID: &sl.ID, Summary: "Could not move the replaced copy aside: " + err.Error(),
					Data: map[string]any{"file_id": up.old.ID, "path": up.old.RelativePath}, GroupKey: "slot:" + strconv.FormatInt(sl.ID, 10)}
				if im.Store.AddEvent(ctx, e) == nil {
					im.publish("event", e)
				}
			}
			replacedTo = to
		}
	}
	sizes := make([]int64, len(plan))
	for i, r := range imp.Result {
		if r.Outcome == "placed" {
			if fi, err := os.Stat(plan[i].TargetPath); err == nil {
				sizes[i] = fi.Size()
			}
		}
	}
	how := ""
	for _, r := range imp.Result {
		if r.Outcome == "placed" {
			how = r.PlacedBy
			if r.FellBack {
				how += " (hardlink not possible)"
			}
		}
	}
	slotTo, reason := mediafiles.SlotNeedsADecision, "nothing could be placed; look at the import plan"
	if unpackWhy != "" {
		reason = unpackWhy
	}
	if up.copyWhy != "" {
		reason = up.copyWhy
	}
	for _, r := range imp.Result {
		if strings.HasPrefix(r.Error, "not enough free space") {
			reason = "nothing could be placed: not enough free space in the Library folder"
		}
	}
	placed, failed := 0, 0
	var fileID int64
	newName := ""
	now := time.Now().UTC()
	err = im.Store.Write(ctx, func(tx *sql.Tx) error {
		if up.old != nil && placedSome(imp.Result) {
			relPath := up.old.RelativePath
			if replacedTo != "" {
				if rp, err := filepath.Rel(lf.Path, replacedTo); err == nil {
					relPath = filepath.ToSlash(rp)
				}
			} else if sameIdx >= 0 {
				relPath += "#replaced-" + strconv.FormatInt(up.old.ID, 10)
			}
			if _, err := tx.Exec("UPDATE files SET state = 'replaced', relative_path = ?, updated_at = ?, updated_by = ? WHERE id = ?",
				relPath, database.FormatTime(now), actorText(by), up.old.ID); err != nil {
				return err
			}
		}
		for i, r := range imp.Result {
			switch r.Outcome {
			case "placed":
				placed++
			case "failed":
				failed++
				continue
			default:
				continue
			}
			step := plan[i]
			rel, err := filepath.Rel(lf.Path, step.TargetPath)
			if err != nil {
				return err
			}
			p := releasename.Parse(filepath.Base(step.SourceFile), releasename.Hints{})
			size := sizes[i]
			name := strings.TrimSuffix(filepath.Base(step.SourceFile), filepath.Ext(step.SourceFile))
			newName = name
			f := &mediafiles.File{SlotIDs: step.TargetSlotIDs, VersionID: ver.ID, RelativePath: filepath.ToSlash(rel), Size: size, AddedAt: now,
				Quality:   mediafiles.Quality{Resolution: p.Resolution.Value, Source: p.Source.Value, Codec: p.Codec.Value, HDR: p.HDR.Value},
				Languages: InspectedLanguages(up.facts[step.SourceFile]), MediaInfo: map[string]any{"step": mediafiles.StepFor(p)}, ReleaseName: &name, Origin: "imported", PlacedBy: r.PlacedBy,
				FellBack: r.FellBack, Role: "main", State: "present"}
			if f.Languages == nil {
				f.Languages = []string{}
			}
			for k, value := range up.facts[step.SourceFile] {
				f.MediaInfo[k] = value
			}
			if g := p.Group.Value; g != "" {
				f.Group = &g
			}
			id, err := database.Insert(tx, "files", f, by)
			if err != nil {
				return err
			}
			fileID = id
		}
		switch {
		case placed > 0 && failed == 0:
			imp.State = "done"
		case placed > 0:
			imp.State = "partly_done"
		default:
			imp.State = "failed"
		}
		if err := database.Update(tx, "imports", imp, by); err != nil {
			return err
		}
		ge := false
		if fileID > 0 {
			ge = mediafiles.ReachesGoodEnough(newName, up.rules)
			if _, err := tx.Exec("UPDATE slots SET file_id = ?, active_download_id = NULL, good_enough = ? WHERE id = ?", fileID, ge, sl.ID); err != nil {
				return err
			}
			slotTo, reason = mediafiles.SlotInLibrary, "placed by "+how
			if up.old != nil {
				reason += "; replaced " + up.oldName
			}
		}
		if err := mediafiles.MoveSlotTx(tx, sl.ID, slotTo, reason, by); err != nil {
			return err
		}
		if slotTo == mediafiles.SlotInLibrary && !ge && ver.KeepUpToDate && it.KeepUpToDate {
			slotTo = mediafiles.SlotBetterWanted
			return mediafiles.MoveSlotTx(tx, sl.ID, slotTo, reason+"; below good enough, so a better copy is sought", by)
		}
		return nil
	})
	if err != nil {
		return imp, err
	}
	im.publish("slot", map[string]any{"id": sl.ID, "state": slotTo, "reason": reason})
	summary := fmt.Sprintf("Import %s: %d placed", imp.State, placed)
	if how != "" {
		summary += " by " + how
	}
	if placed == 0 && imp.State == "failed" {
		summary = "Import failed: " + reason
	}
	slotID, dlID, titleID := sl.ID, d.ID, ti.ID
	e := &domain.Event{Actor: by, Kind: "import_" + imp.State, TitleID: &titleID, SlotID: &slotID, DownloadID: &dlID, Summary: summary,
		Data: map[string]any{"import_id": imp.ID, "result": imp.Result, "from": imp.FromPath, "paths": planPaths(imp.Plan)}, GroupKey: "slot:" + strconv.FormatInt(slotID, 10)}
	if im.Store.AddEvent(ctx, e) == nil {
		im.publish("event", e)
	}
	if up.old != nil && fileID > 0 {
		e := &domain.Event{Actor: by, Kind: "file_replaced", TitleID: &titleID, SlotID: &slotID, DownloadID: &dlID,
			Summary: "Replaced " + up.oldName + " with a better copy", Data: map[string]any{"old_file_id": up.old.ID, "new_file_id": fileID, "moved_to": replacedTo},
			GroupKey: "slot:" + strconv.FormatInt(slotID, 10)}
		im.upgradeFacts(ctx, e.Data, up.old.ID, fileID)
		if im.Store.AddEvent(ctx, e) == nil {
			im.publish("event", e)
		}
	}
	im.publish("import", imp)
	if imp.State == "done" && !dryRun && im.OnImported != nil {
		im.OnImported(ctx, d.ID)
	}
	return imp, nil
}

func guardPlan(plan []mediafiles.PlanStep, lib string, cands []Candidate, rules FileRules) *SpaceError {
	low, _ := guardPlanWith(plan, lib, cands, rules, nil)
	return low
}

func guardPlanWith(plan []mediafiles.PlanStep, lib string, cands []Candidate, rules FileRules, base map[string]int64) (*SpaceError, map[string]int64) {
	var lowSpace *SpaceError
	reserved := map[string]int64{}
	mine := map[string]int64{}
	for k, n := range base {
		reserved[k] = n
	}
	for i := range plan {
		if plan[i].TargetPath != "" && (!under(lib, plan[i].TargetPath) || !insideReal(lib, filepath.Dir(plan[i].TargetPath))) {
			plan[i].Method, plan[i].TargetPath = "refuse", ""
			plan[i].Why = append(plan[i].Why, "the target would be outside the Library folder")
		}
		if plan[i].Method == "hardlink" {
			if why := PathTooLong(plan[i].TargetPath); why != "" {
				plan[i].Method, plan[i].TargetPath = "refuse", ""
				plan[i].Why = append([]string{why}, plan[i].Why...)
			}
		}
		if plan[i].Method == "hardlink" && !rules.SkipFreeSpaceCheck {
			dir := existingDir(filepath.Dir(plan[i].TargetPath))
			key := diskID(dir)
			if key == "" {
				key = dir
			}
			size := sizeOf(cands, plan[i].SourceFile)
			if err := checkSpaceReserved(dir, size, reserved[key], rules.MinFreeMB); err != nil {
				errors.As(err, &lowSpace)
				plan[i].Method, plan[i].TargetPath = "refuse", ""
				plan[i].Why = append([]string{err.Error()}, plan[i].Why...)
				continue
			}
			if !sameDiskOf(plan[i].SourceFile, dir) {
				reserved[key] += size
				mine[key] += size
			}
		}
	}
	return lowSpace, mine
}

func (im *Importer) guard(plan []mediafiles.PlanStep, lib string, cands []Candidate, rules FileRules) (*SpaceError, func()) {
	im.spaceMu.Lock()
	defer im.spaceMu.Unlock()
	if im.booked == nil {
		im.booked = map[string]int64{}
	}
	low, mine := guardPlanWith(plan, lib, cands, rules, im.booked)
	for k, n := range mine {
		im.booked[k] += n
	}
	return low, func() {
		im.spaceMu.Lock()
		defer im.spaceMu.Unlock()
		for k, n := range mine {
			if im.booked[k] -= n; im.booked[k] <= 0 {
				delete(im.booked, k)
			}
		}
	}
}

var sameDiskOf = sameDisk

func sameDisk(src, dir string) bool {
	a, b := diskID(src), diskID(dir)
	return a != "" && a == b
}

// MaxPathLen is the longest path the system allows.
var MaxPathLen = func() int {
	if runtime.GOOS == "windows" {
		return 259
	}
	return 4095
}()

// PathTooLong returns a message when p is longer than MaxPathLen, or an empty string.
func PathTooLong(p string) string {
	if len(p) > MaxPathLen {
		return fmt.Sprintf("the new path would be too long (%d characters, the limit here is %d)", len(p), MaxPathLen)
	}
	return ""
}

func planPaths(plan []mediafiles.PlanStep) []map[string]string {
	out := []map[string]string{}
	for _, st := range plan {
		out = append(out, map[string]string{"from": st.SourceFile, "to": st.TargetPath, "method": st.Method})
	}
	return out
}

func placedSome(rs []mediafiles.PlaceResult) bool {
	for _, r := range rs {
		if r.Outcome == "placed" {
			return true
		}
	}
	return false
}

func hasMain(plan []mediafiles.PlanStep) bool {
	for _, st := range plan {
		if st.Method == "hardlink" {
			return true
		}
	}
	return false
}

func (im *Importer) lowSpace(ctx context.Context, se *SpaceError, slotID, titleID, dlID int64, by domain.Actor) {
	e := &domain.Event{Actor: by, Kind: domain.EventFreeSpaceLow, TitleID: &titleID, SlotID: &slotID, DownloadID: &dlID, Summary: "Import refused: " + se.Error(),
		Data: map[string]any{"folder": se.Dir, "free": se.Free, "need": se.Need, "keep": se.Keep}, GroupKey: "slot:" + strconv.FormatInt(slotID, 10)}
	if im.Store.AddEvent(ctx, e) == nil {
		im.publish("event", e)
	}
	if im.OnLowSpace != nil {
		im.OnLowSpace(ctx, se.Dir, se.Free, se.Need)
	}
}
