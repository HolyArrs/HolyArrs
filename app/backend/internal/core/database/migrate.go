package database

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/files"
)

// backupTries is how many times the pre-update backup is tried.
const backupTries = 3

// SkipSpaceCheckEnv set to yes skips the free space check before an update.
const SkipSpaceCheckEnv = "HOLYARRS_SKIP_SPACE_CHECK"

// forbiddenStmt matches statements a migration must not run. BEGIN inside a
// trigger (BEGIN ... END) is allowed; a transaction BEGIN is not.
var forbiddenStmt = regexp.MustCompile(`(?im)(^|;)\s*(PRAGMA\b|VACUUM\b|COMMIT\b|END\s+TRANSACTION\b|ROLLBACK\b|BEGIN\s*(;|\z|TRANSACTION\b|DEFERRED\b|IMMEDIATE\b|EXCLUSIVE\b))`)

// Errors that stop an update of the data before (or right after) it runs.
// The data is left as it was, or the pinned backup is named.
var (
	ErrNoSpace     = errors.New("not enough free space")
	ErrBackup      = errors.New("the backup before the update failed")
	ErrCheckFailed = errors.New("the data check after the update failed")
	ErrMigration   = errors.New("the update of the data failed")
)

// Test hooks: free space on a folder's disk, whether two folders share a
// disk, and the pre-update backup.
var (
	freeSpace  = files.DiskFree
	sameVolume = files.OnSameVolume
	takeBackup = pinnedBackup
)

type migration struct {
	version int
	name    string
	sql     string
}

// NewerDataError means a database holds migrations this build does not know:
// a newer HolyArrs ran on this config folder. Nothing is changed.
type NewerDataError struct {
	Label      string
	Have, Know int
}

type querier interface {
	QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error)
}

// Migration is one numbered SQL step of the built-in schema.
type Migration struct {
	Version int
	Name    string
	SQL     string
}

// SchemaVersion is the newest migration applied to db, read the way the
// migrator reads it (doctor uses it to tell a database still being set up).
func SchemaVersion(db *sql.DB) (int, error) { return version(db) }

func (e *NewerDataError) Error() string {
	return fmt.Sprintf("this data is from a newer HolyArrs (schema %d, this build knows %d). Run that version again, or restore a backup.", e.Have, e.Know)
}

// CheckBackupSum compares a pre-update backup with the sha256 file written
// next to it. A backup without that file (an older build's) passes.
func CheckBackupSum(p string) error {
	b, err := os.ReadFile(p + ".sha256")
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	want, _, _ := strings.Cut(string(b), " ")
	got, err := fileSHA256(p)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("%s does not match its sha256 file", filepath.Base(p))
	}
	return nil
}

// EmbeddedMigrations lists the built-in steps of one database in order. dir
// is "migrations/main" or "migrations/events". Tests of other packages use it
// to build older schemas.
func EmbeddedMigrations(dir string) ([]Migration, error) {
	src, err := fs.Sub(embedded, dir)
	if err != nil {
		return nil, err
	}
	ms, err := loadMigrations(src)
	if err != nil {
		return nil, err
	}
	out := make([]Migration, len(ms))
	for i, m := range ms {
		out[i] = Migration{Version: m.version, Name: m.name, SQL: m.sql}
	}
	return out, nil
}

func loadMigrations(src fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(src, ".")
	if err != nil {
		return nil, err
	}
	var out []migration
	seen := map[int]string{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		num, _, _ := strings.Cut(e.Name(), "_")
		v, err := strconv.Atoi(num)
		if err != nil || v <= 0 {
			return nil, fmt.Errorf("migration %q must start with a positive number and an underscore", e.Name())
		}
		if prev, dup := seen[v]; dup {
			return nil, fmt.Errorf("migrations %q and %q share number %d", prev, e.Name(), v)
		}
		seen[v] = e.Name()
		b, err := fs.ReadFile(src, e.Name())
		if err != nil {
			return nil, err
		}
		sql := string(b)
		if bad := forbiddenStmt.FindString(stripComments(sql)); bad != "" {
			return nil, fmt.Errorf("migration %q has %q: migrations run in a transaction on a managed connection and must not hold PRAGMA, VACUUM, BEGIN or COMMIT", e.Name(), strings.TrimSpace(bad))
		}
		out = append(out, migration{version: v, name: e.Name(), sql: sql})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	return out, nil
}

func stripComments(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if j := strings.Index(l, "--"); j >= 0 {
			lines[i] = l[:j]
		}
	}
	return strings.Join(lines, "\n")
}

func version(db *sql.DB) (int, error) {
	var v sql.NullInt64
	if err := db.QueryRow("SELECT max(version) FROM schema_migrations").Scan(&v); err != nil {
		return 0, err
	}
	return int(v.Int64), nil
}

// headroom is the free space kept on top of what a backup or update
// needs: 10% of the database, at least 50 MB.
func headroom(size int64) uint64 {
	return max(uint64(50<<20), uint64(size)/10)
}

func skipSpaceCheck() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(SkipSpaceCheckEnv))) {
	case "yes", "1", "true", "on":
		return true
	}
	return false
}

// plan makes sure schema_migrations exists and returns the pending
// migrations and the version applied. Data newer than this build is refused.
func plan(w *writer, src fs.FS, label string) ([]migration, int, error) {
	ms, err := loadMigrations(src)
	if err != nil {
		return nil, 0, err
	}
	err = w.doUncapped(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations(
			version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at TEXT NOT NULL)`)
		return err
	})
	if err != nil {
		return nil, 0, err
	}
	cur, err := version(w.db)
	if err != nil {
		return nil, 0, err
	}
	know := 0
	if len(ms) > 0 {
		know = ms[len(ms)-1].version
	}
	if cur > know {
		return nil, 0, &NewerDataError{Label: label, Have: cur, Know: know}
	}
	var out []migration
	for _, m := range ms {
		if m.version > cur {
			out = append(out, m)
		}
	}
	return out, cur, nil
}

// migrate applies pending migrations in order. When the database already
// held data it first takes one checked backup (pinned: the way back from
// this update), after checking there is room for it and the update.
// Migrations run with synchronous=FULL and the result is checked.
// onUpdating, when set, is called before existing data is updated.
func migrate(w *writer, path string, ms []migration, cur int, backupDir, label string, onUpdating func()) error {
	if len(ms) == 0 {
		return nil
	}
	pinned := ""
	if cur > 0 {
		if onUpdating != nil {
			onUpdating()
		}
		if err := os.MkdirAll(backupDir, 0o750); err != nil {
			return fmt.Errorf("%w (%s): the backups folder %s cannot be made: %v; the data was not changed", ErrBackup, label, backupDir, err)
		}
		stale, _ := filepath.Glob(filepath.Join(backupDir, label+"-pre-update-*.db.partial"))
		for _, p := range stale {
			_ = os.Remove(p)
		}
		if skipSpaceCheck() {
			log.Printf("store: %s=yes, so the free space check before the %s update is skipped", SkipSpaceCheckEnv, label)
		} else if err := checkSpace(path, filepath.Dir(path), backupDir); err != nil {
			return err
		}
		to := ms[len(ms)-1].version
		var err error
		if kept, from := cutOffBackup(backupDir, label); kept != "" {
			// A raw, checked copy is only left when an update was cut off
			// (a good start adopts it), so resume with that backup.
			log.Printf("store: resuming an update that was cut off; keeping the backup from %d (%s)", from, kept)
			pinned = kept
		}
		for try := 1; pinned == "" && try <= backupTries; try++ {
			if pinned, err = takeBackup(w, backupDir, label, cur, to); err == nil {
				break
			}
			log.Printf("store: backup before the %s update (try %d of %d): %v", label, try, backupTries, err)
		}
		if err != nil {
			return fmt.Errorf("%w (%s, %d tries): %v; the data was not changed", ErrBackup, label, backupTries, err)
		}
	}
	// Every PRAGMA and migration runs on one connection, so a setting can
	// never land on a different (or recycled) one.
	ctx := context.Background()
	conn, err := w.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	// foreign_keys can only change outside a transaction; migrations that
	// rebuild tables need it off, and each one is checked before commit.
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=OFF"); err != nil {
		return err
	}
	defer func() { _, _ = conn.ExecContext(ctx, "PRAGMA foreign_keys=ON") }()
	if _, err := conn.ExecContext(ctx, "PRAGMA synchronous=FULL"); err != nil {
		return err
	}
	defer func() { _, _ = conn.ExecContext(ctx, "PRAGMA synchronous=NORMAL") }()
	for _, m := range ms {
		err := func() error {
			tx, err := conn.BeginTx(ctx, nil)
			if err != nil {
				return err
			}
			defer func() { _ = tx.Rollback() }()
			if _, err := tx.Exec(m.sql); err != nil {
				return err
			}
			if _, err := tx.Exec("INSERT INTO schema_migrations(version, name, applied_at) VALUES (?, ?, ?)",
				m.version, m.name, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
				return err
			}
			if bad, err := foreignKeyCheck(tx); err != nil {
				return err
			} else if bad != "" {
				return fmt.Errorf("foreign key check: %s", bad)
			}
			return tx.Commit()
		}()
		if err != nil {
			return fmt.Errorf("%w: migration %s (%s): %v; this step was undone", ErrMigration, m.name, label, err)
		}
	}
	res, err := integrityCheck(conn)
	if err == nil && res == "ok" {
		var bad string
		if bad, err = foreignKeyCheck(conn); err == nil && bad != "" {
			res = "foreign key check: " + bad
		}
	}
	if err != nil || res != "ok" {
		if err != nil {
			res = err.Error()
		}
		where := "this was new data, so there is no backup"
		if pinned != "" {
			where = "restore the backup " + pinned
		}
		return fmt.Errorf("%w (%s): %s; %s", ErrCheckFailed, label, res, where)
	}
	return nil
}

// integrityCheck runs the full PRAGMA integrity_check; "ok" means sound.
func integrityCheck(q querier) (string, error) {
	rows, err := q.QueryContext(context.Background(), "PRAGMA integrity_check")
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return "", err
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return strings.Join(out, "; "), nil
}

// foreignKeyCheck returns "" when no row breaks a foreign key, otherwise
// the first broken row (table, rowid, parent).
func foreignKeyCheck(q querier) (string, error) {
	rows, err := q.QueryContext(context.Background(), "PRAGMA foreign_key_check")
	if err != nil {
		return "", err
	}
	defer rows.Close()
	if !rows.Next() {
		return "", rows.Err()
	}
	var table, parent string
	var rowid sql.NullInt64
	var fkid int
	if err := rows.Scan(&table, &rowid, &parent, &fkid); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s row %d points to a missing %s", table, rowid.Int64, parent), nil
}

func quickCheck(db *sql.DB) (string, error) {
	var res string
	err := db.QueryRow("PRAGMA quick_check").Scan(&res)
	return res, err
}

// checkSpace refuses when the backups disk cannot hold a copy of the
// database (plus headroom), or the data disk cannot hold one and a half
// copies (the update's own growth, plus headroom). One disk needs both.
// When free space cannot be measured (some filesystems) it warns and lets
// the update go on: only a measured shortage refuses.
func checkSpace(dbPath, dataDir, backupDir string) error {
	size := int64(0)
	for _, p := range []string{dbPath, dbPath + "-wal"} {
		if fi, err := os.Stat(p); err == nil {
			size += fi.Size()
		}
	}
	needBackup, needData := uint64(size)+headroom(size), uint64(size)*3/2+headroom(size)
	same, err := sameVolume(dataDir, backupDir)
	if err != nil {
		log.Printf("store: warning: cannot tell whether %s and %s share a disk (%v); checking each on its own", dataDir, backupDir, err)
		same = false
	}
	type need struct {
		dir string
		n   uint64
	}
	needs := []need{{backupDir, needBackup}, {dataDir, needData}}
	if same {
		needs = []need{{dataDir, needBackup + needData}}
	}
	for _, n := range needs {
		have, err := freeSpace(n.dir)
		if err != nil {
			log.Printf("store: warning: cannot measure free space on %s (%v); updating anyway (needs about %s)", n.dir, err, humanSize(n.n))
			continue
		}
		if have < n.n {
			return fmt.Errorf("%w: need %s, have %s on %s", ErrNoSpace, humanSize(n.n), humanSize(have), n.dir)
		}
	}
	return nil
}

// humanSize writes a byte count as 1.5 GB, 200.0 MB or 12 B.
func humanSize(n uint64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	f, i := float64(n)/1024, 0
	for f >= 1024 && i < 4 {
		f /= 1024
		i++
	}
	return fmt.Sprintf("%.1f %cB", f, "KMGTP"[i])
}

// cutOffBackup returns the newest raw pre-update copy for label that has a
// sha256 file and matches it, and the schema it was taken at.
func cutOffBackup(backupDir, label string) (string, int) {
	ps, _ := filepath.Glob(filepath.Join(backupDir, label+"-pre-update-*.db"))
	sort.Sort(sort.Reverse(sort.StringSlice(ps)))
	for _, p := range ps {
		if _, err := os.Stat(p + ".sha256"); err != nil {
			continue
		}
		if err := CheckBackupSum(p); err != nil {
			log.Printf("store: not resuming with %s: %v", filepath.Base(p), err)
			continue
		}
		var from, to int
		rest := strings.TrimPrefix(filepath.Base(p), label+"-pre-update-")
		if _, err := fmt.Sscanf(rest, "%04d-to-%04d-", &from, &to); err != nil {
			continue
		}
		return p, from
	}
	return "", 0
}

// pinnedBackup copies the database with VACUUM INTO to a .partial file,
// checks the copy (quick_check, read-only), writes its sha256 next to it
// and only then names it .db. It returns the backup's path.
func pinnedBackup(w *writer, backupDir, label string, from, to int) (string, error) {
	if _, err := w.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		return "", fmt.Errorf("checkpoint: %w", err)
	}
	name := fmt.Sprintf("%s-pre-update-%04d-to-%04d-%s.db", label, from, to, time.Now().UTC().Format("20060102T150405.000Z"))
	final := filepath.Join(backupDir, name)
	part := final + ".partial"
	fail := func(err error) (string, error) {
		_ = os.Remove(part)
		return "", err
	}
	if _, err := w.db.Exec("VACUUM INTO ?", part); err != nil {
		return fail(fmt.Errorf("copy: %w", err))
	}
	ro, err := sql.Open("sqlite", fileURI(part)+"?mode=ro")
	if err != nil {
		return fail(err)
	}
	res, err := quickCheck(ro)
	_ = ro.Close()
	if err != nil {
		return fail(fmt.Errorf("check: %w", err))
	}
	if res != "ok" {
		return fail(fmt.Errorf("check: %s", res))
	}
	sum, err := fileSHA256(part)
	if err != nil {
		return fail(err)
	}
	if err := files.SyncFile(part); err != nil {
		return fail(fmt.Errorf("sync: %w", err))
	}
	if err := files.WriteFileSynced(final+".sha256", []byte(sum+"  "+name+"\n"), 0o600); err != nil {
		return fail(err)
	}
	if err := os.Rename(part, final); err != nil {
		_ = os.Remove(final + ".sha256")
		return fail(err)
	}
	if err := files.SyncDir(backupDir); err != nil {
		return "", fmt.Errorf("sync %s: %w", backupDir, err)
	}
	log.Printf("store: backup before the %s update %04d -> %04d: %s (sha256 %s)", label, from, to, final, sum)
	return final, nil
}

func fileSHA256(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
