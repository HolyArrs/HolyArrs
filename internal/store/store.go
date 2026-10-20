package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"

	_ "modernc.org/sqlite" // pure Go SQLite driver
)

// File names inside the config folder.
const (
	MainFile   = "holyarrs.db"
	EventsFile = "holyarrs-events.db"
)

// ErrNetworkShare is returned when the config folder is on a network share.
var ErrNetworkShare = errors.New("config folder is on a network share")

//go:embed migrations
var embedded embed.FS

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
	// WriteCap is the time cap for one write transaction (default
	// DefaultWriteCap; below zero: none). Migrations have no cap.
	WriteCap time.Duration
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
	hooks   []func(domain.Event)
	filesMu sync.Mutex
}

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
		check = isNetworkShare
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	share, kind, err := check(dir)
	if err != nil {
		return nil, fmt.Errorf("checking config folder: %w", err)
	}
	if share {
		return nil, fmt.Errorf("%w (%s): %s; SQLite in WAL mode needs a local disk, so move the config folder to local storage", ErrNetworkShare, kind, dir)
	}
	cache := opt.CacheKiB
	if cache <= 0 {
		cache = 8000
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
	if s.read, err = openPool(mainPath, cache, true, 4); err != nil {
		return nil, err
	}
	if s.readEv, err = openPool(evPath, cache, true, 2); err != nil {
		return nil, err
	}
	backups := filepath.Join(dir, "backups")
	if err := migrate(s.main, mainMig, backups, "main"); err != nil {
		return nil, err
	}
	if err := migrate(s.events, evMig, backups, "events"); err != nil {
		return nil, err
	}
	ok = true
	return s, nil
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
	return "file:" + filepath.ToSlash(path) + "?" + q.Encode()
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
