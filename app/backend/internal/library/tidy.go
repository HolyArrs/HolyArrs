package library

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// Tidy my folders: loose movie files, the ones a scan leaves out,
// each get a folder of their own at the top of the Library folder. The
// preview moves nothing; a tidy moves only what the preview showed, and
// the last tidy of a Library folder can be undone.

// TidyFile is one file a tidy moves, as paths inside the Library folder.
type TidyFile struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// TidyMove is one loose movie: its video and the files named after it
// (subtitles, .nfo, pictures). Problem says why it cannot move.
type TidyMove struct {
	TidyFile
	Name    string     `json:"name"`
	Year    int        `json:"year"`
	Files   []TidyFile `json:"files"`
	Problem string     `json:"problem,omitempty"`
	// Why is the kind of problem: exists, two or has_movie; Path is the
	// file or folder it is about.
	Why  string `json:"why,omitempty"`
	Path string `json:"path,omitempty"`
}

// TidyUndo is the last tidy of a Library folder that can still be undone.
type TidyUndo struct {
	At      time.Time  `json:"at"`
	Moved   []TidyFile `json:"moved"`
	Folders []string   `json:"folders"`
}

// TidyPlan is the preview of a tidy.
type TidyPlan struct {
	LibraryFolderID int64      `json:"library_folder_id"`
	Moves           []TidyMove `json:"moves"`
	Blocked         []TidyMove `json:"blocked"`
	Unmatched       []string   `json:"unmatched"`
	Hash            string     `json:"hash"`
	// UndoFiles is how many files the last tidy moved that Undo puts back.
	UndoFiles int `json:"undo_files"`
}

// TidyResult says what a tidy or an undo did.
type TidyResult struct {
	Moved  int           `json:"moved"`
	Failed []TidyFailure `json:"failed"`
}

// TidyFailure is one file that did not move, and why.
type TidyFailure struct {
	From string `json:"from"`
	Why  string `json:"why"`
}

// ErrTidyChanged says the folder changed since the preview.
var ErrTidyChanged = errors.New("the folder changed since the preview; look at the list again")

// ErrNothingToUndo says no tidy of this Library folder can be undone.
var ErrNothingToUndo = errors.New("there is no tidy to undo")

// The undo of a tidy is kept with the other where-files-go settings.
const tidyGroup, tidyKeyPrefix = "where_files_go", "tidy_undo_"

var tidyMu sync.Mutex

// LooseMovieCount counts the loose movies in the folders movieFoldersLoose
// named.
func LooseMovieCount(root string, folders []string) int {
	n := 0
	for _, f := range folders {
		ents, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(f)))
		if err == nil {
			n += looseMovies(ents)
		}
	}
	return n
}

// PlanTidy works out where every loose movie of a movie Library folder
// would go: Title (Year)/Title (Year).ext inside the subfolder it already
// sits in (at the top for files loose there), read from the file name.
// Nothing on disk changes.
func PlanTidy(ctx context.Context, db Reader, folderID int64) (TidyPlan, error) {
	var lf mediafiles.LibraryFolder
	if err := database.Get(ctx, db, "library_folders", folderID, &lf); err != nil {
		return TidyPlan{}, ErrNoFolder
	}
	if lf.Kind != "movie" {
		return TidyPlan{}, ErrNotMovieFolder
	}
	if lf.LookOnly {
		return TidyPlan{}, mediafiles.ErrLookOnly
	}
	// A movie already in the Library as a loose file keeps its place: Tidy
	// moves only files no movie owns yet. A flat folder keeps the files at
	// its top as they are.
	owned, err := looseKnown(ctx, db, lf.ID, FoldsCase(lf.Path))
	if err != nil {
		return TidyPlan{}, err
	}
	p := planTidy(lf.Path, owned, lf.Flat)
	p.LibraryFolderID = lf.ID
	if u, ok := readTidyUndo(ctx, db, lf.ID); ok {
		p.UndoFiles = len(u.Moved)
	}
	return p, nil
}

// A movie always gets its folder inside the subfolder it was in, so the
// person's own genre folders stay, whether Group by genre is on or off.
func planTidy(root string, owned map[string]int64, flat bool) TidyPlan {
	fold := FoldsCase(root)
	p := TidyPlan{Moves: []TidyMove{}, Blocked: []TidyMove{}, Unmatched: []string{}}
	ents, err := os.ReadDir(root)
	if err != nil {
		return p
	}
	_, loose := movieFoldersLoose(root, ents)
	taken := map[string]bool{}
	for _, rel := range loose {
		if flat && rel == "" {
			continue
		}
		ents, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			continue
		}
		var videos, others []string
		for _, e := range ents {
			if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			if videoExt[strings.ToLower(filepath.Ext(e.Name()))] {
				videos = append(videos, e.Name())
			} else {
				others = append(others, e.Name())
			}
		}
		owner := companionsOf(videos, others)
		// The plainest name first, so a movie's cuts keep their own names.
		sort.SliceStable(videos, func(i, j int) bool { return len(videos[i]) < len(videos[j]) })
		for _, v := range videos {
			base := strings.TrimSuffix(v, filepath.Ext(v))
			from := path.Join(rel, v)
			if _, ok := owned[FolderKey(from, fold)]; ok {
				continue
			}
			name, year := FolderName(base)
			if year == 0 {
				p.Unmatched = append(p.Unmatched, from)
				continue
			}
			if !strings.Contains(name, " ") {
				name = strings.Join(strings.FieldsFunc(name, func(r rune) bool { return r == '.' || r == '_' }), " ")
			}
			newBase := importer.FolderName(name, year)
			folder := path.Join(rel, newBase)
			to := path.Join(folder, newBase+filepath.Ext(v))
			// A second cut of the same movie keeps its own name in the folder.
			if taken[strings.ToLower(to)] {
				newBase, to = base, path.Join(folder, v)
			}
			m := TidyMove{TidyFile: TidyFile{From: from, To: to}, Name: name, Year: year, Files: []TidyFile{}}
			for _, o := range owner[v] {
				m.Files = append(m.Files, TidyFile{From: path.Join(rel, o), To: path.Join(folder, newBase+o[len(base):])})
			}
			m.Why, m.Path, m.Problem = tidyProblem(root, folder, m, taken)
			if m.Problem != "" {
				p.Blocked = append(p.Blocked, m)
				continue
			}
			for _, f := range append([]TidyFile{m.TidyFile}, m.Files...) {
				taken[strings.ToLower(f.To)] = true
			}
			taken[strings.ToLower(folder)+"/"] = true
			p.Moves = append(p.Moves, m)
		}
	}
	p.Hash = tidyHash(p.Moves)
	return p
}

// companionsOf gives each video the other files named after it; a file
// matching two videos goes to the one with the longer name.
func companionsOf(videos, others []string) map[string][]string {
	byLen := append([]string(nil), videos...)
	sort.Slice(byLen, func(i, j int) bool { return len(byLen[i]) > len(byLen[j]) })
	out := map[string][]string{}
	for _, o := range others {
		for _, v := range byLen {
			base := strings.TrimSuffix(v, filepath.Ext(v))
			if strings.HasPrefix(o, base+".") || strings.HasPrefix(o, base+"-") {
				out[v] = append(out[v], o)
				break
			}
		}
	}
	return out
}

// tidyProblem says why a move cannot happen: a file is already at a new
// name, or a folder of that name already holds a movie not from this tidy.
func tidyProblem(root, folder string, m TidyMove, taken map[string]bool) (why, at, text string) {
	for _, f := range append([]TidyFile{m.TidyFile}, m.Files...) {
		if taken[strings.ToLower(f.To)] {
			return "two", f.To, f.To + " would be used by two files"
		}
		if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(f.To))); err == nil {
			return "exists", f.To, f.To + " already exists"
		}
	}
	if !taken[strings.ToLower(folder)+"/"] && videoIn(filepath.Join(root, filepath.FromSlash(folder))) {
		return "has_movie", folder, "the folder " + folder + " already holds a movie"
	}
	return "", "", ""
}

func tidyHash(moves []TidyMove) string {
	h := sha256.New()
	for _, m := range moves {
		fmt.Fprintf(h, "\n%s\x00%s", m.From, m.To)
		for _, f := range m.Files {
			fmt.Fprintf(h, "\n\x01%s\x00%s", f.From, f.To)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

// ApplyTidy moves the files of the preview with this hash, each with the
// Library's safe move (a hardlink, or a copy across disks; never replacing
// a file), and keeps what it moved so Undo can put it back.
func ApplyTidy(ctx context.Context, st *database.Store, folderID int64, hash string, by database.Actor) (TidyResult, error) {
	tidyMu.Lock()
	defer tidyMu.Unlock()
	p, err := PlanTidy(ctx, st.Read(), folderID)
	if err != nil {
		return TidyResult{}, err
	}
	if hash == "" || hash != p.Hash {
		return TidyResult{}, ErrTidyChanged
	}
	var lf mediafiles.LibraryFolder
	if err := database.Get(ctx, st.Read(), "library_folders", folderID, &lf); err != nil {
		return TidyResult{}, ErrNoFolder
	}
	res := TidyResult{Failed: []TidyFailure{}}
	undo := TidyUndo{At: time.Now().UTC(), Moved: []TidyFile{}, Folders: []string{}}
	abs := func(rel string) string { return filepath.Join(lf.Path, filepath.FromSlash(rel)) }
	for _, m := range p.Moves {
		folder := path.Dir(m.To)
		if _, err := os.Stat(abs(folder)); errors.Is(err, os.ErrNotExist) {
			undo.Folders = append(undo.Folders, folder)
		}
		// The video first: its extras follow only when it moved.
		if err := moveFile(abs(m.From), abs(m.To)); err != nil {
			res.Failed = append(res.Failed, TidyFailure{From: m.From, Why: err.Error()})
			continue
		}
		undo.Moved = append(undo.Moved, m.TidyFile)
		res.Moved++
		for _, f := range m.Files {
			if err := moveFile(abs(f.From), abs(f.To)); err != nil {
				res.Failed = append(res.Failed, TidyFailure{From: f.From, Why: err.Error()})
				continue
			}
			undo.Moved = append(undo.Moved, f)
			res.Moved++
		}
	}
	if len(undo.Moved) > 0 {
		if err := writeTidyUndo(ctx, st, folderID, &undo, by); err != nil {
			return res, err
		}
	}
	return res, nil
}

// UndoTidy puts the files of the last tidy back where they were and
// removes the folders it made once they are empty. A file whose old place
// is taken stays and is listed; Undo can be tried again for it.
func UndoTidy(ctx context.Context, st *database.Store, folderID int64, by database.Actor) (TidyResult, error) {
	tidyMu.Lock()
	defer tidyMu.Unlock()
	var lf mediafiles.LibraryFolder
	if err := database.Get(ctx, st.Read(), "library_folders", folderID, &lf); err != nil {
		return TidyResult{}, ErrNoFolder
	}
	if lf.LookOnly {
		return TidyResult{}, mediafiles.ErrLookOnly
	}
	u, ok := readTidyUndo(ctx, st.Read(), folderID)
	if !ok {
		return TidyResult{}, ErrNothingToUndo
	}
	abs := func(rel string) string { return filepath.Join(lf.Path, filepath.FromSlash(rel)) }
	res := TidyResult{Failed: []TidyFailure{}}
	left := TidyUndo{At: u.At, Moved: []TidyFile{}, Folders: u.Folders}
	for i := len(u.Moved) - 1; i >= 0; i-- {
		f := u.Moved[i]
		if !tidyRelOK(f.From) || !tidyRelOK(f.To) {
			continue
		}
		if _, err := os.Lstat(abs(f.From)); err == nil {
			res.Failed = append(res.Failed, TidyFailure{From: f.To, Why: f.From + " already exists; HolyArrs never replaces it"})
			left.Moved = append([]TidyFile{f}, left.Moved...)
			continue
		}
		if err := moveFile(abs(f.To), abs(f.From)); err != nil {
			res.Failed = append(res.Failed, TidyFailure{From: f.To, Why: err.Error()})
			left.Moved = append([]TidyFile{f}, left.Moved...)
			continue
		}
		res.Moved++
	}
	for _, d := range u.Folders {
		if tidyRelOK(d) {
			_ = os.Remove(abs(d)) // only when empty
		}
	}
	var keep *TidyUndo
	if len(left.Moved) > 0 {
		keep = &left
	}
	return res, writeTidyUndo(ctx, st, folderID, keep, by)
}

// tidyRelOK says a kept path stays inside the Library folder.
func tidyRelOK(rel string) bool {
	return rel != "" && !strings.Contains(rel, `\`) && filepath.IsLocal(filepath.FromSlash(rel))
}

func readTidyUndo(ctx context.Context, db Reader, folderID int64) (TidyUndo, bool) {
	v, err := database.Setting(ctx, db, tidyGroup, tidyKeyPrefix+strconv.FormatInt(folderID, 10))
	if err != nil || v == "" {
		return TidyUndo{}, false
	}
	var u TidyUndo
	if json.Unmarshal([]byte(v), &u) != nil || len(u.Moved) == 0 {
		return TidyUndo{}, false
	}
	return u, true
}

// writeTidyUndo keeps u as the folder's undo; nil forgets it.
func writeTidyUndo(ctx context.Context, st *database.Store, folderID int64, u *TidyUndo, by database.Actor) error {
	key := tidyKeyPrefix + strconv.FormatInt(folderID, 10)
	return st.Write(ctx, func(tx *sql.Tx) error {
		if u == nil {
			_, err := tx.Exec("DELETE FROM settings WHERE grp = ? AND key = ?", tidyGroup, key)
			return err
		}
		b, err := json.Marshal(u)
		if err != nil {
			return err
		}
		return database.PutSetting(tx, tidyGroup, key, string(b), by, time.Now())
	})
}
