// Package backup makes and restores backups (design/domain/backups.md): a zip
// with a safe copy of the databases (which hold the settings) and a small
// version and date file. It never holds media, downloads, cached pictures or
// the master key.
package backup

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
	"sort"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// Kinds of backup.
const (
	KindScheduled = "scheduled"
	KindManual    = "manual" // kept until the person deletes it
	KindUpdate    = "update" // kept until the next update starts fine
)

// Preset is a ready-made schedule and keep rule.
type Preset string

// The three presets offered in the first-run setup.
const (
	Balanced Preset = "balanced"
	Light    Preset = "light"
	Careful  Preset = "careful"
)

// Rule says how often a preset backs up and what it keeps.
type Rule struct {
	Every   time.Duration
	Nightly int           // newest scheduled backups always kept
	Weekly  int           // plus the newest backup of this many earlier weeks
	MaxAge  time.Duration // or: keep everything younger than this
}

// Rules for each preset.
var Rules = map[Preset]Rule{
	Balanced: {Every: 24 * time.Hour, Nightly: 7, Weekly: 4},
	Light:    {Every: 7 * 24 * time.Hour, MaxAge: 28 * 24 * time.Hour},
	Careful:  {Every: 24 * time.Hour, MaxAge: 30 * 24 * time.Hour},
}

// MetaFile is the version and date file inside every backup.
const MetaFile = "backup.json"

// StagedDir holds a checked restore until the next start.
const StagedDir = "restore-staged"

const readyFile = "READY"

// KeyNote is written into every backup and shown in the docs.
const KeyNote = "The master key (holyarrs.key, or the file named by HOLYARRS_MASTER_KEY_FILE) is not in this backup. " +
	"Keep a copy of it somewhere safe: without it the passwords and keys stored in this backup cannot be read."

// Meta is the content of backup.json.
type Meta struct {
	App          string    `json:"app"`
	Version      string    `json:"version"`
	SchemaMain   int       `json:"schema_main"`
	SchemaEvents int       `json:"schema_events"`
	Kind         string    `json:"kind"`
	CreatedAt    time.Time `json:"created_at"`
	Note         string    `json:"note"`
}

// Manager makes, lists, prunes and restores backups.
type Manager struct {
	Store   *store.Store
	Dir     string // the config folder
	Version string
	Now     func() time.Time
	// CheckKey, when set, is asked whether a database file can be read with
	// the current master key, before a restore is staged.
	CheckKey func(dbPath string) error

	mu      sync.Mutex
	lastErr string // the last scheduled backup's error, "" after a good one
}

// State is what the health check needs: the newest backup's time (zero when
// there is none), how often scheduled backups are made, and the last
// scheduled backup's error.
func (m *Manager) State(ctx context.Context) (time.Time, time.Duration, string, error) {
	m.mu.Lock()
	last := m.lastErr
	m.mu.Unlock()
	p, err := m.Preset(ctx)
	if err != nil {
		return time.Time{}, 0, last, err
	}
	all, err := m.List()
	if err != nil || len(all) == 0 {
		return time.Time{}, Rules[p].Every, last, err
	}
	return all[0].CreatedAt, Rules[p].Every, last, nil
}

func (m *Manager) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

func (m *Manager) backupDir() string { return filepath.Join(m.Dir, "backups") }

const stampLayout = "20060102T150405.000Z"

var nameRx = regexp.MustCompile(`^holyarrs-backup-(scheduled|manual|update)-(\d{8}T\d{6}\.\d{3}Z)\.zip$`)

func fileName(kind string, at time.Time) string {
	return "holyarrs-backup-" + kind + "-" + at.UTC().Format(stampLayout) + ".zip"
}

// Info is one backup file.
type Info struct {
	Name      string    `json:"name"`
	Kind      string    `json:"kind"`
	CreatedAt time.Time `json:"created_at"`
	Size      int64     `json:"size"`
	Path      string    `json:"-"`
}

func parseName(n string) (string, time.Time, bool) {
	mm := nameRx.FindStringSubmatch(n)
	if mm == nil {
		return "", time.Time{}, false
	}
	t, err := time.Parse(stampLayout, mm[2])
	return mm[1], t, err == nil
}

// List returns the backups, newest first.
func (m *Manager) List() ([]Info, error) {
	es, err := os.ReadDir(m.backupDir())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Info
	for _, e := range es {
		kind, at, ok := parseName(e.Name())
		if !ok || e.IsDir() {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, Info{Name: e.Name(), Kind: kind, CreatedAt: at, Size: fi.Size(), Path: filepath.Join(m.backupDir(), e.Name())})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

// Create makes a backup of the running databases.
func (m *Manager) Create(_ context.Context, kind string) (Info, error) {
	tmp, err := os.MkdirTemp(m.Dir, ".backup-")
	if err != nil {
		return Info{}, err
	}
	defer os.RemoveAll(tmp)
	mainCopy, evCopy, err := m.Store.Snapshot(tmp)
	if err != nil {
		return Info{}, err
	}
	return m.write(kind, map[string]string{store.MainFile: mainCopy, store.EventsFile: evCopy})
}

// write zips the files with a backup.json into the backups folder.
func (m *Manager) write(kind string, files map[string]string) (Info, error) {
	if err := os.MkdirAll(m.backupDir(), 0o750); err != nil {
		return Info{}, err
	}
	at := m.now().UTC().Truncate(time.Millisecond)
	for { // never replace an existing backup
		if _, err := os.Stat(filepath.Join(m.backupDir(), fileName(kind, at))); errors.Is(err, os.ErrNotExist) {
			break
		}
		at = at.Add(time.Millisecond)
	}
	sm, se := store.SchemaVersions()
	meta := Meta{App: "holyarrs", Version: m.Version, SchemaMain: sm, SchemaEvents: se, Kind: kind, CreatedAt: at, Note: KeyNote}
	name := fileName(kind, at)
	final := filepath.Join(m.backupDir(), name)
	part := final + ".part"
	f, err := os.OpenFile(part, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return Info{}, err
	}
	ok := false
	defer func() {
		if !ok {
			_ = f.Close()
			_ = os.Remove(part)
		}
	}()
	zw := zip.NewWriter(f)
	mw, err := zw.Create(MetaFile)
	if err != nil {
		return Info{}, err
	}
	enc := json.NewEncoder(mw)
	enc.SetIndent("", "  ")
	if err := enc.Encode(meta); err != nil {
		return Info{}, err
	}
	keys := make([]string, 0, len(files))
	for k := range files {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if err := addFile(zw, k, files[k]); err != nil {
			return Info{}, err
		}
	}
	if err := zw.Close(); err != nil {
		return Info{}, err
	}
	if err := f.Sync(); err != nil {
		return Info{}, err
	}
	if err := f.Close(); err != nil {
		return Info{}, err
	}
	if err := os.Rename(part, final); err != nil {
		return Info{}, err
	}
	ok = true
	fi, _ := os.Stat(final)
	return Info{Name: name, Kind: kind, CreatedAt: at, Size: fi.Size(), Path: final}, nil
}

func addFile(zw *zip.Writer, name, path string) error {
	src, err := os.Open(path)
	if err != nil {
		return err
	}
	defer src.Close()
	w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
	if err != nil {
		return err
	}
	_, err = io.Copy(w, src)
	return err
}

// Prune deletes scheduled backups the preset no longer keeps. Manual
// backups are never touched; update backups are handled by AfterStart.
func (m *Manager) Prune(p Preset) error {
	rule, ok := Rules[p]
	if !ok {
		return fmt.Errorf("unknown preset %q", p)
	}
	all, err := m.List()
	if err != nil {
		return err
	}
	var sched []Info
	for _, b := range all {
		if b.Kind == KindScheduled {
			sched = append(sched, b)
		}
	}
	keep := map[string]bool{}
	if len(sched) > 0 {
		keep[sched[0].Name] = true // never drop the newest
	}
	if rule.MaxAge > 0 {
		for _, b := range sched {
			if m.now().Sub(b.CreatedAt) <= rule.MaxAge {
				keep[b.Name] = true
			}
		}
	}
	for i, b := range sched {
		if i < rule.Nightly {
			keep[b.Name] = true
		}
	}
	if rule.Weekly > 0 && len(sched) > rule.Nightly {
		weeks := map[string]bool{}
		for _, b := range sched[rule.Nightly:] { // newest first, so the first per week is its newest
			y, w := b.CreatedAt.ISOWeek()
			wk := fmt.Sprintf("%d-%02d", y, w)
			if weeks[wk] {
				continue
			}
			if len(weeks) == rule.Weekly {
				break
			}
			weeks[wk] = true
			keep[b.Name] = true
		}
	}
	var errs []error
	for _, b := range sched {
		if !keep[b.Name] {
			errs = append(errs, os.Remove(b.Path))
		}
	}
	return errors.Join(errs...)
}

const (
	settingPreset  = "backup_preset"
	settingVersion = "app_version"
)

func (m *Manager) setting(ctx context.Context, key string) (string, error) {
	var v string
	err := m.Store.Read().QueryRowContext(ctx, "SELECT value FROM settings WHERE grp = 'this_app' AND key = ?", key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

func (m *Manager) setSetting(ctx context.Context, key, val string, by domain.Actor) error {
	who, _ := json.Marshal(by)
	t := store.FormatTime(m.now())
	return m.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO settings(grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('this_app', ?, ?, ?, ?, ?, ?)
			ON CONFLICT(grp, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at, updated_by = excluded.updated_by`,
			key, val, t, string(who), t, string(who))
		return err
	})
}

// Preset returns the chosen preset; Balanced when none was chosen.
func (m *Manager) Preset(ctx context.Context) (Preset, error) {
	v, err := m.setting(ctx, settingPreset)
	if err != nil || v == "" {
		return Balanced, err
	}
	if _, ok := Rules[Preset(v)]; !ok {
		return Balanced, nil
	}
	return Preset(v), nil
}

// SetPreset chooses a preset.
func (m *Manager) SetPreset(ctx context.Context, p Preset, by domain.Actor) error {
	if _, ok := Rules[p]; !ok {
		return fmt.Errorf("unknown preset %q; pick balanced, light or careful", p)
	}
	return m.setSetting(ctx, settingPreset, string(p), by)
}

// due says whether a scheduled backup should be made now.
func (m *Manager) due(p Preset) bool {
	all, err := m.List()
	if err != nil {
		return false
	}
	for _, b := range all {
		if b.Kind == KindScheduled {
			// a little slack so a nightly run does not drift later each day
			return m.now().Sub(b.CreatedAt) >= Rules[p].Every-30*time.Minute
		}
	}
	return true
}

// Tick makes a scheduled backup when one is due and prunes old ones.
func (m *Manager) Tick(ctx context.Context) error {
	p, err := m.Preset(ctx)
	if err != nil {
		return err
	}
	if !m.due(p) {
		return nil
	}
	b, err := m.Create(ctx, KindScheduled)
	if err != nil {
		return err
	}
	log.Printf("backup: made %s (%d bytes)", b.Name, b.Size)
	return m.Prune(p)
}

// Pass checks once whether a backup is due (the supervisor runs it every
// ten minutes) and keeps the outcome for the health check.
func (m *Manager) Pass(ctx context.Context) error {
	err := m.Tick(ctx)
	if err != nil && ctx.Err() == nil {
		log.Printf("backup: %v", err)
	}
	if ctx.Err() == nil {
		m.mu.Lock()
		m.lastErr = ""
		if err != nil {
			m.lastErr = err.Error()
		}
		m.mu.Unlock()
	}
	return err
}

var migrationCopyRx = regexp.MustCompile(`^(main|events)-before-\d{4}-.*\.db$`)

// AdoptMigrationCopies turns the raw database copies the store makes before
// each migration into update backups with a version file.
func (m *Manager) AdoptMigrationCopies() (int, error) {
	es, err := os.ReadDir(m.backupDir())
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range es {
		mm := migrationCopyRx.FindStringSubmatch(e.Name())
		if mm == nil || e.IsDir() {
			continue
		}
		inner := store.MainFile
		if mm[1] == "events" {
			inner = store.EventsFile
		}
		p := filepath.Join(m.backupDir(), e.Name())
		if _, err := m.write(KindUpdate, map[string]string{inner: p}); err != nil {
			return n, err
		}
		if err := os.Remove(p); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// AfterStart backs up when this build's version differs from the last one
// that ran, then drops update backups older than that one: the previous
// update is now known to start fine.
func (m *Manager) AfterStart(ctx context.Context) error {
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

var allowed = map[string]bool{MetaFile: true, store.MainFile: true, store.EventsFile: true}

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
	if !got[store.MainFile] {
		return errors.New("the backup holds no database")
	}
	sm, se := store.SchemaVersions()
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
		if err := m.CheckKey(filepath.Join(out, store.MainFile)); err != nil {
			return fmt.Errorf("the backup's secrets cannot be read with this master key: %w", err)
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

// ApplyStaged swaps a staged restore in. Call it before the store opens.
// The databases it replaces are kept in backups/before-restore-<time>.
func ApplyStaged(dir string) (bool, error) {
	staged := filepath.Join(dir, StagedDir)
	if !Staged(dir) {
		if _, err := os.Stat(staged); err == nil {
			_ = os.RemoveAll(staged) // half-written: never use it
		}
		return false, nil
	}
	keep := filepath.Join(dir, "backups", "before-restore-"+time.Now().UTC().Format(stampLayout))
	if err := os.MkdirAll(keep, 0o750); err != nil {
		return false, err
	}
	type movedFile struct{ from, to string }
	var old, installed []movedFile
	rollback := func(cause error) error {
		var errs []error
		// Put staged databases back first, so their live names are free for
		// the coherent old database sets below. This also leaves the staged
		// restore retryable after a recoverable failure.
		for i := len(installed) - 1; i >= 0; i-- {
			m := installed[i]
			if err := applyRename(m.to, m.from); err != nil {
				errs = append(errs, fmt.Errorf("putting staged %s back: %w", filepath.Base(m.from), err))
			}
		}
		for i := len(old) - 1; i >= 0; i-- {
			m := old[i]
			if err := applyRename(m.to, m.from); err != nil {
				errs = append(errs, fmt.Errorf("restoring %s: %w", filepath.Base(m.from), err))
			}
		}
		if len(errs) == 0 {
			_ = os.Remove(keep)
			return cause
		}
		return errors.Join(append([]error{cause}, errs...)...)
	}
	for _, db := range []string{store.MainFile, store.EventsFile} {
		if _, err := os.Stat(filepath.Join(staged, db)); err != nil {
			continue // not in the backup: keep the current one
		}
		for _, suffix := range []string{"", "-wal", "-shm"} {
			cur := filepath.Join(dir, db+suffix)
			if _, err := os.Stat(cur); err == nil {
				kept := filepath.Join(keep, db+suffix)
				if err := applyRename(cur, kept); err != nil {
					return false, rollback(err)
				}
				old = append(old, movedFile{from: cur, to: kept})
			}
		}
		from, to := filepath.Join(staged, db), filepath.Join(dir, db)
		if err := applyRename(from, to); err != nil {
			return false, rollback(err)
		}
		installed = append(installed, movedFile{from: from, to: to})
	}
	if err := os.RemoveAll(staged); err != nil {
		return true, err
	}
	log.Printf("backup: restored the staged backup; the replaced databases are in %s", keep)
	return true, nil
}
