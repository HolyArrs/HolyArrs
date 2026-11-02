// Package mbdump builds the slim MusicBrainz copy: it streams the dump
// archives (tar.bz2, decompressed by an lbzip2 pipe), keeps only the
// tables music pages read and loads them into one SQLite file. Every
// table carries one source tag (dump date, schema sequence, rows) in
// dump_meta. The build always writes into a fresh directory of its own,
// never into a store the running server uses.
package mbdump

import (
	"archive/tar"
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/srctag"

	_ "modernc.org/sqlite"
)

// DBName is the slim copy's file inside its build directory.
const DBName = "mb.sqlite"

// ErrSchema: the dump's SCHEMA_SEQUENCE is not the pinned one.
var ErrSchema = errors.New("mbdump: schema sequence mismatch")

// Decompress starts tool (lbzip2, or bzip2 where lbzip2 is missing) to
// decompress r; "" or "none" reads r as it is.
func Decompress(ctx context.Context, tool string, r io.Reader) (io.ReadCloser, error) {
	if tool == "" || tool == "none" {
		return io.NopCloser(r), nil
	}
	cmd := exec.CommandContext(ctx, tool, "-dc")
	cmd.Stdin = r
	cmd.Stderr = os.Stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &procReader{ReadCloser: out, cmd: cmd}, nil
}

type procReader struct {
	io.ReadCloser
	cmd *exec.Cmd
}

func (p *procReader) Close() error {
	_, _ = io.Copy(io.Discard, p.ReadCloser) // the tar's end padding
	p.ReadCloser.Close()
	return p.cmd.Wait()
}

// Options steer one build.
type Options struct {
	Dir      string          // build directory (made fresh; must not exist or be empty)
	DumpDate string          // e.g. 20261028-002228
	Only     map[string]bool // keep only these tables (nil: all of Tables)
	Guard    *Guard          // checked while loading (nil: none)
	Logf     func(string, ...any)
}

// Result is what a build loaded.
type Result struct {
	Schema int
	Rows   map[string]int64 // rows read from the dump per kept table
	Took   time.Duration
}

// Builder loads archives into one slim copy.
type Builder struct {
	o      Options
	db     *sql.DB
	res    Result
	schema int
	start  time.Time
	bad    []string
}

// NewBuilder opens a fresh slim copy in o.Dir.
func NewBuilder(o Options) (*Builder, error) {
	if o.Logf == nil {
		o.Logf = func(string, ...any) {}
	}
	if ents, err := os.ReadDir(o.Dir); err == nil && len(ents) > 0 {
		return nil, fmt.Errorf("mbdump: build directory %s is not empty", o.Dir)
	}
	if err := os.MkdirAll(o.Dir, 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", filepath.Join(o.Dir, DBName))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA journal_mode=OFF; PRAGMA synchronous=OFF; PRAGMA locking_mode=EXCLUSIVE; PRAGMA cache_size=-262144; PRAGMA temp_store=FILE;`); err != nil {
		db.Close()
		return nil, err
	}
	if err := srctag.Migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Builder{o: o, db: db, res: Result{Rows: map[string]int64{}}, start: time.Now()}, nil
}

// Read loads the kept tables of one decompressed tar stream. The
// archive's SCHEMA_SEQUENCE must come before any table and match.
func (b *Builder) Read(ctx context.Context, r io.Reader) error {
	tr := tar.NewReader(r)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		name := path.Clean(h.Name)
		switch {
		case name == "SCHEMA_SEQUENCE":
			raw, err := io.ReadAll(io.LimitReader(tr, 64))
			if err != nil {
				return err
			}
			n, err := strconv.Atoi(strings.TrimSpace(string(raw)))
			if err != nil || n != SchemaSequence {
				return fmt.Errorf("%w: dump %q, pinned %d", ErrSchema, strings.TrimSpace(string(raw)), SchemaSequence)
			}
			b.schema = n
		case strings.HasPrefix(name, "mbdump/"):
			tbl := strings.TrimPrefix(name, "mbdump/")
			t, keep := Tables[tbl]
			if !keep || (b.o.Only != nil && !b.o.Only[tbl]) {
				continue
			}
			if b.schema == 0 {
				return fmt.Errorf("%w: table %s before SCHEMA_SEQUENCE", ErrSchema, tbl)
			}
			if err := b.load(ctx, tbl, t, tr); err != nil {
				return err
			}
		}
	}
}

// errWidth: a table's rows don't have the pinned column count.
type errWidth struct {
	table      string
	got, wants int
}

func (e errWidth) Error() string {
	return fmt.Sprintf("%s has %d columns, pinned %d", e.table, e.got, e.wants)
}

func (b *Builder) load(ctx context.Context, name string, t table, r io.Reader) error {
	started := time.Now()
	names := t.names()
	if _, err := b.db.Exec(`DROP TABLE IF EXISTS "` + name + `"; CREATE TABLE "` + name + `"(` + strings.Join(t.cols, ", ") + `)`); err != nil {
		return err
	}
	ins := `INSERT INTO "` + name + `" VALUES(` + strings.TrimSuffix(strings.Repeat("?,", len(names)), ",") + `)`
	tx, err := b.db.Begin()
	if err != nil {
		return err
	}
	st, err := tx.Prepare(ins)
	if err != nil {
		tx.Rollback()
		return err
	}
	br := bufio.NewReaderSize(r, 1<<20)
	args := make([]any, len(names))
	var n int64
	for {
		line, err := br.ReadString('\n')
		if err != nil && err != io.EOF {
			tx.Rollback()
			return err
		}
		if line == "" && err == io.EOF {
			break
		}
		fields := Fields(strings.TrimSuffix(line, "\n"))
		if len(fields) != len(names) {
			tx.Rollback()
			b.bad = append(b.bad, errWidth{name, len(fields), len(names)}.Error())
			b.o.Logf("mbdump: %s skipped: %d columns, pinned %d", name, len(fields), len(names))
			_, _ = b.db.Exec(`DROP TABLE IF EXISTS "` + name + `"`)
			_, _ = io.Copy(io.Discard, br)
			return nil
		}
		for i, f := range fields {
			if f.Null {
				args[i] = nil
			} else {
				args[i] = f.S
			}
		}
		if _, err := st.Exec(args...); err != nil {
			tx.Rollback()
			return fmt.Errorf("mbdump: %s row %d: %w", name, n+1, err)
		}
		n++
		if n%200000 == 0 {
			st.Close()
			if err := tx.Commit(); err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if b.o.Guard != nil && n%2000000 == 0 {
				if _, err := b.o.Guard.CheckBuild(ctx); err != nil {
					return err
				}
			}
			if tx, err = b.db.Begin(); err != nil {
				return err
			}
			if st, err = tx.Prepare(ins); err != nil {
				tx.Rollback()
				return err
			}
		}
		if err == io.EOF {
			break
		}
	}
	st.Close()
	if err := tx.Commit(); err != nil {
		return err
	}
	for _, col := range t.indexes {
		if _, err := b.db.Exec(`CREATE INDEX "` + name + `_` + col + `" ON "` + name + `"(` + col + `)`); err != nil {
			return err
		}
	}
	b.res.Rows[name] = n
	if err := srctag.PutDump(ctx, b.db, srctag.DumpMeta{Table: name, DumpDate: b.o.DumpDate, SchemaSequence: b.schema, Rows: n}); err != nil {
		return err
	}
	b.o.Logf("mbdump: %s %d rows in %s", name, n, time.Since(started).Round(time.Second))
	return nil
}

// Finish checks the loaded counts against the dump and closes the copy.
func (b *Builder) Finish(ctx context.Context) (Result, error) {
	defer b.db.Close()
	b.res.Schema = b.schema
	b.res.Took = time.Since(b.start)
	if len(b.bad) > 0 {
		return b.res, fmt.Errorf("mbdump: column counts differ from the pinned schema: %s", strings.Join(b.bad, "; "))
	}
	if err := verifyDB(ctx, b.db, b.res.Rows); err != nil {
		return b.res, err
	}
	if b.o.Guard != nil {
		if _, err := b.o.Guard.CheckBuild(ctx); err != nil {
			return b.res, err
		}
	}
	_, err := b.db.Exec(`PRAGMA optimize`)
	return b.res, err
}

// Verify opens a built copy and checks every table's row count against
// the dump's (dump_meta), and that every wanted table is there.
func Verify(ctx context.Context, dir string, want []string) (map[string]int64, error) {
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, DBName)+"?mode=ro")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT tbl, rows FROM dump_meta`)
	if err != nil {
		return nil, err
	}
	meta := map[string]int64{}
	for rows.Next() {
		var t string
		var n int64
		if err := rows.Scan(&t, &n); err != nil {
			rows.Close()
			return nil, err
		}
		meta[t] = n
	}
	rows.Close()
	for _, t := range want {
		if _, ok := meta[t]; !ok {
			return meta, fmt.Errorf("mbdump: table %s missing from the copy", t)
		}
	}
	return meta, verifyDB(ctx, db, meta)
}

func verifyDB(ctx context.Context, db *sql.DB, want map[string]int64) error {
	names := make([]string, 0, len(want))
	for t := range want {
		names = append(names, t)
	}
	sort.Strings(names)
	for _, t := range names {
		var n int64
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM "`+t+`"`).Scan(&n); err != nil {
			return err
		}
		if n != want[t] {
			return fmt.Errorf("mbdump: %s has %d rows, the dump %d", t, n, want[t])
		}
	}
	return nil
}
