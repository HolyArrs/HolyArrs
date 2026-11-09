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
	"strings"
	"syscall"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
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

// The three presets offered in the first-run setup.
const (
	Balanced Preset = "balanced"
	Light    Preset = "light"
	Careful  Preset = "careful"
)

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
	return database.Setting(ctx, m.Store.Read(), "this_app", key)
}

func (m *Manager) setSetting(ctx context.Context, key, val string, by domain.Actor) error {
	return m.Store.Write(ctx, func(tx *sql.Tx) error {
		return database.PutSetting(tx, "this_app", key, val, by, m.now())
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
