package library

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
)

// MoveStep is one File in a move plan.
type MoveStep struct {
	FileID int64  `json:"file_id"`
	Origin string `json:"origin"`
	From   string `json:"from"`
	To     string `json:"to"`
}

// MovePlan is the dry run of moving a Version to another Library folder.
type MovePlan struct {
	VersionID int64      `json:"version_id"`
	FromDir   string     `json:"from_folder"`
	ToDir     string     `json:"to_folder"`
	Steps     []MoveStep `json:"steps"`
	// Blocked says why the move cannot be applied: HolyArrs moves only Files
	// it imported or adopted, never ones it merely found.
	Blocked          string `json:"blocked,omitempty"`
	fromRoot, toRoot string
}

// folderKindOf is the Library folder kind a Title kind lives in, with the
// error for a folder of another kind.
func folderKindOf(kind string) (string, error) {
	switch kind {
	case "show":
		return "show", ErrNotShowFolder
	case "artist":
		return "music", ErrNotMusicFolder
	}
	return "movie", ErrNotMovieFolder
}

// PlanMove works out moving a Title's main Version into another Library
// folder (other Versions keep their own folders). Nothing on disk changes.
func PlanMove(ctx context.Context, db database.Querier, lister database.Lister, titleID, folderID int64) ([]MovePlan, error) {
	var to mediafiles.LibraryFolder
	if err := database.Get(ctx, db, "library_folders", folderID, &to); err != nil {
		return nil, ErrNoFolder
	}
	var t mediafiles.Title
	if err := database.Get(ctx, db, "titles", titleID, &t); err != nil {
		return nil, err
	}
	if want, wrong := folderKindOf(t.Kind); to.Kind != want {
		return nil, wrong
	}
	vs, err := database.Select[mediafiles.Version](ctx, lister, "versions", "title_id = ? AND is_main = 1 ORDER BY id", titleID)
	if err != nil {
		return nil, err
	}
	var out []MovePlan
	for _, v := range vs {
		if v.LibraryFolderID == folderID {
			continue
		}
		from, fromLib, err := versionFolder(ctx, db, v)
		if err != nil {
			return nil, err
		}
		p := MovePlan{VersionID: v.ID, FromDir: from, ToDir: filepath.Join(to.Path, v.FolderName), Steps: []MoveStep{},
			fromRoot: fromLib.Path, toRoot: to.Path}
		files, err := database.Select[mediafiles.File](ctx, lister, "files", "version_id = ? AND state = 'present' ORDER BY id", v.ID)
		if err != nil {
			return nil, err
		}
		found := 0
		for _, f := range files {
			// A File's path is relative to its Library folder; what is moved is
			// its place inside the Version folder.
			rel, err := filepath.Rel(p.FromDir, filepath.Join(fromLib.Path, filepath.FromSlash(f.RelativePath)))
			if err != nil || !filepath.IsLocal(rel) {
				p.Blocked = "a File path leaves its Version folder: " + f.RelativePath
				continue
			}
			if f.Origin != "imported" && f.Origin != "adopted" {
				found++
				continue
			}
			step := MoveStep{FileID: f.ID, Origin: f.Origin, From: filepath.Join(p.FromDir, rel), To: filepath.Join(p.ToDir, rel)}
			if !importer.InsideReal(p.fromRoot, step.From) || !importer.InsideReal(p.toRoot, step.To) {
				p.Blocked = "a link leads a File outside its Library folder"
				continue
			}
			p.Steps = append(p.Steps, step)
		}
		if found > 0 && p.Blocked == "" {
			p.Blocked = fmt.Sprintf("%d files were found in the folder, not imported or adopted; HolyArrs never moves those", found)
		}
		out = append(out, p)
	}
	return out, nil
}

// handleMove plans a move (apply false) or carries it out (apply true).
func (s *Service) handleMove(ctx context.Context, r *jobs.Run) error {
	if r.Job.Subject == nil || r.Job.Subject.Type != "title" {
		return errors.New("move_title needs a title subject")
	}
	titleID := r.Job.Subject.ID
	fid, _ := r.Job.Params["library_folder_id"].(float64)
	apply, _ := r.Job.Params["apply"].(bool)
	db := s.Store.Read()
	plans, err := PlanMove(ctx, db, db, titleID, int64(fid))
	if err != nil {
		return err
	}
	files := 0
	for _, p := range plans {
		files += len(p.Steps)
	}
	if !apply {
		r.SetResult(map[string]any{"dry_run": true, "plans": plans, "files": files})
		s.Emit(ctx, &domain.Event{Actor: JobActor(r), Kind: domain.EventTitleMovePlanned, TitleID: &titleID, JobID: &r.Job.ID,
			Summary: fmt.Sprintf("Planned moving %d files to another Library folder (dry run)", files),
			Data:    map[string]any{"plans": plans}, GroupKey: GroupKey(titleID)})
		return nil
	}
	for _, p := range plans {
		if p.Blocked != "" {
			return jobError{code: "blocked", msg: p.Blocked}
		}
	}
	var lf mediafiles.LibraryFolder
	if err := database.Get(ctx, db, "library_folders", int64(fid), &lf); err != nil {
		return err
	}
	if h := CheckFolder(lf.Path, nil); !h.Usable() {
		return jobError{code: "folder_unusable", msg: (&FolderError{Health: h}).Error()}
	}
	moved := 0
	for _, p := range plans {
		for _, st := range p.Steps {
			if !importer.InsideReal(p.fromRoot, st.From) || !importer.InsideReal(p.toRoot, st.To) {
				return jobError{code: "blocked", msg: "a link leads a File outside its Library folder"}
			}
			if err := moveFile(st.From, st.To); err != nil {
				return fmt.Errorf("moving %s: %w", st.From, err)
			}
			moved++
			_ = r.Progress(ctx, float64(moved)/float64(max(files, 1)), "moved "+filepath.Base(st.To))
		}
		err := s.Store.Write(ctx, func(tx *sql.Tx) error {
			_, err := tx.Exec("UPDATE versions SET library_folder_id = ?, updated_at = ?, updated_by = ? WHERE id = ?",
				int64(fid), database.FormatTime(s.now()), ActorText(JobActor(r)), p.VersionID)
			return err
		})
		if err != nil {
			return err
		}
		if p.FromDir != p.fromRoot {
			// A show's season folders and an artist's album folders are
			// left empty too; the old folder goes only when nothing is in it.
			removeEmptyTree(p.FromDir)
		}
	}
	r.SetResult(map[string]any{"dry_run": false, "plans": plans, "files": moved})
	s.Emit(ctx, &domain.Event{Actor: JobActor(r), Kind: domain.EventTitleMoved, TitleID: &titleID, JobID: &r.Job.ID,
		Summary: fmt.Sprintf("Moved %d files to Library folder %s", moved, lf.Label), Data: map[string]any{"plans": plans},
		GroupKey: GroupKey(titleID)})
	return nil
}

// Move queues a move of a Title to another Library folder: a dry run, or
// the real move when apply is true.
func (s *Service) Move(ctx context.Context, titleID, folderID int64, apply bool, by domain.Actor) (int64, error) {
	if _, err := PlanMove(ctx, s.Store.Read(), s.Store.Read(), titleID, folderID); err != nil {
		return 0, err
	}
	id, _, err := s.Queue.Enqueue(ctx, KindMoveTitle, &domain.Subject{Type: "title", ID: titleID},
		map[string]any{"library_folder_id": folderID, "apply": apply}, moveKey(titleID, apply), by)
	return id, err
}

// jobError is a Job failure with a code.
// A final one is never tried again.
type jobError struct {
	code, msg string
	final     bool
}

func (e jobError) Error() string { return e.msg }
func (e jobError) Code() string  { return e.code }
func (e jobError) Final() bool   { return e.final }

// Hooks tests replace: the same-disk link, the cross-disk copy and the
// free-space check.
var (
	linkFile   = os.Link
	copyFile   = importer.CopyNoReplace
	checkSpace = importer.CheckSpace
	// caseCrash lets tests stop a case-only rename between its two steps.
	caseCrash func() error
)

// caseTempSuffix marks the file of a case-only rename between its two
// renames; moveFile finishes it on the next try.
const caseTempSuffix = ".holyarrs-case"

type preparedFileProof struct {
	value string
	info  os.FileInfo
	link  bool
}

// linkTarget returns where name points when name is a symlink, as an
// absolute path.
func linkTarget(name string) (string, os.FileInfo, bool) {
	fi, err := os.Lstat(name)
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		return "", nil, false
	}
	target, err := os.Readlink(name)
	if err != nil {
		return "", nil, false
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(name), target)
	}
	return target, fi, true
}

// moveLink moves a symlink by making it again at to and removing from. The
// file it points to is never opened: on a streaming mount that would read
// the whole file. It never replaces anything at to.
func moveLink(from, to, target string, src os.FileInfo) error {
	if dst, err := os.Lstat(to); err == nil {
		if os.SameFile(src, dst) {
			// One entry under two spellings (a case-only rename).
			return os.Rename(from, to)
		}
		if have, _, ok := linkTarget(to); ok && have == target {
			return os.Remove(from)
		}
		return fmt.Errorf("%s already exists and is a different file; HolyArrs never replaces it", to)
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o777); err != nil {
		return err
	}
	if err := os.Symlink(target, to); err != nil {
		return err
	}
	return os.Remove(from)
}

func prepareFileProof(name string) (preparedFileProof, error) {
	// A link's proof is where it points; its file is not read.
	if target, fi, ok := linkTarget(name); ok {
		return preparedFileProof{value: "link:" + target, info: fi, link: true}, nil
	}
	f, err := os.Open(name)
	if err != nil {
		return preparedFileProof{}, err
	}
	defer f.Close()
	before, err := f.Stat()
	if err != nil {
		return preparedFileProof{}, err
	}
	if !before.Mode().IsRegular() {
		return preparedFileProof{}, fmt.Errorf("%s is not a plain file", name)
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return preparedFileProof{}, err
	}
	after, err := f.Stat()
	if err != nil {
		return preparedFileProof{}, err
	}
	if !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return preparedFileProof{}, fmt.Errorf("%s changed while its rename proof was prepared", name)
	}
	return preparedFileProof{value: strconv.FormatInt(after.Size(), 10) + ":" + hex.EncodeToString(h.Sum(nil)), info: after}, nil
}

func fileProof(name string) (string, error) {
	p, err := prepareFileProof(name)
	return p.value, err
}

func proofSourceUnchanged(name string, p preparedFileProof) bool {
	st, err := os.Stat(name)
	if p.link {
		target, fi, ok := linkTarget(name)
		return ok && os.SameFile(p.info, fi) && p.value == "link:"+target
	}
	return err == nil && os.SameFile(p.info, st) && p.info.Size() == st.Size() && p.info.ModTime().Equal(st.ModTime())
}

// moveFile hardlinks from to to and removes from, or on another disk copies
// it through a temp file and then removes from. Neither ever replaces a file
// at to. A move cut off after the copy finishes cleanly on the next try: a
// target that is the same file, or has the same content, counts as ours.
func moveFile(from, to string) error {
	return moveFileWithProof(from, to, "")
}

// moveRecordedFile finishes a move whose exact source and destination were
// durably recorded before the disk changed. That record is the proof that an
// existing destination belongs to an interrupted move when the source is
// already gone.
func moveRecordedFile(from, to, proof string) error {
	return moveFileWithProof(from, to, proof)
}

func moveFileWithProof(from, to, proof string) error {
	if from == to {
		return nil
	}
	if target, fi, ok := linkTarget(from); ok {
		return moveLink(from, to, target, fi)
	}
	if strings.EqualFold(from, to) {
		return caseRename(from, to, proof)
	}
	return moveFileDistinctWithProof(from, to, proof)
}

// moveFileDistinct is moveFile for two names that differ by more than case.
func moveFileDistinct(from, to string) error {
	return moveFileDistinctWithProof(from, to, "")
}

func moveFileDistinctWithProof(from, to, proof string) error {
	src, err := os.Stat(from)
	if err != nil {
		if dst, derr := os.Stat(to); proof != "" && derr == nil && errors.Is(err, os.ErrNotExist) && dst.Mode().IsRegular() {
			got, ferr := fileProof(to)
			if ferr == nil && got == proof {
				return nil
			}
			return fmt.Errorf("%s is gone and %s does not match the recorded file", from, to)
		}
		return err
	}
	if !src.Mode().IsRegular() {
		return fmt.Errorf("%s is not a plain file", from)
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o777); err != nil {
		return err
	}
	if dst, err := os.Lstat(to); err == nil {
		if os.SameFile(src, dst) {
			// Two names of one file: from may go only when to is a real
			// second hard link, never when both names reach one entry.
			if n, err := linkCount(to); err != nil || n < 2 || !distinctEntries(from, to) {
				return fmt.Errorf("%s and %s are the same file under two names; HolyArrs leaves it as it is", from, to)
			}
			return os.Remove(from)
		}
		if dst.Mode().IsRegular() && importer.SameContent(from, to, src.Size()) {
			return os.Remove(from)
		}
		return fmt.Errorf("%s already exists and is a different file; HolyArrs never replaces it", to)
	}
	err = linkFile(from, to)
	if errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("%s appeared while moving; HolyArrs never replaces it", to)
	}
	if err != nil {
		if err := checkSpace(filepath.Dir(to), src.Size()); err != nil {
			return err
		}
		if err := copyFile(from, to); err != nil {
			return err
		}
	}
	return os.Remove(from)
}

// caseRename changes only the case of a name. On a case-insensitive disk
// from and to are one entry, so it renames through from+caseTempSuffix;
// a run cut off between the two renames is finished by the next one.
func caseRename(from, to, proof string) error {
	if fd, td := filepath.Dir(from), filepath.Dir(to); fd != td {
		ok, err := folderCase(fd, td)
		if err != nil {
			return err
		}
		if ok { // the folders now have the new case: rename the file in it
			from = filepath.Join(td, filepath.Base(from))
			if from == to {
				return nil
			}
		}
	}
	tmp := from + caseTempSuffix
	if _, err := os.Lstat(tmp); err == nil {
		if _, err := os.Lstat(to); err == nil {
			return fmt.Errorf("%s and %s both exist; HolyArrs leaves them for a look", tmp, to)
		}
		got, err := fileProof(tmp)
		if proof == "" || err != nil || got != proof {
			return fmt.Errorf("%s does not match the recorded case-only rename", tmp)
		}
		return os.Rename(tmp, to)
	}
	src, err := os.Lstat(from)
	if err != nil {
		if dst, derr := os.Stat(to); proof != "" && derr == nil && errors.Is(err, os.ErrNotExist) && dst.Mode().IsRegular() {
			got, ferr := fileProof(to)
			if ferr == nil && got == proof {
				return nil
			}
			return fmt.Errorf("%s is gone and %s does not match the recorded file", from, to)
		}
		return err
	}
	if !src.Mode().IsRegular() {
		return fmt.Errorf("%s is not a plain file", from)
	}
	dst, err := os.Lstat(to)
	if err != nil {
		// A case-sensitive disk: two different names, a plain move.
		return moveFileDistinctWithProof(from, to, proof)
	}
	if !os.SameFile(src, dst) {
		return fmt.Errorf("%s already exists and is a different file; HolyArrs never replaces it", to)
	}
	if ExactName(to) {
		return nil // already renamed
	}
	if err := os.Rename(from, tmp); err != nil {
		return err
	}
	if caseCrash != nil {
		if err := caseCrash(); err != nil {
			return err // as if the process died here: the next try finishes it
		}
	}
	if err := os.Rename(tmp, to); err != nil {
		if rerr := os.Rename(tmp, from); rerr != nil {
			return fmt.Errorf("%w; the file waits at %s and the next try finishes it", err, tmp)
		}
		return err
	}
	return nil
}

// folderCase gives the folder from (and each parent that differs) the case
// of to, when they are one folder on a case-insensitive disk; each folder
// goes through from+caseTempSuffix and a run cut off between the two
// renames is finished by the next one. It says false when from and to are
// different folders (a case-sensitive disk), and leaves them as they are.
func folderCase(from, to string) (bool, error) {
	if from == to {
		return true, nil
	}
	if filepath.Base(from) == filepath.Base(to) || !strings.EqualFold(filepath.Base(from), filepath.Base(to)) {
		// This folder's own name is right: fix the parents.
		ok, err := folderCase(filepath.Dir(from), filepath.Dir(to))
		return ok, err
	}
	pf, pt := filepath.Dir(from), filepath.Dir(to)
	if pf != pt {
		ok, err := folderCase(pf, pt)
		if err != nil || !ok {
			return ok, err
		}
		from = filepath.Join(pt, filepath.Base(from))
	}
	tmp := from + caseTempSuffix
	if fi, err := os.Lstat(tmp); err == nil && fi.IsDir() {
		if _, err := os.Lstat(to); err == nil {
			return false, fmt.Errorf("%s and %s both exist; HolyArrs leaves them for a look", tmp, to)
		}
		return true, os.Rename(tmp, to)
	}
	src, err := os.Lstat(from)
	if err != nil || !src.IsDir() {
		return false, nil
	}
	dst, err := os.Lstat(to)
	if err != nil || !os.SameFile(src, dst) {
		return false, nil // two folders: the file moves as any other
	}
	if ExactName(to) {
		return true, nil
	}
	if err := os.Rename(from, tmp); err != nil {
		return false, err
	}
	if err := os.Rename(tmp, to); err != nil {
		if rerr := os.Rename(tmp, from); rerr != nil {
			return false, fmt.Errorf("%w; the folder waits at %s and the next try finishes it", err, tmp)
		}
		return false, err
	}
	return true, nil
}

// distinctEntries proves that from and to are two directory entries, not
// one entry reached by two spellings (case, short names, a symlinked or
// junctioned parent). Parents that are different folders hold different
// entries; in one folder both names must be listed literally and differ.
func distinctEntries(from, to string) bool {
	pf, err1 := os.Stat(filepath.Dir(from))
	pt, err2 := os.Stat(filepath.Dir(to))
	if err1 != nil || err2 != nil {
		return false
	}
	if !os.SameFile(pf, pt) {
		return true
	}
	bf, bt := filepath.Base(from), filepath.Base(to)
	if strings.EqualFold(bf, bt) {
		return false
	}
	return ExactName(from) && ExactName(to)
}

// ExactName reports whether p's folder lists p's name with the same case.
func ExactName(p string) bool {
	ents, err := os.ReadDir(filepath.Dir(p))
	if err != nil {
		return false
	}
	for _, e := range ents {
		if e.Name() == filepath.Base(p) {
			return true
		}
	}
	return false
}

func moveKey(titleID int64, apply bool) string {
	if apply {
		return "move_title:" + strconv.FormatInt(titleID, 10) + ":apply"
	}
	return "move_title:" + strconv.FormatInt(titleID, 10) + ":dry"
}
