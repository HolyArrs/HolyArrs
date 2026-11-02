package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/model"
	_ "modernc.org/sqlite"
)

const (
	defaultMaxBytes     = 512 << 20
	defaultMaxEntries   = 100000
	defaultMinFreeBytes = 16 << 20
	maxResultBytes      = 2 << 20
	accessMaxBytes      = 64 << 20
	walLimit            = 8 << 20
)

// MaxBytes bounds saved answer bytes, including keys, cursors and timestamp
// accounting. SQLite pages and indexes have a separate physical ceiling.
type Options struct {
	MaxBytes     int64
	MaxEntries   int
	MinFreeBytes uint64
}

type Store struct {
	cache  *sql.DB
	access *sql.DB
	// registered holds self-registered credentials, apart from the
	// operator-issued ones in access.
	registered *sql.DB
	dir        string
	opts       Options
	writer     chan struct{}
	closeOnce  sync.Once
	closeErr   error
	owner      *os.File
	// The exclusive directory owner loads these once; writer guards updates.
	cacheEntries int
	cacheBytes   int64
}

func Open(dir string, opts Options) (*Store, error) {
	if opts.MaxBytes < 0 || opts.MaxEntries < 0 || opts.MaxBytes > 1<<40 || opts.MaxEntries > 10000000 {
		return nil, errors.New("invalid storage limits")
	}
	if opts.MaxBytes == 0 {
		opts.MaxBytes = defaultMaxBytes
	}
	if opts.MaxEntries == 0 {
		opts.MaxEntries = defaultMaxEntries
	}
	if opts.MinFreeBytes == 0 {
		opts.MinFreeBytes = defaultMinFreeBytes
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, errors.New("invalid storage directory")
	}
	if strings.HasPrefix(abs, `\\`) {
		return nil, errors.New("network storage is unsupported")
	}
	if err := os.MkdirAll(abs, 0700); err != nil {
		return nil, errors.New("cannot create storage directory")
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, errors.New("cannot resolve storage directory")
	}
	if err := checkSpace(resolved, opts.MinFreeBytes); err != nil {
		return nil, err
	}
	lockPath := filepath.Join(resolved, ".store.lock")
	if info, err := os.Lstat(lockPath); err == nil && !info.Mode().IsRegular() {
		return nil, errors.New("storage lock must be a regular file")
	}
	owner, err := lockDirectory(lockPath)
	if err != nil {
		return nil, err
	}
	opened := false
	defer func() {
		if !opened {
			owner.Close()
		}
	}()
	s := &Store{dir: resolved, opts: opts, writer: make(chan struct{}, 1), owner: owner}
	// Payload and index duplication are bounded independently from the logical
	// quota. A small schema allowance also permits deliberately tiny test quotas.
	physicalBytes := opts.MaxBytes*3 + int64(opts.MaxEntries)*1024 + 1<<20
	s.cache, err = openDB(filepath.Join(resolved, "cache.sqlite"), physicalBytes, "cache")
	if err != nil {
		return nil, err
	}
	s.access, err = openDB(filepath.Join(resolved, "access.sqlite"), accessMaxBytes, "access")
	if err != nil {
		s.cache.Close()
		return nil, err
	}
	s.registered, err = openDB(filepath.Join(resolved, "registered.sqlite"), accessMaxBytes, "registered")
	if err != nil {
		s.cache.Close()
		s.access.Close()
		return nil, err
	}
	if err := s.Check(context.Background()); err != nil {
		s.Close()
		return nil, err
	}
	if err := s.cache.QueryRow("SELECT COUNT(*),COALESCE(SUM(size),0) FROM entries").Scan(&s.cacheEntries, &s.cacheBytes); err != nil {
		s.Close()
		return nil, errors.New("cache quota unavailable")
	}
	opened = true
	return s, nil
}

func openDB(path string, physicalBytes int64, kind string) (*sql.DB, error) {
	cache := kind == "cache"
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return nil, errors.New("database must be a regular local file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, errors.New("cannot inspect database")
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, errors.New("cannot open database")
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	failed := true
	defer func() {
		if failed {
			db.Close()
		}
	}()
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return nil, errors.New("cannot read database schema")
	}
	if version != 0 && version != 1 && !(kind == "registered" && version == 2) {
		return nil, errors.New("unsupported database schema")
	}
	if kind == "registered" && version == 1 {
		// v2: network (widest address group) and used (0 until first use).
		// Existing credentials count as used.
		if _, err := db.Exec(`ALTER TABLE registered ADD COLUMN network TEXT NOT NULL DEFAULT ''; ALTER TABLE registered ADD COLUMN used INTEGER NOT NULL DEFAULT 1 CHECK(used IN (0,1)); PRAGMA user_version=2;`); err != nil {
			return nil, errors.New("cannot upgrade database schema")
		}
	}
	if version == 0 {
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_schema WHERE name NOT LIKE 'sqlite_%'").Scan(&count); err != nil || count != 0 {
			return nil, errors.New("unrecognized database schema")
		}
		schema := `CREATE TABLE tokens (hash BLOB PRIMARY KEY CHECK(length(hash)=32), revoked INTEGER NOT NULL CHECK(revoked IN (0,1))) WITHOUT ROWID; PRAGMA user_version=1;`
		if kind == "registered" {
			schema = `CREATE TABLE registered (hash BLOB PRIMARY KEY CHECK(length(hash)=32), source TEXT NOT NULL, last_used INTEGER NOT NULL, revoked INTEGER NOT NULL CHECK(revoked IN (0,1)), network TEXT NOT NULL DEFAULT '', used INTEGER NOT NULL DEFAULT 1 CHECK(used IN (0,1))) WITHOUT ROWID; CREATE INDEX registered_used ON registered(revoked,last_used); PRAGMA user_version=2;`
		}
		if cache {
			schema = `CREATE TABLE entries (key TEXT PRIMARY KEY, data BLOB NOT NULL, cursor TEXT NOT NULL, fetched_at TEXT NOT NULL, expires_at TEXT NOT NULL, age INTEGER NOT NULL, size INTEGER NOT NULL CHECK(size>0)) WITHOUT ROWID; CREATE INDEX entries_age ON entries(age,key); PRAGMA user_version=1;`
		}
		tx, err := db.Begin()
		if err != nil {
			return nil, errors.New("cannot initialize schema")
		}
		if _, err := tx.Exec(schema); err != nil {
			tx.Rollback()
			return nil, errors.New("cannot initialize schema")
		}
		if err := tx.Commit(); err != nil {
			return nil, errors.New("cannot commit schema")
		}
	}
	shape := "SELECT hash,revoked FROM tokens LIMIT 0"
	if kind == "registered" {
		shape = "SELECT hash,source,last_used,revoked,network,used FROM registered LIMIT 0"
	}
	if cache {
		shape = "SELECT key,data,cursor,fetched_at,expires_at,age,size FROM entries LIMIT 0"
	}
	rows, err := db.Query(shape)
	if err != nil {
		return nil, errors.New("database schema does not match its version")
	}
	rows.Close()
	var pages, pageSize int64
	if err := db.QueryRow("PRAGMA page_count").Scan(&pages); err != nil {
		return nil, errors.New("cannot inspect database size")
	}
	if err := db.QueryRow("PRAGMA page_size").Scan(&pageSize); err != nil {
		return nil, errors.New("cannot inspect database pages")
	}
	maxPages := physicalBytes / pageSize
	if pages > maxPages {
		return nil, errors.New("database exceeds physical storage limit")
	}
	// The answer cache is the one large, hot database: it reads through a
	// 64 MB page cache and up to 2 GB mapped (the VM has 16 GB for the
	// server); the small credential databases keep the tight defaults.
	cacheSize, mmapSize := "-2048", "0"
	if cache {
		cacheSize, mmapSize = "-65536", "2147418112" // SQLite's ceiling
	}
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL", "PRAGMA synchronous=FULL", "PRAGMA busy_timeout=1500",
		"PRAGMA cache_size=" + cacheSize, "PRAGMA mmap_size=" + mmapSize, "PRAGMA temp_store=FILE",
		"PRAGMA wal_autocheckpoint=64", "PRAGMA journal_size_limit=262144",
		fmt.Sprintf("PRAGMA max_page_count=%d", maxPages),
	} {
		if _, err := db.Exec(pragma); err != nil {
			return nil, errors.New("cannot configure database")
		}
	}
	if err := os.Chmod(path, 0600); err != nil {
		return nil, errors.New("cannot secure database")
	}
	failed = false
	return db, nil
}

func (s *Store) Close() error {
	s.closeOnce.Do(func() {
		s.closeErr = errors.Join(s.cache.Close(), s.access.Close(), s.registered.Close(), s.owner.Close())
	})
	return s.closeErr
}

func (s *Store) lock(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case s.writer <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Store) unlock() { <-s.writer }

func (s *Store) writable(ctx context.Context, db *sql.DB, filename string) error {
	if err := checkSpace(s.dir, s.opts.MinFreeBytes); err != nil {
		return err
	}
	info, err := os.Stat(filepath.Join(s.dir, filename+"-wal"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.New("cannot inspect write log")
	}
	if err == nil && info.Size() >= walLimit {
		var busy, logPages, checkpointed int
		if err := db.QueryRowContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &logPages, &checkpointed); err != nil || busy != 0 {
			return errors.New("write log capacity unavailable")
		}
	}
	return ctx.Err()
}

func (s *Store) Get(ctx context.Context, key string) (model.Entry, bool, error) {
	var e model.Entry
	var fetched, expires string
	err := s.cache.QueryRowContext(ctx, "SELECT key,data,cursor,fetched_at,expires_at FROM entries WHERE key=?", key).Scan(&e.Key, &e.Result.Data, &e.Result.NextCursor, &fetched, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return e, false, nil
	}
	if err != nil {
		return e, false, errors.New("cache read unavailable")
	}
	e.FetchedAt, err = time.Parse(time.RFC3339Nano, fetched)
	if err != nil {
		return model.Entry{}, false, errors.New("invalid stored answer time")
	}
	e.ExpiresAt, err = time.Parse(time.RFC3339Nano, expires)
	if err != nil {
		return model.Entry{}, false, errors.New("invalid stored expiry time")
	}
	e.Result.Complete = true
	return e, true, nil
}

func (s *Store) Put(ctx context.Context, e model.Entry) error {
	if !e.Result.Complete || e.Key == "" || len(e.Key) > 256 || len(e.Result.NextCursor) > 4096 || len(e.Result.Data) > maxResultBytes || e.FetchedAt.IsZero() || e.ExpiresAt.Before(e.FetchedAt) {
		return errors.New("incomplete or invalid answer")
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(e.Result.Data, &object); err != nil || object == nil {
		return errors.New("answer must be a complete JSON object")
	}
	size := int64(len(e.Key) + len(e.Result.Data) + len(e.Result.NextCursor) + 64)
	if size > s.opts.MaxBytes {
		return errors.New("answer exceeds cache quota")
	}
	if err := s.lock(ctx); err != nil {
		return err
	}
	defer s.unlock()
	if err := s.writable(ctx, s.cache, "cache.sqlite"); err != nil {
		return err
	}
	tx, err := s.cache.BeginTx(ctx, nil)
	if err != nil {
		return errors.New("cache write unavailable")
	}
	defer tx.Rollback()
	count, used := s.cacheEntries, s.cacheBytes
	var oldSize int64
	err = tx.QueryRowContext(ctx, "SELECT size FROM entries WHERE key=?", e.Key).Scan(&oldSize)
	if err == nil {
		if _, err := tx.ExecContext(ctx, "DELETE FROM entries WHERE key=?", e.Key); err != nil {
			return errors.New("cache replacement unavailable")
		}
		count--
		used -= oldSize
	} else if !errors.Is(err, sql.ErrNoRows) {
		return errors.New("cache replacement unavailable")
	}
	for count >= s.opts.MaxEntries || used+size > s.opts.MaxBytes {
		var key string
		var victimSize int64
		if err := tx.QueryRowContext(ctx, "SELECT key,size FROM entries ORDER BY age,key LIMIT 1").Scan(&key, &victimSize); err != nil {
			return errors.New("cache eviction unavailable")
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM entries WHERE key=?", key); err != nil {
			return errors.New("cache eviction unavailable")
		}
		count--
		used -= victimSize
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO entries(key,data,cursor,fetched_at,expires_at,age,size) VALUES(?,?,?,?,?,?,?)", e.Key, []byte(e.Result.Data), e.Result.NextCursor, e.FetchedAt.UTC().Format(time.RFC3339Nano), e.ExpiresAt.UTC().Format(time.RFC3339Nano), e.FetchedAt.UnixNano(), size); err != nil {
		return errors.New("cache write failed")
	}
	if err := tx.Commit(); err != nil {
		return errors.New("cache commit failed")
	}
	s.cacheEntries, s.cacheBytes = count+1, used+size
	return nil
}

// Delete removes stored answers by key (eviction of long-unrequested rows).
func (s *Store) Delete(ctx context.Context, keys []string) error {
	if err := s.lock(ctx); err != nil {
		return err
	}
	defer s.unlock()
	tx, err := s.cache.BeginTx(ctx, nil)
	if err != nil {
		return errors.New("cache write unavailable")
	}
	defer tx.Rollback()
	count, used := s.cacheEntries, s.cacheBytes
	for _, k := range keys {
		var size int64
		if err := tx.QueryRowContext(ctx, "SELECT size FROM entries WHERE key=?", k).Scan(&size); errors.Is(err, sql.ErrNoRows) {
			continue
		} else if err != nil {
			return errors.New("cache delete unavailable")
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM entries WHERE key=?", k); err != nil {
			return errors.New("cache delete unavailable")
		}
		count--
		used -= size
	}
	if err := tx.Commit(); err != nil {
		return errors.New("cache commit failed")
	}
	s.cacheEntries, s.cacheBytes = count, used
	return nil
}

func tokenHash(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

func (s *Store) IssueToken(ctx context.Context, token string) error {
	if strings.TrimSpace(token) == "" || len(token) > 4096 {
		return errors.New("invalid installation credential")
	}
	if err := s.lock(ctx); err != nil {
		return err
	}
	defer s.unlock()
	if err := s.writable(ctx, s.access, "access.sqlite"); err != nil {
		return err
	}
	var revoked int
	err := s.access.QueryRowContext(ctx, "SELECT revoked FROM tokens WHERE hash=?", tokenHash(token)).Scan(&revoked)
	if err == nil {
		if revoked != 0 {
			return errors.New("installation credential was revoked")
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return errors.New("credential storage unavailable")
	}
	if _, err := s.access.ExecContext(ctx, "INSERT INTO tokens(hash,revoked) VALUES(?,0)", tokenHash(token)); err != nil {
		return errors.New("credential storage failed")
	}
	return nil
}

func (s *Store) Authorized(ctx context.Context, token string) (bool, error) {
	if token == "" || len(token) > 4096 {
		return false, nil
	}
	var revoked int
	err := s.access.QueryRowContext(ctx, "SELECT revoked FROM tokens WHERE hash=?", tokenHash(token)).Scan(&revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, errors.New("credential check unavailable")
	}
	return revoked == 0, nil
}

func (s *Store) RevokeToken(ctx context.Context, token string) error {
	if strings.TrimSpace(token) == "" || len(token) > 4096 {
		return errors.New("invalid installation credential")
	}
	if err := s.lock(ctx); err != nil {
		return err
	}
	defer s.unlock()
	if err := s.writable(ctx, s.access, "access.sqlite"); err != nil {
		return err
	}
	if _, err := s.access.ExecContext(ctx, "INSERT INTO tokens(hash,revoked) VALUES(?,1) ON CONFLICT(hash) DO UPDATE SET revoked=1", tokenHash(token)); err != nil {
		return errors.New("credential revocation failed")
	}
	if _, err := s.registered.ExecContext(ctx, "UPDATE registered SET revoked=1 WHERE hash=?", tokenHash(token)); err != nil {
		return errors.New("credential revocation failed")
	}
	return nil
}

// RegisterToken stores a self-registered credential for source (the
// grouped client address) in network (its widest address group).
// Credentials idle past idle are dropped. When maxActive are active, room
// is made only from credentials that are not protected: never used ones
// from the same network, or ones idle longer than protect. Recently used
// credentials are never evicted; when nothing can go, it returns
// model.ErrRegistryFull and stores nothing.
func (s *Store) RegisterToken(ctx context.Context, token, source, network string, maxActive int, idle, protect time.Duration) error {
	if strings.TrimSpace(token) == "" || len(token) > 4096 || source == "" || len(source) > 64 || len(network) > 64 || maxActive < 1 || idle <= 0 || protect <= 0 {
		return errors.New("invalid registration")
	}
	if err := s.lock(ctx); err != nil {
		return err
	}
	defer s.unlock()
	if err := s.writable(ctx, s.registered, "registered.sqlite"); err != nil {
		return err
	}
	now := time.Now().Unix()
	tx, err := s.registered.BeginTx(ctx, nil)
	if err != nil {
		return errors.New("credential storage unavailable")
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "DELETE FROM registered WHERE revoked=0 AND last_used<?", now-int64(idle/time.Second)); err != nil {
		return errors.New("credential storage failed")
	}
	var active int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM registered WHERE revoked=0").Scan(&active); err != nil {
		return errors.New("credential storage unavailable")
	}
	if active >= maxActive {
		need := active - maxActive + 1
		res, err := tx.ExecContext(ctx, `DELETE FROM registered WHERE hash IN (SELECT hash FROM registered WHERE revoked=0 AND ((used=0 AND network=? AND network<>'') OR last_used<?) ORDER BY used, last_used LIMIT ?)`, network, now-int64(protect/time.Second), need)
		if err != nil {
			return errors.New("credential storage failed")
		}
		if n, err := res.RowsAffected(); err != nil || n < int64(need) {
			return model.ErrRegistryFull
		}
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO registered(hash,source,last_used,revoked,network,used) VALUES(?,?,?,0,?,0)", tokenHash(token), source, now, network); err != nil {
		return errors.New("credential storage failed")
	}
	if err := tx.Commit(); err != nil {
		return errors.New("credential storage failed")
	}
	return nil
}

// Registered reports whether token is an active self-registered credential
// and its source group. Use renews it (at most once a minute; the first
// use at once, which also marks it used).
func (s *Store) Registered(ctx context.Context, token string, idle time.Duration) (string, bool, error) {
	if token == "" || len(token) > 4096 {
		return "", false, nil
	}
	var source string
	var used int64
	var revoked, wasUsed int
	err := s.registered.QueryRowContext(ctx, "SELECT source,last_used,revoked,used FROM registered WHERE hash=?", tokenHash(token)).Scan(&source, &used, &revoked, &wasUsed)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, errors.New("credential check unavailable")
	}
	now := time.Now().Unix()
	if revoked != 0 || used < now-int64(idle/time.Second) {
		return "", false, nil
	}
	if (wasUsed == 0 || now-used >= 60) && s.lock(ctx) == nil {
		_, _ = s.registered.ExecContext(ctx, "UPDATE registered SET last_used=?, used=1 WHERE hash=? AND revoked=0", now, tokenHash(token))
		s.unlock()
	}
	return source, true, nil
}

func integrity(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, "PRAGMA integrity_check")
	if err != nil {
		return errors.New("database integrity unavailable")
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var status string
		if err := rows.Scan(&status); err != nil || status != "ok" {
			return errors.New("database integrity failed")
		}
		count++
	}
	if err := rows.Err(); err != nil || count != 1 {
		return errors.New("database integrity failed")
	}
	return nil
}

func (s *Store) Check(ctx context.Context) error {
	if err := integrity(ctx, s.cache); err != nil {
		return err
	}
	if err := integrity(ctx, s.access); err != nil {
		return err
	}
	return integrity(ctx, s.registered)
}

func (s *Store) Ready(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	for _, db := range []*sql.DB{s.cache, s.access, s.registered} {
		if err := db.PingContext(ctx); err != nil {
			return errors.New("storage unavailable")
		}
	}
	return nil
}

func (s *Store) Backup(ctx context.Context, destination string) error {
	if err := s.lock(ctx); err != nil {
		return err
	}
	defer s.unlock()
	abs, err := filepath.Abs(destination)
	if err != nil {
		return errors.New("invalid backup destination")
	}
	parent := filepath.Dir(abs)
	if err := checkSpace(parent, s.opts.MinFreeBytes); err != nil {
		return err
	}
	if err := os.Mkdir(abs, 0700); err != nil {
		return errors.New("backup destination must not exist")
	}
	complete := false
	defer func() {
		if !complete {
			os.Remove(filepath.Join(abs, "cache.sqlite"))
			os.Remove(filepath.Join(abs, "access.sqlite"))
			os.Remove(filepath.Join(abs, "registered.sqlite"))
			os.Remove(abs)
		}
	}()
	for _, item := range []struct {
		name string
		db   *sql.DB
	}{{"cache.sqlite", s.cache}, {"access.sqlite", s.access}, {"registered.sqlite", s.registered}} {
		path := filepath.Join(abs, item.name)
		if _, err := item.db.ExecContext(ctx, "VACUUM INTO ?", path); err != nil {
			return errors.New("database snapshot failed")
		}
		if err := os.Chmod(path, 0600); err != nil {
			return errors.New("cannot secure snapshot")
		}
		db, err := sql.Open("sqlite", path)
		if err != nil {
			return errors.New("cannot verify snapshot")
		}
		db.SetMaxOpenConns(1)
		err = integrity(ctx, db)
		db.Close()
		if err != nil {
			return err
		}
	}
	complete = true
	return nil
}

func checkSpace(path string, minimum uint64) error {
	free, err := localSpace(path)
	if err != nil {
		return err
	}
	if free < minimum {
		return errors.New("insufficient free storage")
	}
	return nil
}
