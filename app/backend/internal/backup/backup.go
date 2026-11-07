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
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
)

// Kinds of backup.
const (
	KindScheduled = "scheduled"
	KindManual    = "manual" // kept until the person deletes it
	KindUpdate    = "update" // kept until the next update starts fine
	// KindPreUpdate is the copy of the data taken right before an update
	// changed it: the way back. Never removed by Prune; the newest
	// KeepPreUpdate are kept, older ones dropped only after a good start.
	KindPreUpdate = "pre-update"
)

// KeepPreUpdate is how many pre-update backups are kept.
const KeepPreUpdate = 3

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
	// Detail says what the backup is, such as "before updating from 0.3.0
	// to 0.4.0" (pre-update backups only).
	Detail string `json:"detail,omitempty"`
	// FromVersion and ToVersion name the update a pre-update backup was
	// taken for, when the version that ran before is known.
	FromVersion string `json:"from_version,omitempty"`
	ToVersion   string `json:"to_version,omitempty"`
}

// Manager makes, lists, prunes and restores backups.
type Manager struct {
	Store   *database.Store
	Dir     string // the config folder
	Version string
	Now     func() time.Time
	// CheckKey, when set, is asked whether a database file can be read with
	// the current master key, before a restore is staged.
	CheckKey func(dbPath string) error
	// Key is the master key, for the key kit download and the Backups card.
	Key []byte
	// KeyFromEnv says the key is a named file (HOLYARRS_MASTER_KEY_FILE)
	// the person looks after, so the card does not ask for a key kit.
	KeyFromEnv bool
	// FreeSpace reads free bytes on a disk (nil means importer.FreeSpace;
	// -1 unknown).
	FreeSpace func(dir string) int64
	// Stop, when set, ends with HolyArrs (shutdown). A manual backup stops
	// only then, never because the browser or a proxy dropped the request.
	Stop context.Context

	mu      sync.Mutex
	lastErr string // the last scheduled backup's error, "" after a good one
	passed  bool   // a scheduled pass has finished since start
}

// Passed says whether a scheduled pass has finished since start, so "no
// backup yet" is not said while the first one is still being made.
func (m *Manager) Passed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.passed
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

var nameRx = regexp.MustCompile(`^holyarrs-backup-(scheduled|manual|update|pre-update)-(\d{8}T\d{6}\.\d{3}Z)\.zip$`)

func fileName(kind string, at time.Time) string {
	return "holyarrs-backup-" + kind + "-" + at.UTC().Format(stampLayout) + ".zip"
}

// Info is one backup file.
type Info struct {
	Name      string    `json:"name"`
	Kind      string    `json:"kind"`
	CreatedAt time.Time `json:"created_at"`
	Size      int64     `json:"size"`
	Detail    string    `json:"detail,omitempty"`
	// FromVersion and ToVersion: the update a pre-update backup was taken
	// for (when known). Going back to FromVersion takes this backup.
	FromVersion string `json:"from_version,omitempty"`
	ToVersion   string `json:"to_version,omitempty"`
	Path        string `json:"-"`
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
		info := Info{Name: e.Name(), Kind: kind, CreatedAt: at, Size: fi.Size(), Path: filepath.Join(m.backupDir(), e.Name())}
		if kind == KindPreUpdate {
			if meta, ok := readMeta(info.Path); ok {
				info.Detail, info.FromVersion, info.ToVersion = meta.Detail, meta.FromVersion, meta.ToVersion
			}
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

// readMeta reads a backup's backup.json (false on any error).
func readMeta(path string) (Meta, bool) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return Meta{}, false
	}
	defer zr.Close()
	for _, zf := range zr.File {
		if zf.Name != MetaFile {
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			return Meta{}, false
		}
		defer rc.Close()
		var meta Meta
		if json.NewDecoder(io.LimitReader(rc, 1<<16)).Decode(&meta) != nil {
			return Meta{}, false
		}
		return meta, true
	}
	return Meta{}, false
}

// Create makes a backup of the running databases. A stop (ctx ends) cuts it
// off and removes what it made so far.
func (m *Manager) Create(ctx context.Context, kind string) (Info, error) {
	if err := ctx.Err(); err != nil {
		return Info{}, err
	}
	tmp, err := os.MkdirTemp(m.Dir, ".backup-")
	if err != nil {
		return Info{}, err
	}
	defer os.RemoveAll(tmp)
	mainCopy, evCopy, err := m.Store.Snapshot(tmp)
	if err != nil {
		return Info{}, err
	}
	return m.write(ctx, kind, map[string]string{database.MainFile: mainCopy, database.EventsFile: evCopy}, nil)
}

// ctxReader stops a copy once ctx ends.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

// CleanLeftovers removes what a backup or a restore upload cut off by a stop
// or a kill left in the config folder: its .backup-* and .restore-* working
// folders and backups/*.zip.part. None of them is valid after a restart.
// Call it before the store opens. It returns the bytes freed; a file it
// cannot remove is skipped.
func CleanLeftovers(dir string) (int64, error) {
	var freed int64
	var errs []error
	drop := func(p string) {
		n := sizeOf(p)
		if err := os.RemoveAll(p); err != nil {
			errs = append(errs, err)
			return
		}
		freed += n
	}
	es, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	for _, e := range es {
		if e.IsDir() && (strings.HasPrefix(e.Name(), ".backup-") || strings.HasPrefix(e.Name(), ".restore-")) {
			drop(filepath.Join(dir, e.Name()))
		}
	}
	bs, err := os.ReadDir(filepath.Join(dir, "backups"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, err)
	}
	for _, e := range bs {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".zip.part") {
			drop(filepath.Join(dir, "backups", e.Name()))
		}
	}
	return freed, errors.Join(errs...)
}

func sizeOf(p string) int64 {
	var n int64
	_ = filepath.WalkDir(p, func(_ string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if fi, err := d.Info(); err == nil {
				n += fi.Size()
			}
		}
		return nil
	})
	return n
}

// write zips the files with a backup.json into the backups folder.
// edit, when set, changes the backup.json before it is written.
func (m *Manager) write(ctx context.Context, kind string, files map[string]string, edit func(*Meta)) (Info, error) {
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
	sm, se := database.SchemaVersions()
	meta := Meta{App: "holyarrs", Version: m.Version, SchemaMain: sm, SchemaEvents: se, Kind: kind, CreatedAt: at, Note: KeyNote}
	if edit != nil {
		edit(&meta)
	}
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
		if err := addFile(ctx, zw, k, files[k]); err != nil {
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
	return Info{Name: name, Kind: kind, CreatedAt: at, Size: fi.Size(), Detail: meta.Detail, Path: final}, nil
}

func addFile(ctx context.Context, zw *zip.Writer, name, path string) error {
	src, err := os.Open(path)
	if err != nil {
		return err
	}
	defer src.Close()
	w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
	if err != nil {
		return err
	}
	_, err = io.Copy(w, ctxReader{ctx, src})
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
	t := database.FormatTime(m.now())
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
	if err := m.roomFor(); err != nil {
		return fmt.Errorf("%w; HolyArrs tries again in ten minutes", err)
	}
	b, err := m.Create(ctx, KindScheduled)
	if err != nil {
		return err
	}
	log.Printf("backup: made %s (%d bytes)", b.Name, b.Size)
	return m.Prune(p)
}

// roomFor refuses a backup that clearly would not fit: it needs a
// copy of the databases and the zip, so about twice their size. Unknown free
// space never refuses.
func (m *Manager) roomFor() error {
	free := m.FreeSpace
	if free == nil {
		free = importer.FreeSpace
	}
	n := free(m.Dir)
	if n < 0 {
		return nil
	}
	var need int64
	for _, f := range []string{database.MainFile, database.MainFile + "-wal", database.EventsFile, database.EventsFile + "-wal"} {
		if st, err := os.Stat(filepath.Join(m.Dir, f)); err == nil {
			need += st.Size()
		}
	}
	need *= 2
	if n >= need {
		return nil
	}
	return fmt.Errorf("Not enough free space: it needs about %d MB but only %d MB are free on the disk that holds the config folder. Free some space there", (need>>20)+1, n>>20)
}

// noSpace is said when the disk filled up during a backup.
const noSpace = "Not enough free space on the disk that holds the config folder. Free some space there"

// diskFull reports an error from a full disk (SQLite or the system).
func diskFull(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.ENOSPC) {
		return true
	}
	var en syscall.Errno
	if runtime.GOOS == "windows" && errors.As(err, &en) && (en == 39 || en == 112) { // handle / disk full
		return true
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "disk is full") || strings.Contains(s, "no space left")
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
		m.lastErr, m.passed = "", true
		if err != nil {
			m.lastErr = err.Error()
		}
		m.mu.Unlock()
	}
	return err
}

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
			return fmt.Errorf("the backup's secrets cannot be read with this master key (moving to a new machine? see \"Move to a new machine\" in the HolyArrs docs, in docs/backups.md): %w", err)
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
		err := database.WriteFileSynced(p, b, 0o600)
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
	return database.SyncDir(dir)
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
			if err := database.SyncDir(d); err != nil && !errors.Is(err, os.ErrNotExist) {
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
