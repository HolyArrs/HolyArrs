package backups

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/files"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// migrationCopyRx matches the store's pinned pre-update copies
// (main-pre-update-0039-to-0040-<time>.db) and older builds' copies made
// before each migration (main-before-0039-<time>.db).
var migrationCopyRx = regexp.MustCompile(`^(main|events)-(?:before-(\d{4})|pre-update-(\d{4})-to-(\d{4}))-.*\.db$`)

type rawCopy struct {
	label    string
	path     string
	from, to int
	mod      time.Time
}

// AdoptMigrationCopies turns the raw database copies the store makes before
// an update into pre-update backups with a version file. The newest main
// and events copies go into one backup, so restoring it takes both back.
// A copy that does not match its sha256 file is left where it is and
// logged. It needs no open store (the restore command uses it too).
func (m *Manager) AdoptMigrationCopies() (int, error) {
	es, err := os.ReadDir(m.backupDir())
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var raws []rawCopy
	for _, e := range es {
		mm := migrationCopyRx.FindStringSubmatch(e.Name())
		if mm == nil || e.IsDir() {
			continue
		}
		p := filepath.Join(m.backupDir(), e.Name())
		if err := database.CheckBackupSum(p); err != nil {
			log.Printf("backup: not adopting %s: %v", e.Name(), err)
			continue
		}
		c := rawCopy{label: mm[1], path: p}
		if mm[2] != "" {
			c.to, _ = strconv.Atoi(mm[2])
			c.from = c.to - 1
		} else {
			c.from, _ = strconv.Atoi(mm[3])
			c.to, _ = strconv.Atoi(mm[4])
		}
		if fi, err := e.Info(); err == nil {
			c.mod = fi.ModTime()
		}
		raws = append(raws, c)
	}
	sort.SliceStable(raws, func(i, j int) bool { return raws[i].mod.After(raws[j].mod) })
	// The newest copy of each database together, every other copy on its
	// own; older groups first, so backup times keep their order.
	var groups [][]rawCopy
	var first []rawCopy
	seen := map[string]bool{}
	for i := len(raws) - 1; i >= 0; i-- {
		c := raws[i]
		newest := true
		for _, d := range raws[:i] {
			if d.label == c.label {
				newest = false
				break
			}
		}
		if newest && !seen[c.label] {
			seen[c.label] = true
			first = append(first, c)
			continue
		}
		groups = append(groups, []rawCopy{c})
	}
	if len(first) > 0 {
		groups = append(groups, first)
	}
	prev := m.lastVersion()
	n := 0
	for _, g := range groups {
		files := map[string]string{}
		var parts []string
		for _, c := range g {
			inner := database.MainFile
			if c.label == "events" {
				inner = database.EventsFile
			}
			files[inner] = c.path
			parts = append(parts, fmt.Sprintf("%s %d to %d", c.label, c.from, c.to))
		}
		sort.Strings(parts)
		edit := func(meta *Meta) {
			meta.SchemaMain, meta.SchemaEvents = 0, 0
			for _, c := range g {
				if c.label == "main" {
					meta.SchemaMain = c.from
				} else {
					meta.SchemaEvents = c.from
				}
			}
			if prev != "" && prev != m.Version {
				meta.Version, meta.FromVersion, meta.ToVersion = prev, prev, m.Version
				meta.Detail = fmt.Sprintf("before updating from %s to %s", prev, m.Version)
			} else {
				meta.Detail = "before updating the data (" + strings.Join(parts, ", ") + ")"
			}
		}
		if _, err := m.write(context.Background(), KindPreUpdate, files, edit); err != nil {
			return n, err
		}
		for _, c := range g {
			if err := os.Remove(c.path); err != nil {
				return n, err
			}
			_ = os.Remove(c.path + ".sha256")
			n++
		}
	}
	return n, nil
}

// lastVersion is the version of the build that ran before this one, when a
// store is open ("" otherwise).
func (m *Manager) lastVersion() string {
	if m.Store == nil {
		return ""
	}
	v, err := m.setting(context.Background(), settingVersion)
	if err != nil {
		return ""
	}
	return v
}

// AfterStart runs once the app started fine. It keeps the newest
// KeepPreUpdate pre-update backups (older ones go only now, after a good
// start). When this build's version differs from the last one that ran it
// makes an update backup and drops older update backups; pre-update
// backups are never dropped by that.
func (m *Manager) AfterStart(ctx context.Context) error {
	all, err := m.List()
	if err != nil {
		return err
	}
	kept, update := 0, false
	for _, x := range all {
		switch x.Kind {
		case KindPreUpdate:
			if kept++; kept > KeepPreUpdate {
				if err := os.Remove(x.Path); err != nil {
					log.Printf("backup: dropping old pre-update backup %s: %v", x.Name, err)
				}
			}
		case KindUpdate:
			// Only the newest update backup is kept, also when no update ran
			// since (older ones were otherwise only dropped by the next one).
			if update {
				if err := os.Remove(x.Path); err != nil {
					log.Printf("backup: dropping old update backup %s: %v", x.Name, err)
				}
			}
			update = true
		}
	}
	last, err := m.setting(ctx, settingVersion)
	if err != nil {
		return err
	}
	if last == m.Version {
		return nil
	}
	if last != "" {
		b, err := m.Create(ctx, KindUpdate)
		if err != nil {
			return fmt.Errorf("backup before update: %w", err)
		}
		log.Printf("backup: version %s -> %s, made %s", last, m.Version, b.Name)
		all, err := m.List()
		if err != nil {
			return err
		}
		for _, x := range all {
			if x.Kind == KindUpdate && x.CreatedAt.Before(b.CreatedAt) {
				_ = os.Remove(x.Path)
			}
		}
	}
	return m.setSetting(ctx, settingVersion, m.Version, domain.System)
}

// NewestPreUpdate returns the newest pre-update backup this build can
// restore. After going back more than one update (1.2 to 1.0) the newest
// one holds data from 1.1, which this build cannot read, so it is passed
// over for the one taken before the update from this build. When none
// fits, the newest is returned (and Stage says why it cannot be used).
func (m *Manager) NewestPreUpdate() (Info, error) {
	all, err := m.List()
	if err != nil {
		return Info{}, err
	}
	sm, se := database.SchemaVersions()
	var newest *Info
	for i, x := range all {
		if x.Kind != KindPreUpdate {
			continue
		}
		if newest == nil {
			newest = &all[i]
		}
		if meta, ok := readMeta(x.Path); ok && meta.SchemaMain <= sm && meta.SchemaEvents <= se {
			return x, nil
		}
	}
	if newest != nil {
		return *newest, nil
	}
	return Info{}, errors.New("there is no pre-update backup")
}

// StageNamed checks a backup from the backups folder and stages it, like an
// upload.
func (m *Manager) StageNamed(ctx context.Context, name string) error {
	f, err := m.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	return m.Stage(ctx, f)
}

func (m *Manager) path(name string) (string, error) {
	if _, _, ok := parseName(name); !ok || filepath.Base(name) != name {
		return "", errors.New("no such backup")
	}
	return filepath.Join(m.backupDir(), name), nil
}

// Delete removes one backup.
func (m *Manager) Delete(name string) error {
	p, err := m.path(name)
	if err != nil {
		return err
	}
	return os.Remove(p)
}

// Open opens one backup for download.
func (m *Manager) Open(name string) (*os.File, error) {
	p, err := m.path(name)
	if err != nil {
		return nil, err
	}
	return os.Open(p)
}

// MaxUpload is the largest restore zip accepted.
const MaxUpload = 2 << 30

// maxUnpacked caps all entries of a restore zip together.
var maxUnpacked int64 = MaxUpload

var allowed = map[string]bool{MetaFile: true, database.MainFile: true, database.EventsFile: true}

// Stage checks an uploaded backup and puts it in place for the next start.
// Nothing is staged unless every check passes.
func (m *Manager) Stage(_ context.Context, r io.Reader) (err error) {
	tmp, err := os.MkdirTemp(m.Dir, ".restore-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	zp := filepath.Join(tmp, "upload.zip")
	f, err := os.OpenFile(zp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, io.LimitReader(r, MaxUpload+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if n > MaxUpload {
		return errors.New("the backup is too large")
	}
	zr, err := zip.OpenReader(zp)
	if err != nil {
		return errors.New("this is not a zip file")
	}
	defer zr.Close()
	out := filepath.Join(tmp, "files")
	if err := os.Mkdir(out, 0o750); err != nil {
		return err
	}
	var meta *Meta
	got := map[string]bool{}
	var total int64
	for _, zf := range zr.File {
		if !allowed[zf.Name] || got[zf.Name] {
			return fmt.Errorf("the backup holds an unexpected entry %q", zf.Name)
		}
		got[zf.Name] = true
		rc, err := zf.Open()
		if err != nil {
			return err
		}
		dst := filepath.Join(out, zf.Name)
		w, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			rc.Close()
			return err
		}
		n, err := io.Copy(w, io.LimitReader(rc, maxUnpacked-total+1))
		total += n
		rc.Close()
		if cerr := w.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return fmt.Errorf("reading %s from the backup: %w", zf.Name, err)
		}
		if total > maxUnpacked {
			return errors.New("the backup unpacks too large")
		}
		if zf.Name == MetaFile {
			b, _ := os.ReadFile(dst)
			meta = &Meta{}
			if err := json.Unmarshal(b, meta); err != nil {
				return errors.New("backup.json is damaged")
			}
		}
	}
	if meta == nil || meta.App != "holyarrs" {
		return errors.New("this is not a HolyArrs backup")
	}
	if !got[database.MainFile] {
		return errors.New("the backup holds no database")
	}
	sm, se := database.SchemaVersions()
	if meta.SchemaMain > sm || meta.SchemaEvents > se {
		return fmt.Errorf("the backup comes from a newer HolyArrs (%s); update first", meta.Version)
	}
	for name := range got {
		if name == MetaFile {
			continue
		}
		if err := integrity(filepath.Join(out, name)); err != nil {
			return fmt.Errorf("%s in the backup is damaged: %w", name, err)
		}
	}
	if m.CheckKey != nil {
		if err := m.CheckKey(filepath.Join(out, database.MainFile)); err != nil {
			return fmt.Errorf("the backup's secrets cannot be read with this master key (moving to a new machine? see \"Move to a new machine\" in the HolyArrs docs, in docs/run/backups.md): %w", err)
		}
	}
	staged := filepath.Join(m.Dir, StagedDir)
	if err := os.RemoveAll(staged); err != nil {
		return err
	}
	if err := os.Rename(out, staged); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(staged, readyFile), []byte(meta.CreatedAt.Format(time.RFC3339)+"\n"), 0o600)
}

func integrity(path string) error {
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro")
	if err != nil {
		return err
	}
	defer db.Close()
	var res string
	if err := db.QueryRow("PRAGMA integrity_check").Scan(&res); err != nil {
		return err
	}
	if res != "ok" {
		return errors.New(res)
	}
	return nil
}

// Staged reports whether a restore waits for the next start.
func Staged(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, StagedDir, readyFile))
	return err == nil
}

// applyRename is os.Rename; tests replace it to exercise interrupted restore
// swaps without depending on platform-specific file locking.
var applyRename = os.Rename

// JournalFile, in the config folder, lists the moves of a restore in
// progress. If it is still there at the next start, the restore was cut
// off: it is finished, or undone, before the store opens.
const JournalFile = "restore.journal"

type journalMove struct {
	From string `json:"from"` // relative to the config folder
	To   string `json:"to"`
}

type journal struct {
	Keep  string        `json:"keep"`
	Moves []journalMove `json:"moves"`
}

func writeJournal(dir string, j journal) error {
	b, err := json.Marshal(j)
	if err != nil {
		return err
	}
	p := filepath.Join(dir, JournalFile)
	// A killed restore can leave the .tmp behind, and on Windows a file
	// another process just let go of can still refuse to open or rename for
	// a moment, so a stale one is removed and sharing errors are retried.
	deadline := time.Now().Add(2 * time.Second)
	for {
		if err := os.Remove(p + ".tmp"); err != nil && !errors.Is(err, os.ErrNotExist) && !sharingErr(err) {
			return err
		}
		err := files.WriteFileSynced(p, b, 0o600)
		if err == nil || !sharingErr(err) || time.Now().After(deadline) {
			return err
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// sharingErr reports errors Windows gives while another handle or mapping
// still holds a file (sharing violation, lock violation, access denied,
// user-mapped section).
func sharingErr(err error) bool {
	if err == nil || runtime.GOOS != "windows" {
		return false
	}
	var en syscall.Errno
	if errors.As(err, &en) {
		switch en {
		case 5, 32, 33, 1224: // access denied, sharing, lock, user-mapped section
			return true
		}
	}
	return errors.Is(err, os.ErrPermission)
}

func removeJournal(dir string) error {
	if err := os.Remove(filepath.Join(dir, JournalFile)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return files.SyncDir(dir)
}

func syncMoveDirs(dir string, j journal) error {
	seen := map[string]bool{}
	for _, m := range j.Moves {
		for _, p := range []string{m.From, m.To} {
			d := filepath.Dir(filepath.Join(dir, p))
			if seen[d] {
				continue
			}
			seen[d] = true
			// A folder already gone (restore-staged removed just before a
			// cut-off) has nothing left to sync.
			if err := files.SyncDir(d); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}
	return nil
}

func present(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// undoMoves puts back moves that were done, newest first.
func undoMoves(dir string, done []journalMove) error {
	var errs []error
	for i := len(done) - 1; i >= 0; i-- {
		if err := applyRename(filepath.Join(dir, done[i].To), filepath.Join(dir, done[i].From)); err != nil {
			errs = append(errs, fmt.Errorf("putting %s back: %w", done[i].From, err))
		}
	}
	return errors.Join(errs...)
}

// donePrefix counts the moves done before a stop. Moves run in order, and a
// later move can fill a name an earlier one emptied (the restored database
// takes the old one's name), so each count is played through from the start
// and the one that best fits the files on disk wins (the first on a tie).
func donePrefix(dir string, j journal) int {
	// A name is there at the start when its first use is as a move's source.
	start := map[string]bool{}
	for _, m := range j.Moves {
		if _, seen := start[m.From]; !seen {
			start[m.From] = true
		}
		if _, seen := start[m.To]; !seen {
			start[m.To] = false
		}
	}
	real := map[string]bool{}
	state := map[string]bool{}
	for name, v := range start {
		real[name] = present(filepath.Join(dir, name))
		state[name] = v
	}
	best, bestOff := 0, -1
	for k := 0; k <= len(j.Moves); k++ {
		if k > 0 {
			state[j.Moves[k-1].From], state[j.Moves[k-1].To] = false, true
		}
		off := 0
		for name, want := range state {
			if real[name] != want {
				off++
			}
		}
		if bestOff < 0 || off < bestOff {
			best, bestOff = k, off
		}
	}
	return best
}

// allMoved reports whether every move's new name is there.
func allMoved(dir string, j journal) bool {
	for _, m := range j.Moves {
		if !present(filepath.Join(dir, m.To)) {
			return false
		}
	}
	return true
}

// recoverJournal finishes a restore that was cut off, or undoes it when it
// cannot be finished, so the live files are never a mix of old and new.
func recoverJournal(dir string) error {
	b, err := os.ReadFile(filepath.Join(dir, JournalFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var j journal
	if err := json.Unmarshal(b, &j); err != nil || len(j.Moves) == 0 {
		// Written through a synced rename, so a bad journal was never used.
		log.Printf("backup: ignoring an unreadable %s", JournalFile)
		return removeJournal(dir)
	}
	// Cut off after restore-staged was removed: every move was done and
	// synced, so only the journal is left to clear.
	if !present(filepath.Join(dir, StagedDir)) && allMoved(dir, j) {
		log.Printf("backup: a restore was cut off after it had finished; the replaced databases are in %s", filepath.Join(dir, j.Keep))
		return removeJournal(dir)
	}
	done := donePrefix(dir, j)
	forward := func() error {
		for _, m := range j.Moves[done:] {
			from, to := filepath.Join(dir, m.From), filepath.Join(dir, m.To)
			if !present(from) || present(to) {
				return fmt.Errorf("%s -> %s: the files are not where the journal expects", m.From, m.To)
			}
			if err := applyRename(from, to); err != nil {
				return err
			}
			done++
		}
		return nil
	}
	if err := forward(); err != nil {
		log.Printf("backup: a restore was cut off and cannot be finished (%v); undoing it", err)
		if uerr := undoMoves(dir, j.Moves[:done]); uerr != nil {
			return fmt.Errorf("a restore was cut off and could not be undone: %w", errors.Join(err, uerr))
		}
		if err := syncMoveDirs(dir, j); err != nil {
			return err
		}
		log.Printf("backup: the cut-off restore was undone; the data from before it is back in place")
		if j.Keep != "" {
			_ = os.Remove(filepath.Join(dir, j.Keep)) // only when empty
		}
		return removeJournal(dir)
	}
	if err := syncMoveDirs(dir, j); err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(dir, StagedDir)); err != nil {
		return err
	}
	log.Printf("backup: a restore was cut off and has been finished; the replaced databases are in %s", filepath.Join(dir, j.Keep))
	return removeJournal(dir)
}

// ApplyStaged swaps a staged restore in. Call it before the store opens.
// The databases it replaces are kept in backups/before-restore-<time>. The
// moves are written to a journal first, so a restore cut off part way is
// finished (or undone) on the next call.
func ApplyStaged(dir string) (bool, error) {
	if err := recoverJournal(dir); err != nil {
		return false, err
	}
	staged := filepath.Join(dir, StagedDir)
	if !Staged(dir) {
		if _, err := os.Stat(staged); err == nil {
			_ = os.RemoveAll(staged) // half-written: never use it
		}
		return false, nil
	}
	keepRel := filepath.Join("backups", "before-restore-"+time.Now().UTC().Format(stampLayout))
	keep := filepath.Join(dir, keepRel)
	if err := os.MkdirAll(keep, 0o750); err != nil {
		return false, err
	}
	j := journal{Keep: keepRel}
	for _, db := range []string{database.MainFile, database.EventsFile} {
		if _, err := os.Stat(filepath.Join(staged, db)); err != nil {
			continue // not in the backup: keep the current one
		}
		for _, suffix := range []string{"", "-wal", "-shm"} {
			if present(filepath.Join(dir, db+suffix)) {
				j.Moves = append(j.Moves, journalMove{From: db + suffix, To: filepath.Join(keepRel, db+suffix)})
			}
		}
		j.Moves = append(j.Moves, journalMove{From: filepath.Join(StagedDir, db), To: db})
	}
	if err := writeJournal(dir, j); err != nil {
		return false, err
	}
	for i, m := range j.Moves {
		if err := applyRename(filepath.Join(dir, m.From), filepath.Join(dir, m.To)); err != nil {
			// Put staged databases back too, so the restore stays retryable.
			if uerr := undoMoves(dir, j.Moves[:i]); uerr != nil {
				return false, errors.Join(err, uerr)
			}
			_ = os.Remove(keep)
			if rerr := removeJournal(dir); rerr != nil {
				return false, errors.Join(err, rerr)
			}
			return false, err
		}
	}
	if err := syncMoveDirs(dir, j); err != nil {
		return true, err
	}
	if err := os.RemoveAll(staged); err != nil {
		return true, err
	}
	if err := removeJournal(dir); err != nil {
		return true, err
	}
	log.Printf("backup: restored the staged backup; the replaced databases are in %s", keep)
	return true, nil
}
