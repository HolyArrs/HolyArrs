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
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

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
		linked, isLink := LinkedFile(p)
		if (d.Type()&fs.ModeSymlink != 0 && !isLink) || d.IsDir() {
			return nil
		}
		fi, err := d.Info()
		if isLink {
			fi, err = linked, nil
		}
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
	// Move renames a file without replacing the target.
	Move func(src, dst string) error
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

// openHook sees every source a copy opens (tests count the reads).
var openHook func(path string)

func openSource(src string) (*os.File, error) {
	if openHook != nil {
		openHook(src)
	}
	return os.Open(src)
}

func copyFile(src, dst string) (err error) {
	in, err := openSource(src)
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
		// No hardlink here: rename the finished copy into place rather than
		// writing the whole file a second time.
		rerr := renameNoReplace(tmp, dst)
		if rerr == nil {
			return nil
		}
		if errors.Is(rerr, fs.ErrExist) {
			return fmt.Errorf("%s appeared while copying", dst)
		}
		return copyDirect(src, dst)
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
	in, err := openSource(src)
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

// sameFullContent hashes both files in full and compares them.
func sameFullContent(a, b string) bool {
	sum := func(p string) []byte {
		f, err := os.Open(p)
		if err != nil {
			return nil
		}
		defer f.Close()
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			return nil
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
	if ops.Move == nil {
		ops.Move = renameNoReplace
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
	_, isLink := LinkedFile(s.SourceFile)
	if !sfi.Mode().IsRegular() && !isLink {
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
		// Another app may have placed the same file first; take it as ours.
		if tfi.Mode().IsRegular() && tfi.Size() == sfi.Size() && sameFullContent(s.SourceFile, s.TargetPath) {
			r.Outcome, r.PlacedBy, r.FellBack = "placed", "copy", true
			return r
		}
		r.Outcome, r.Error = "refused", "a file HolyArrs did not place is already at the target"
		return r
	}
	if isLink {
		// A followed link is placed as a link: its target is never copied.
		if err := placeLink(s.SourceFile, s.TargetPath); err != nil {
			r.Outcome, r.Error = "failed", err.Error()
			return r
		}
		r.Outcome, r.PlacedBy = "placed", "copy" // placed_by has no link kind; a link is a copy of the entry
		return r
	}
	if err := os.MkdirAll(filepath.Dir(s.TargetPath), 0o777); err != nil {
		r.Outcome, r.Error = "failed", err.Error()
		return r
	}
	moveNote := ""
	if s.Move && ops.Move != nil {
		err := ops.Move(s.SourceFile, s.TargetPath)
		if err == nil {
			r.Outcome, r.PlacedBy = "placed", "move"
			return r
		}
		if errors.Is(err, fs.ErrExist) {
			r.Outcome, r.Error = "refused", "a file appeared at the target"
			return r
		}
		// Another drive or a refused rename: link or copy as for any download.
		moveNote = "move not possible: " + err.Error() + "; "
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
	r.Error = moveNote + "hardlink not possible: " + linkErr.Error()
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
			if _, isLink := LinkedFile(cand); isLink {
				if real, err := filepath.EvalSymlinks(filepath.Dir(cand)); err == nil && real != realRoot && !under(realRoot, real) {
					ok = false
				}
			} else if real, err := filepath.EvalSymlinks(cand); err == nil && !under(realRoot, real) {
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

// sourcesGone reports whether another app took a finished download's files:
// every file the plan would read is gone while the download's own folder is
// still there. For a torrent that folder is the Downloader's save folder, so
// a missing one is a wrong path map or an empty mount, never proof. A Usenet
// job folder (ownFolder) belongs to the download alone; it may be gone too
// when the folder above it is still there.
func sourcesGone(root string, plan []mediafiles.PlanStep, ownFolder bool) bool {
	if root == "" {
		return false
	}
	if _, err := os.Stat(root); err != nil {
		if !ownFolder || !errors.Is(err, fs.ErrNotExist) {
			return false
		}
		if _, err := os.Stat(filepath.Dir(root)); err != nil {
			return false
		}
	}
	seen := false
	for _, st := range plan {
		if !filepath.IsAbs(st.SourceFile) {
			continue
		}
		if _, err := os.Lstat(st.SourceFile); !errors.Is(err, fs.ErrNotExist) {
			return false
		}
		seen = true
	}
	if !seen {
		_, err := os.Lstat(root)
		return errors.Is(err, fs.ErrNotExist)
	}
	return true
}

// markGone records every step that was not placed as gone, with the reason.
func markGone(res []mediafiles.PlaceResult) {
	for i := range res {
		if res[i].Outcome != "placed" && res[i].Outcome != "skipped" {
			res[i].Outcome, res[i].Error = "gone", FilesGoneReason
		}
	}
}

// FilesGoneReason is why an import let its download go: every file it would
// read was already removed, as another app sharing the Downloader does after
// its own import.
const FilesGoneReason = "the files were removed by another app before HolyArrs could import them"

// Publisher announces import events.
type Publisher interface {
	Publish(topic string, data any) uint64
}

// Importer places finished downloads into the library.
type Importer struct {
	Store      *database.Store
	Live       Publisher
	Ops        Ops
	SavePath   string
	OnRejected func(ctx context.Context, slotID int64)
	OnImported func(ctx context.Context, downloadID int64)
	// OnFilesGone runs when a finished download's files were all taken by
	// another app before the import, so a rescan can link the copy that app
	// placed in the Title's folder.
	OnFilesGone  func(ctx context.Context, titleID int64)
	OnLowSpace   func(ctx context.Context, dir string, free, need int64)
	NoSizeFloors bool
	// MayMove says whether a Downloader's finished files may be moved out of
	// its folder. Nil never moves.
	MayMove func(d domain.Downloader) bool

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

// ErrPackWaiting is returned when every episode a season pack fetched a few
// at a time waits for is still downloading; the download goes back to
// downloading.
var ErrPackWaiting = errors.New("the season pack's episodes are still downloading; the download goes back to downloading")

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
		WHERE d.state = 'finished' AND s.state = 'downloading' AND (s.active_download_id IS NULL OR s.active_download_id = d.id) AND (t.kind = 'show' OR t.kind = 'artist' AND ?)
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
	if errors.Is(err, ErrEpisodeImport) || errors.Is(err, ErrAlbumImport) || errors.Is(err, ErrPackWaiting) {
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
