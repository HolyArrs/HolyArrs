package main

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/CoderLuii/holyarrs-metadata/internal/imgstore"

	_ "modernc.org/sqlite"
)

// musicKinds is the allow-list of picture kinds this command may touch.
// TV kinds (title-banner, title-logo, title-wide) and person are never in it.
var musicKinds = []string{imgstore.KindArtist, imgstore.KindAlbum, imgstore.KindWide, imgstore.KindLogo}

// picturesDir, when set, is where the picture files are counted (a
// read-only mount, when the database files are a copy). Deletes ignore it.
var picturesDir string

func isMusicKind(k string) bool {
	for _, m := range musicKinds {
		if m == k {
			return true
		}
	}
	return false
}

// target is one table (or the music part of one) that the wipe clears.
type target struct {
	File  string // path under the data dir
	Table string
	// Where limits the rows (empty: all rows); Args are its values.
	Where string
	Args  []any
	// Key is an SQL expression naming a row, for the id sample.
	Key string
	// Bytes is an SQL expression for a row's stored bytes ("" counts none).
	Bytes string
}

func kindIn(col string) (string, []any) {
	ph := make([]string, len(musicKinds))
	args := make([]any, len(musicKinds))
	for i, k := range musicKinds {
		ph[i], args[i] = "?", k
	}
	return col + " IN (" + strings.Join(ph, ",") + ")", args
}

// keyKinds matches queue keys such as "img:album:<mbid>" and "big:logo:<mbid>".
func keyKinds() (string, []any) {
	var parts []string
	var args []any
	for _, p := range []string{"img", "big"} {
		for _, k := range musicKinds {
			parts = append(parts, "key LIKE ?")
			args = append(args, p+":"+k+":%")
		}
	}
	return "(" + strings.Join(parts, " OR ") + ")", args
}

func targets() []target {
	kw, ka := kindIn("kind")
	qw, qa := keyKinds()
	return []target{
		{File: "img/images.sqlite", Table: "images", Where: kw, Args: ka, Key: "kind||':'||mbid", Bytes: "bytes"},
		{File: "img/images.sqlite", Table: "misses", Where: kw, Args: ka, Key: "kind||':'||mbid"},
		{File: "img/images.sqlite", Table: "source_ids", Where: kw, Args: ka, Key: "kind||':'||mbid||':'||source"},
		{File: "fetchq-images.sqlite", Table: "fetch_jobs", Where: qw, Args: qa, Key: "key"},
		{File: "fetchq-musicbrainz.sqlite", Table: "fetch_jobs", Key: "key"},
		{File: "music.sqlite", Table: "chart_days", Key: "day||':'||store", Bytes: "length(body)"},
		{File: "music.sqlite", Table: "matches", Key: "kind||':'||name||':'||artist"},
		{File: "music.sqlite", Table: "lists", Key: "kind||':'||region", Bytes: "length(body)"},
		{File: "newmusic.sqlite", Table: "deezer_map", Key: "deezer_id"},
		{File: "newmusic.sqlite", Table: "windows", Key: "window", Bytes: "length(body)"},
		{File: "similar.sqlite", Table: "similar", Key: "mbid", Bytes: "length(body)"},
		{File: "similar.sqlite", Table: "artist_data", Key: "mbid", Bytes: "length(body)"},
		{File: "musicdata.sqlite", Table: "answers", Key: "k", Bytes: "length(body)"},
		{File: "discog.sqlite", Table: "artists", Key: "mbid", Bytes: "length(body)+length(walk)"},
		{File: "discog.sqlite", Table: "albums", Key: "mbid", Bytes: "length(body)+length(walk)"},
		{File: "discog.sqlite", Table: "refs", Key: "ref||':'||owner"},
		{File: "discog.sqlite", Table: "counters", Key: "name"},
		{File: "kworb.sqlite", Table: "kept", Key: "k", Bytes: "length(body)"},
		{File: "kworb.sqlite", Table: "pages", Key: "spotify"},
		{File: "kworb.sqlite", Table: "pairs", Key: "spotify"},
		{File: "kworb.sqlite", Table: "tried", Key: "k"},
		{File: "kworb.sqlite", Table: "opened", Key: "mbid"},
		{File: "kworb.sqlite", Table: "due", Key: "job"},
		{File: "musictags.sqlite", Table: "mt_cache", Key: "url", Bytes: "length(body)"},
		{File: "musictags.sqlite", Table: "mt_bundle", Key: "version", Bytes: "length(body)"},
		{File: "packages.sqlite", Table: "packages", Key: "mbid", Bytes: "length(body)"},
		{File: "packages.sqlite", Table: "wanted", Key: "mbid"},
		{File: "packages.sqlite", Table: "seq", Key: "id"},
		{File: "herovideo.sqlite", Table: "videos", Key: "mbid", Bytes: "length(body)"},
		{File: "herovideo.sqlite", Table: "artists", Key: "mbid"},
		{File: "herovideo.sqlite", Table: "channels", Key: "mbid||':'||channel"},
		{File: "herovideo.sqlite", Table: "cands", Key: "mbid||':'||video"},
		{File: "herovideo.sqlite", Table: "seen", Key: "channel||':'||video"},
		{File: "herovideo.sqlite", Table: "verdicts", Key: "video"},
		{File: "genretiles.sqlite", Table: "genre_photos", Key: "slug||':'||idx", Bytes: "length(jpeg)"},
		{File: "lists.sqlite", Table: "lists_source", Key: "key"},
		{File: "lists.sqlite", Table: "lists_day", Key: "key||':'||day", Bytes: "length(body)"},
		{File: "lists.sqlite", Table: "lists_season", Key: "key"},
		{File: "lists.sqlite", Table: "matches", Key: "kind||':'||name||':'||artist"},
		{File: "lists.sqlite", Table: "tracks", Key: "id", Bytes: "length(body)"},
	}
}

// snapshot holds row counts of what is kept, by "file/table"; the wipe must
// leave it as it was.
type snapshot map[string]int64

func (s snapshot) equal(o snapshot) (bool, []string) {
	var diff []string
	for k, v := range s {
		if o[k] != v {
			diff = append(diff, fmt.Sprintf("%s: %d -> %d", k, v, o[k]))
		}
	}
	for k, v := range o {
		if _, ok := s[k]; !ok {
			diff = append(diff, fmt.Sprintf("%s: absent -> %d", k, v))
		}
	}
	sort.Strings(diff)
	return len(diff) == 0, diff
}

// openDB opens a sqlite file read-only (rw for the real wipe).
func openDB(path string, rw bool) (*sql.DB, error) {
	mode := "ro"
	if rw {
		mode = "rw"
	}
	u := url.URL{Scheme: "file", Opaque: filepath.ToSlash(path), RawQuery: "mode=" + mode + "&_pragma=busy_timeout(10000)"}
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, nil
}

func exists(path string) bool { _, err := os.Stat(path); return err == nil }

func tableExists(db *sql.DB, table string) bool {
	var n int
	return db.QueryRow(`SELECT COUNT(*) FROM sqlite_schema WHERE type='table' AND name=?`, table).Scan(&n) == nil && n > 0
}

func q(name string) string { return `"` + strings.ReplaceAll(name, `"`, `""`) + `"` }

func (t target) where() string {
	if t.Where == "" {
		return ""
	}
	return " WHERE " + t.Where
}

// Measure is what one target holds now.
type Measure struct {
	T      target
	Absent bool
	Rows   int64
	Bytes  int64
	Sample []string
}

func measure(db *sql.DB, t target, sample int) (Measure, error) {
	m := Measure{T: t}
	if !tableExists(db, t.Table) {
		m.Absent = true
		return m, nil
	}
	b := "0"
	if t.Bytes != "" {
		b = "COALESCE(SUM(" + t.Bytes + "),0)"
	}
	if err := db.QueryRow("SELECT COUNT(*), "+b+" FROM "+q(t.Table)+t.where(), t.Args...).Scan(&m.Rows, &m.Bytes); err != nil {
		return m, fmt.Errorf("%s/%s: %w", t.File, t.Table, err)
	}
	if sample > 0 && m.Rows > 0 {
		rows, err := db.Query("SELECT "+t.Key+" FROM "+q(t.Table)+t.where()+" LIMIT ?", append(append([]any{}, t.Args...), sample)...)
		if err != nil {
			return m, fmt.Errorf("%s/%s sample: %w", t.File, t.Table, err)
		}
		defer rows.Close()
		for rows.Next() {
			var s any
			if rows.Scan(&s) == nil {
				m.Sample = append(m.Sample, fmt.Sprint(s))
			}
		}
	}
	return m, nil
}

func sqliteFiles(dir string) []string {
	var files []string
	for _, pat := range []string{"*.sqlite", "img/*.sqlite"} {
		m, _ := filepath.Glob(filepath.Join(dir, pat))
		files = append(files, m...)
	}
	sort.Strings(files)
	return files
}

// kept counts every row the wipe must not touch: whole tables that are not
// targets, and the rows of target tables outside their Where.
func kept(dir string) (snapshot, error) {
	snap := snapshot{}
	byTable := map[string]target{}
	for _, t := range targets() {
		byTable[filepath.ToSlash(t.File)+"/"+t.Table] = t
	}
	for _, f := range sqliteFiles(dir) {
		rel, _ := filepath.Rel(dir, f)
		rel = filepath.ToSlash(rel)
		db, err := openDB(f, false)
		if err != nil {
			return nil, err
		}
		names, err := tableNames(db)
		if err != nil {
			db.Close()
			return nil, fmt.Errorf("%s: %w", rel, err)
		}
		for _, n := range names {
			key := rel + "/" + n
			var c int64
			if t, ok := byTable[key]; ok {
				if t.Where == "" {
					continue // wiped whole: nothing of it is kept
				}
				err = db.QueryRow("SELECT COUNT(*) FROM "+q(n)+" WHERE NOT ("+t.Where+")", t.Args...).Scan(&c)
			} else {
				err = db.QueryRow("SELECT COUNT(*) FROM " + q(n)).Scan(&c)
			}
			if err != nil {
				db.Close()
				return nil, fmt.Errorf("%s: %w", key, err)
			}
			snap[key] = c
		}
		db.Close()
	}
	return snap, nil
}

func tableNames(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`SELECT name FROM sqlite_schema WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// dirStat counts files and bytes under a picture kind's folder.
func dirStat(root string) (files, bytes int64) {
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, e := d.Info(); e == nil {
			files++
			bytes += info.Size()
		}
		return nil
	})
	return
}

// Plan is a full dry-run result.
type Plan struct {
	Dir      string
	Measures []Measure
	Dirs     map[string][2]int64 // kind -> files, bytes on disk
	Kept     snapshot
	Unlisted []string // tables in music files that are not targets
}

func plan(dir string, sample int) (*Plan, error) {
	p := &Plan{Dir: dir, Dirs: map[string][2]int64{}}
	open := map[string]*sql.DB{}
	defer func() {
		for _, d := range open {
			d.Close()
		}
	}()
	listed := map[string]bool{}
	for _, t := range targets() {
		listed[filepath.ToSlash(t.File)+"/"+t.Table] = true
		path := filepath.Join(dir, t.File)
		if !exists(path) {
			p.Measures = append(p.Measures, Measure{T: t, Absent: true})
			continue
		}
		db := open[t.File]
		if db == nil {
			var err error
			if db, err = openDB(path, false); err != nil {
				return nil, err
			}
			open[t.File] = db
		}
		m, err := measure(db, t, sample)
		if err != nil {
			return nil, err
		}
		p.Measures = append(p.Measures, m)
	}
	for f, db := range open {
		if strings.HasPrefix(f, "img/") || strings.HasPrefix(f, "fetchq-images") {
			continue
		}
		names, err := tableNames(db)
		if err != nil {
			return nil, err
		}
		for _, n := range names {
			if !listed[filepath.ToSlash(f)+"/"+n] {
				p.Unlisted = append(p.Unlisted, f+"/"+n)
			}
		}
	}
	sort.Strings(p.Unlisted)
	for _, k := range musicKinds {
		root := filepath.Join(dir, "img")
		if picturesDir != "" {
			root = picturesDir
		}
		f, b := dirStat(filepath.Join(root, k))
		p.Dirs[k] = [2]int64{f, b}
	}
	var err error
	if p.Kept, err = kept(dir); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *Plan) print(w io.Writer, applied bool) {
	verb := "would delete"
	if applied {
		verb = "deleted"
	}
	fmt.Fprintf(w, "musicwipe data dir: %s\n", p.Dir)
	fmt.Fprintf(w, "picture kinds on the allow-list: %s\n\n", strings.Join(musicKinds, ", "))
	fmt.Fprintf(w, "%-48s %12s %14s\n", "table (rows "+verb+")", "rows", "bytes")
	var rows, bytes int64
	for _, m := range p.Measures {
		name := m.T.File + "/" + m.T.Table
		if m.T.Where != "" {
			name += " [music only]"
		}
		if m.Absent {
			fmt.Fprintf(w, "%-48s %12s %14s\n", name, "absent", "-")
			continue
		}
		fmt.Fprintf(w, "%-48s %12d %14d\n", name, m.Rows, m.Bytes)
		rows += m.Rows
		bytes += m.Bytes
		if len(m.Sample) > 0 {
			fmt.Fprintf(w, "    sample ids: %s\n", strings.Join(m.Sample, "  "))
		}
	}
	fmt.Fprintf(w, "%-48s %12d %14d\n\n", "TOTAL in tables", rows, bytes)
	fmt.Fprintln(w, "picture files on disk (img/<kind>/):")
	var fb int64
	for _, k := range musicKinds {
		d := p.Dirs[k]
		fmt.Fprintf(w, "  %-8s %10d files %14d bytes\n", k, d[0], d[1])
		fb += d[1]
	}
	fmt.Fprintf(w, "  %-8s %10s       %14d bytes\n\n", "total", "", fb)
	if len(p.Unlisted) > 0 {
		fmt.Fprintln(w, "tables in music files that are NOT wiped (not on the list):")
		for _, u := range p.Unlisted {
			fmt.Fprintf(w, "  %s\n", u)
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w, "kept (movie/TV and everything else), rows per table:")
	keys := make([]string, 0, len(p.Kept))
	for k := range p.Kept {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(w, "  %-48s %12d\n", k, p.Kept[k])
	}
}

// keptPictures reports the pictures kept: rows, bytes and misses per kind
// that is not on the allow-list.
func keptPictures(dir string) (map[string][3]int64, error) {
	path := filepath.Join(dir, "img", "images.sqlite")
	out := map[string][3]int64{}
	if !exists(path) {
		return out, nil
	}
	db, err := openDB(path, false)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	w, a := kindIn("kind")
	rows, err := db.Query(`SELECT kind, COUNT(*), COALESCE(SUM(bytes),0) FROM images WHERE NOT (`+w+`) GROUP BY kind`, a...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var k string
		var c, b int64
		if err := rows.Scan(&k, &c, &b); err != nil {
			rows.Close()
			return nil, err
		}
		out[k] = [3]int64{c, b, 0}
	}
	rows.Close()
	rows, err = db.Query(`SELECT kind, COUNT(*) FROM misses WHERE NOT (`+w+`) GROUP BY kind`, a...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		var c int64
		if err := rows.Scan(&k, &c); err != nil {
			return nil, err
		}
		v := out[k]
		v[2] = c
		out[k] = v
	}
	return out, nil
}

// apply deletes. Only reached with an explicit -apply; one transaction per file.
func apply(dir string) error {
	for _, k := range musicKinds {
		if !isMusicKind(k) || strings.Contains(k, "-") || k == imgstore.KindPerson {
			return errors.New("refusing: a kind outside the allow-list")
		}
	}
	before, err := kept(dir)
	if err != nil {
		return err
	}
	byFile := map[string][]target{}
	for _, t := range targets() {
		byFile[t.File] = append(byFile[t.File], t)
	}
	files := make([]string, 0, len(byFile))
	for f := range byFile {
		files = append(files, f)
	}
	sort.Strings(files)
	for _, f := range files {
		path := filepath.Join(dir, f)
		if !exists(path) {
			continue
		}
		db, err := openDB(path, true)
		if err != nil {
			return err
		}
		tx, err := db.Begin()
		if err != nil {
			db.Close()
			return err
		}
		for _, t := range byFile[f] {
			var n int
			if tx.QueryRow(`SELECT COUNT(*) FROM sqlite_schema WHERE type='table' AND name=?`, t.Table).Scan(&n) != nil || n == 0 {
				continue
			}
			if _, err := tx.Exec("DELETE FROM "+q(t.Table)+t.where(), t.Args...); err != nil {
				tx.Rollback()
				db.Close()
				return fmt.Errorf("%s/%s: %w", f, t.Table, err)
			}
		}
		if err := tx.Commit(); err != nil {
			db.Close()
			return err
		}
		db.Close()
	}
	for _, k := range musicKinds {
		if err := os.RemoveAll(filepath.Join(dir, "img", k)); err != nil {
			return err
		}
	}
	after, err := kept(dir)
	if err != nil {
		return err
	}
	if ok, diff := before.equal(after); !ok {
		return fmt.Errorf("kept rows changed: %s", strings.Join(diff, "; "))
	}
	return nil
}
