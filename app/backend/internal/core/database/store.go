package database

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/files"

	_ "modernc.org/sqlite" // pure Go SQLite driver
)

// File names inside the config folder.
const (
	MainFile   = "holyarrs.db"
	EventsFile = "holyarrs-events.db"
)

// ErrNetworkShare is returned when the config folder is on a network share,
// a folder shared from the host, or FUSE. The error is a *NetShareError
// that carries the kind.
var ErrNetworkShare = errors.New("config folder is on a filesystem the database can't use safely")

// ErrInMemory is returned when the config folder is in memory (tmpfs,
// ramfs), where everything is gone after a restart.
var ErrInMemory = errors.New("config folder is in memory, so everything in it is lost at the next restart")

// ErrNotWritable is returned when nothing can be written in the config
// folder.
var ErrNotWritable = errors.New("config folder cannot be written")

//go:embed migrations
var embedded embed.FS

// NetShareError is the refusal of a config folder on a share; Kind is the
// statfs kind ("9p", "fuse", "nfs", "fakeowner"...).
type NetShareError struct{ Kind string }

// Options tunes Open. The zero value is the normal setup.
type Options struct {
	// NetworkShare reports whether dir is on a network share, and its type.
	// Nil means the platform check (statfs on Linux).
	NetworkShare func(dir string) (bool, string, error)
	// MainMigrations and EventsMigrations replace the embedded SQL files.
	MainMigrations   fs.FS
	EventsMigrations fs.FS
	// CacheKiB caps the page cache per connection (default 8000 KiB).
	CacheKiB int
	// ReaderCacheKiB caps it per read connection (default: CacheKiB).
	ReaderCacheKiB int
	// WriteCap is the time cap for one write transaction (default
	// DefaultWriteCap; below zero: none). Migrations have no cap.
	WriteCap time.Duration
	// OnUpdating, when set, is called before existing data is updated
	// (pending migrations on a database that already held data).
	OnUpdating func()
}

// Store holds the main database and the events database. Reads go through
// read-only pools; every write goes through one writer goroutine per file.
type Store struct {
	dir    string
	read   *sql.DB
	readEv *sql.DB
	main   *writer
	events *writer
	// hooks are told about every Event after it is stored.
	hookMu  sync.RWMutex
	hooks   []func(Event)
	filesMu sync.Mutex
}

// NotWritable wraps err, from writing in the config folder dir, as
// ErrNotWritable when the reason is a plain one a person fixes (read-only,
// no permission, disk full or over quota), keeping the cause; nil for any
// other error.
func NotWritable(dir string, err error) error {
	why := files.Why(err)
	if why == "" || errors.Is(err, syscall.EINVAL) {
		return nil
	}
	return fmt.Errorf("%w: %s: %s (%w)", ErrNotWritable, dir, why, err)
}

func (e *NetShareError) Error() string {
	return "config folder is on a shared or network folder (" + e.Kind + ")"
}

// Unwrap makes errors.Is(err, ErrNetworkShare) true.
func (e *NetShareError) Unwrap() error { return ErrNetworkShare }

// LockFiles coordinates protection changes with replacement and removal.
// Hold it before reading a plan and until its disk and database work ends.
// It does not hold the database writer while files are being placed.
func (s *Store) LockFiles() func() {
	s.filesMu.Lock()
	return s.filesMu.Unlock
}

// Open opens (and creates) both databases in dir and runs pending migrations.
func Open(dir string, opt Options) (*Store, error) {
	check := opt.NetworkShare
	if check == nil {
		check = files.IsNetworkShare
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	var err error
	if err = checkFolder(dir, check, opt.NetworkShare == nil); err != nil {
		return nil, err
	}
	cache := opt.CacheKiB
	if cache <= 0 {
		cache = 8000
	}
	readCache := opt.ReaderCacheKiB
	if readCache <= 0 {
		readCache = cache
	}
	mainMig, evMig := opt.MainMigrations, opt.EventsMigrations
	if mainMig == nil {
		mainMig, _ = fs.Sub(embedded, "migrations/main")
	}
	if evMig == nil {
		evMig, _ = fs.Sub(embedded, "migrations/events")
	}

	s := &Store{dir: dir}
	ok := false
	defer func() {
		if !ok {
			_ = s.Close()
		}
	}()
	mainPath := filepath.Join(dir, MainFile)
	evPath := filepath.Join(dir, EventsFile)
	limit := opt.WriteCap
	if limit == 0 {
		limit = DefaultWriteCap
	}
	limit = max(limit, 0)
	if s.main, err = newWriter(mainPath, cache, limit); err != nil {
		return nil, err
	}
	if s.events, err = newWriter(evPath, cache, limit); err != nil {
		return nil, err
	}
	s.main.owner, s.events.owner = s, s
	if s.read, err = openPool(mainPath, readCache, true, 4); err != nil {
		return nil, err
	}
	if s.readEv, err = openPool(evPath, readCache, true, 2); err != nil {
		return nil, err
	}
	ownerOnly(mainPath, evPath)
	backups := filepath.Join(dir, "backups")
	// Both files are checked for newer data before either is changed.
	mainMs, mainCur, err := plan(s.main, mainMig, "main")
	if err != nil {
		return nil, err
	}
	evMs, evCur, err := plan(s.events, evMig, "events")
	if err != nil {
		return nil, err
	}
	if err := migrate(s.main, mainPath, mainMs, mainCur, backups, "main", opt.OnUpdating); err != nil {
		return nil, err
	}
	if err := migrate(s.events, evPath, evMs, evCur, backups, "events", opt.OnUpdating); err != nil {
		return nil, err
	}
	ok = true
	return s, nil
}

// Read returns the read-only pool for the main database.
func (s *Store) Read() *sql.DB { return s.read }

// ReadEvents returns the read-only pool for the events database.
func (s *Store) ReadEvents() *sql.DB { return s.readEv }

// Write runs fn in one transaction on the main database, through the single
// writer. An error from fn rolls the transaction back and is returned.
func (s *Store) Write(ctx context.Context, fn func(*sql.Tx) error) error {
	return s.main.do(ctx, fn)
}

// WriteCore is Write for core writes (sign-in, sessions, accounts): it goes
// ahead of every write still waiting, so a busy queue never holds up
// sign-in. A write already running is limited by the time cap.
func (s *Store) WriteCore(ctx context.Context, fn func(*sql.Tx) error) error {
	return s.main.doCore(ctx, fn)
}

// WriteEvents runs fn in one transaction on the events database.
func (s *Store) WriteEvents(ctx context.Context, fn func(*sql.Tx) error) error {
	return s.events.do(ctx, fn)
}

// WriteCount returns how many write transactions both databases committed.
func (s *Store) WriteCount() int64 { return s.main.commits.Load() + s.events.commits.Load() }

// WriteWaitP95 is the 95th percentile wait for the main writer over its
// last 1024 writes, with the number of writes it covers.
func (s *Store) WriteWaitP95() (time.Duration, int) { return s.main.waits.p95() }

// Version returns the newest applied migration of the main database.
func (s *Store) Version() (int, error) { return version(s.read) }

// Dir returns the config folder.
func (s *Store) Dir() string { return s.dir }

// Close stops the writers and closes every pool. Calling it again is safe.
func (s *Store) Close() error {
	var errs []error
	for _, w := range []*writer{s.main, s.events} {
		if w != nil {
			errs = append(errs, w.close())
		}
	}
	for _, db := range []*sql.DB{s.read, s.readEv} {
		if db != nil {
			errs = append(errs, db.Close())
		}
	}
	// The pools stay set: a background reader that outlives the server gets
	// "database is closed" errors instead of a nil pointer panic.
	return errors.Join(errs...)
}

// Snapshot writes consistent copies of both databases into dir with
// VACUUM INTO, which is safe while the app runs. It returns their paths.
func (s *Store) Snapshot(dir string) (mainCopy, eventsCopy string, err error) {
	mainCopy, eventsCopy = filepath.Join(dir, MainFile), filepath.Join(dir, EventsFile)
	for _, x := range []struct {
		w   *writer
		dst string
	}{{s.main, mainCopy}, {s.events, eventsCopy}} {
		if _, err := os.Stat(x.dst); err == nil {
			return "", "", fmt.Errorf("snapshot: %s already exists", x.dst)
		}
		if _, err := x.w.db.Exec("VACUUM INTO ?", x.dst); err != nil {
			return "", "", fmt.Errorf("snapshot: %w", err)
		}
	}
	ownerOnly(mainCopy, eventsCopy)
	return mainCopy, eventsCopy, nil
}

// SchemaVersions returns the newest migration known to this build for the
// main and events databases.
func SchemaVersions() (mainV, eventsV int) {
	for i, sub := range []string{"migrations/main", "migrations/events"} {
		src, _ := fs.Sub(embedded, sub)
		ms, _ := loadMigrations(src)
		if n := len(ms); n > 0 {
			if i == 0 {
				mainV = ms[n-1].version
			} else {
				eventsV = ms[n-1].version
			}
		}
	}
	return mainV, eventsV
}

// FileURI is fileURI for other packages that open a database file by
// hand, so a '#' or '?' in a folder name cannot point SQLite elsewhere.
func FileURI(path string) string { return fileURI(path) }

// testTemp reports a test binary's own temporary folder: on systems where
// /tmp is tmpfs every test would otherwise be refused. The real check still
// refuses memory anywhere else (for example /dev/shm).
func testTemp(dir string) bool {
	exe := filepath.Base(os.Args[0])
	if !strings.HasSuffix(exe, ".test") && !strings.HasSuffix(exe, ".test.exe") {
		return false
	}
	rel, err := filepath.Rel(os.TempDir(), dir)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// checkFolder refuses a config folder the database cannot use safely, before
// any database file is made, then checks that it can be written. real is
// false when a test hook does the share check.
func checkFolder(dir string, check func(string) (bool, string, error), real bool) error {
	share, kind, err := check(dir)
	if err != nil {
		return fmt.Errorf("checking config folder: %w", err)
	}
	switch {
	case !share:
	case files.InMemory(kind):
		if files.ConfigInMemoryAllowed() {
			log.Printf("holyarrs: warning: config folder %s is in memory (%s), kept there because %s is set; everything in it is lost at the next restart", dir, kind, files.EnvConfigInMemory)
		} else if !real || !testTemp(dir) {
			return fmt.Errorf("%w (%s): %s; put /config on a disk (on Unraid: /mnt/<pool>/appdata/holyarrs, and check that the pool exists), or set %s=yes to keep it in memory anyway", ErrInMemory, kind, dir, files.EnvConfigInMemory)
		}
	case kind == "fakeowner" && files.DesktopFolderAllowed():
		log.Printf("holyarrs: warning: config folder %s is a Docker Desktop folder share, used anyway because %s is set; file locks there may not work", dir, files.EnvAllowDesktopFolder)
	default:
		msg := files.ShareFix(kind)
		if kind == "fakeowner" {
			msg += " If you have checked that file locks work in this folder, " + files.EnvAllowDesktopFolder + "=1 starts HolyArrs on it anyway."
		}
		if fs := files.MountType(dir); fs != "" && fs != kind {
			msg = "mount type " + fs + "; " + msg
		}
		return fmt.Errorf("%w: %s; %s", &NetShareError{Kind: kind}, dir, msg)
	}
	f, err := os.CreateTemp(dir, ".holyarrs-write-*")
	if err != nil {
		why := files.Why(err)
		if why == "" {
			why = "the write test failed"
		}
		return fmt.Errorf("%w: %s: %s (%w)", ErrNotWritable, dir, why, err)
	}
	_ = f.Close()
	_ = os.Remove(f.Name())
	return nil
}

func dsn(path string, cacheKiB int, readOnly bool) string {
	q := url.Values{}
	for _, p := range []string{
		"journal_mode(WAL)",
		"busy_timeout(5000)",
		"synchronous(NORMAL)",
		fmt.Sprintf("cache_size(-%d)", cacheKiB),
		"mmap_size(0)",
		"foreign_keys(1)",
	} {
		q.Add("_pragma", p)
	}
	if readOnly {
		q.Add("_pragma", "query_only(1)")
	} else {
		q.Set("_txlock", "immediate")
	}
	return fileURI(path) + "?" + q.Encode()
}

func openPool(path string, cacheKiB int, readOnly bool, conns int) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dsn(path, cacheKiB, readOnly))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(conns)
	db.SetMaxIdleConns(conns)
	db.SetConnMaxIdleTime(5 * time.Minute)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	return db, nil
}

// ownerOnly makes database files and their -wal/-shm files readable by
// the owner only (they hold password and key hashes); SQLite creates them
// with the process umask. A file that is not there or cannot be changed is
// left as it is.
func ownerOnly(paths ...string) {
	for _, p := range paths {
		for _, f := range []string{p, p + "-wal", p + "-shm"} {
			_ = os.Chmod(f, 0o600)
		}
	}
}

// fileURI is path as a SQLite file: URI; '%', '?' and '#' in a folder name
// are escaped so they do not end the path.
func fileURI(path string) string {
	return "file:" + strings.NewReplacer("%", "%25", "?", "%3F", "#", "%23").Replace(filepath.ToSlash(path))
}
