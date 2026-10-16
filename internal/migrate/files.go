package migrate

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	_ "modernc.org/sqlite" // the SQLite driver
)

// backup is a backup copied into a private temporary folder: the files a
// reader wants, by lower-case base name.
type backup struct {
	dir    string
	files  map[string]string
	lim    Limits
	closed sync.Once
}

// reading allows one backup read at a time: each copies up to MaxFile per
// file into the temporary folder.
var reading sync.Mutex

func (b *backup) close() {
	b.closed.Do(func() {
		_ = os.RemoveAll(b.dir)
		reading.Unlock()
	})
}

// has reports whether the backup holds a file called name.
func (b *backup) has(name string) bool { _, ok := b.files[strings.ToLower(name)]; return ok }

// read returns a small file's bytes (settings files).
func (b *backup) read(name string) ([]byte, bool, error) {
	p, ok := b.files[strings.ToLower(name)]
	if !ok {
		return nil, false, nil
	}
	data, err := os.ReadFile(p)
	return data, true, err
}

// openBackup copies the wanted files out of path (a zip, a folder or one
// file) into a new temporary folder. Links are refused, as are zip entries
// that leave the archive and zip bombs; every file is capped at MaxFile.
// One backup is read at a time (ErrBackupBusy).
func openBackup(src Source, wanted []string) (*backup, error) {
	if !reading.TryLock() {
		return nil, ErrBackupBusy
	}
	b, err := openBackupLocked(src, wanted)
	if err != nil {
		reading.Unlock()
	}
	return b, err
}

func openBackupLocked(src Source, wanted []string) (*backup, error) {
	lim := src.Limits.orDefault()
	want := map[string]bool{}
	for _, w := range wanted {
		want[strings.ToLower(w)] = true
	}
	p := filepath.Clean(src.Path)
	fi, err := os.Lstat(p)
	if err != nil {
		return nil, ErrNotBackup
	}
	if fi.Mode()&fs.ModeSymlink != 0 {
		return nil, ErrUnsafeBackup
	}
	dir, err := os.MkdirTemp("", "holyarrs-move-*")
	if err != nil {
		return nil, err
	}
	b := &backup{dir: dir, files: map[string]string{}, lim: lim}
	switch {
	case fi.IsDir():
		err = b.fromFolder(p, want)
	case isZip(p):
		err = b.fromZip(p, want)
	case fi.Mode().IsRegular():
		name := strings.ToLower(filepath.Base(p))
		if !want[name] {
			err = ErrNotBackup
			break
		}
		err = b.copyFile(p, name)
	default:
		err = ErrNotBackup
	}
	if err == nil && len(b.files) == 0 {
		err = ErrNotBackup
	}
	if err != nil {
		_ = os.RemoveAll(b.dir)
		return nil, err
	}
	return b, nil
}

// isZip says whether the file starts like a zip.
func isZip(p string) bool {
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	defer f.Close()
	head := make([]byte, 4)
	_, err = io.ReadFull(f, head)
	return err == nil && bytes.Equal(head, []byte("PK\x03\x04"))
}

// fromFolder copies wanted files from the folder and from its db/ and
// config/ sub-folders (Seerr keeps db/db.sqlite3, Bazarr db/ and config/).
// A sub-folder that is a link (or a Windows junction) refuses the backup.
func (b *backup) fromFolder(root string, want map[string]bool) error {
	for _, sub := range []string{"", "db", "config"} {
		d := filepath.Join(root, sub)
		if sub != "" {
			fi, err := os.Lstat(d)
			if err != nil {
				continue
			}
			if fi.Mode().Type() != fs.ModeDir {
				if fi.Mode()&(fs.ModeSymlink|fs.ModeIrregular) != 0 {
					return ErrUnsafeBackup
				}
				continue
			}
		}
		entries, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := strings.ToLower(e.Name())
			if !want[name] || b.files[name] != "" {
				continue
			}
			if e.Type()&fs.ModeSymlink != 0 {
				return ErrUnsafeBackup
			}
			if !e.Type().IsRegular() {
				continue
			}
			if err := b.copyFile(filepath.Join(d, e.Name()), name); err != nil {
				return err
			}
		}
	}
	return nil
}

// copyFile copies one regular file, at most MaxFile bytes.
func (b *backup) copyFile(from, name string) error {
	fi, err := os.Lstat(from)
	if err != nil {
		return ErrNotBackup
	}
	if !fi.Mode().IsRegular() {
		return ErrUnsafeBackup
	}
	if fi.Size() > b.lim.MaxFile {
		return ErrTooBig
	}
	in, err := os.Open(from)
	if err != nil {
		return ErrNotBackup
	}
	defer in.Close()
	// Open follows links: what was opened must be the very file checked
	// above (no link swapped in between), still regular and in size.
	got, err := in.Stat()
	if err != nil || !os.SameFile(fi, got) || !got.Mode().IsRegular() {
		return ErrUnsafeBackup
	}
	if got.Size() > b.lim.MaxFile {
		return ErrTooBig
	}
	return b.write(name, in)
}

// write stores r under name in the private folder, capped at MaxFile.
func (b *backup) write(name string, r io.Reader) error {
	to := filepath.Join(b.dir, name)
	out, err := os.OpenFile(to, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	n, err := io.Copy(out, io.LimitReader(r, b.lim.MaxFile+1))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("%w: the copy failed", ErrNotBackup)
	}
	if n > b.lim.MaxFile {
		return ErrTooBig
	}
	b.files[name] = to
	return nil
}

// fromZip copies wanted entries out of a zip. Any entry that is a link,
// holds "..", is absolute or names a drive refuses the whole archive, as
// do too many entries and entries that unpack past the size or ratio caps.
// Entries are matched by base name in any folder of the zip.
func (b *backup) fromZip(p string, want map[string]bool) error {
	zr, err := zip.OpenReader(p)
	if err != nil {
		return fmt.Errorf("%w: not a readable zip", ErrNotBackup)
	}
	defer zr.Close()
	if len(zr.File) > b.lim.MaxEntries {
		return ErrUnsafeBackup
	}
	for _, zf := range zr.File {
		name := strings.ReplaceAll(zf.Name, `\`, "/")
		if strings.HasPrefix(name, "/") || strings.Contains(name, ":") || zf.Mode()&fs.ModeSymlink != 0 {
			return ErrUnsafeBackup
		}
		for _, part := range strings.Split(name, "/") {
			if part == ".." {
				return ErrUnsafeBackup
			}
		}
	}
	for _, zf := range zr.File {
		if zf.FileInfo().IsDir() {
			continue
		}
		base := strings.ToLower(path.Base(strings.ReplaceAll(zf.Name, `\`, "/")))
		if !want[base] || b.files[base] != "" {
			continue
		}
		if zf.UncompressedSize64 > uint64(b.lim.MaxFile) {
			return ErrTooBig
		}
		if zf.CompressedSize64 > 0 && zf.UncompressedSize64/zf.CompressedSize64 > b.lim.MaxRatio {
			return ErrUnsafeBackup
		}
		rc, err := zf.Open()
		if err != nil {
			return fmt.Errorf("%w: a zip entry could not be read", ErrNotBackup)
		}
		// The header sizes may lie: write caps at MaxFile, and the ratio is
		// checked again on what really came out.
		err = b.write(base, rc)
		rc.Close()
		if err != nil {
			return err
		}
		if fi, err := os.Stat(b.files[base]); err == nil && zf.CompressedSize64 > 0 && uint64(fi.Size())/zf.CompressedSize64 > b.lim.MaxRatio {
			return ErrUnsafeBackup
		}
	}
	return nil
}

// sqliteMagic starts every SQLite database file.
var sqliteMagic = []byte("SQLite format 3\x00")

// db opens the copied database called name read only. It refuses a file
// that is not SQLite.
func (b *backup) db(name string) (*sql.DB, error) {
	p, ok := b.files[strings.ToLower(name)]
	if !ok {
		return nil, ErrNotBackup
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	head := make([]byte, len(sqliteMagic))
	_, err = io.ReadFull(f, head)
	f.Close()
	if err != nil || !bytes.Equal(head, sqliteMagic) {
		return nil, ErrNotSQLite
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(p)+"?mode=ro&immutable=1&_pragma=query_only(1)")
	if err != nil {
		return nil, ErrNotSQLite
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, ErrNotSQLite
	}
	var n int
	if err := db.QueryRow("SELECT count(*) FROM sqlite_master").Scan(&n); err != nil {
		db.Close()
		return nil, ErrNotSQLite
	}
	return db, nil
}

// table reads chosen columns of a table that may be missing or older:
// a missing table gives no rows and a note, a missing column is left out
// of each row. Keys in the rows are the wanted names as given.
type table struct {
	db    *sql.DB
	lim   Limits
	notes *[]string
}

// columns lists a table's columns by lower-case name.
func (t table) columns(ctx context.Context, name string) (map[string]string, error) {
	rows, err := t.db.QueryContext(ctx, "SELECT name FROM pragma_table_info(?)", name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		out[strings.ToLower(c)] = c
	}
	return out, rows.Err()
}

// exists reports whether the table is there.
func (t table) exists(ctx context.Context, name string) bool {
	cols, err := t.columns(ctx, name)
	return err == nil && len(cols) > 0
}

// rows reads the wanted columns of name, at most MaxRows rows.
func (t table) rows(ctx context.Context, name string, wanted ...string) ([]map[string]any, error) {
	cols, err := t.columns(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotSQLite, err)
	}
	if len(cols) == 0 {
		*t.notes = append(*t.notes, "the backup has no "+name+" table; skipped")
		return nil, nil
	}
	var sel, keys []string
	for _, w := range wanted {
		if real, ok := cols[strings.ToLower(w)]; ok {
			sel = append(sel, `"`+strings.ReplaceAll(real, `"`, `""`)+`"`)
			keys = append(keys, w)
		}
	}
	if len(sel) == 0 {
		return nil, nil
	}
	q := "SELECT " + strings.Join(sel, ", ") + ` FROM "` + strings.ReplaceAll(name, `"`, `""`) + `" LIMIT ` + strconv.Itoa(t.lim.MaxRows+1)
	rs, err := t.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotSQLite, err)
	}
	defer rs.Close()
	var out []map[string]any
	for rs.Next() {
		vals := make([]any, len(sel))
		ptrs := make([]any, len(sel))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rs.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrNotSQLite, err)
		}
		if len(out) == t.lim.MaxRows {
			return nil, ErrTooBig
		}
		m := map[string]any{}
		for i, k := range keys {
			if bs, ok := vals[i].([]byte); ok {
				vals[i] = string(bs)
			}
			m[k] = vals[i]
		}
		out = append(out, m)
	}
	return out, rs.Err()
}

// maxVersion reads the highest number in a version table (VersionInfo).
func (t table) maxVersion(ctx context.Context, name, col string) int {
	if !t.exists(ctx, name) {
		return 0
	}
	var v sql.NullInt64
	_ = t.db.QueryRowContext(ctx, `SELECT max("`+col+`") FROM "`+name+`"`).Scan(&v)
	return int(v.Int64)
}
