// Command mbimport builds the slim MusicBrainz copy in its own directory,
// as a separate process from the metadata server.
//
//	mbimport -root /data/music-next -date 20261028-002228 [-tables artist] [-no-swap]
//	mbimport -root /data/music-next -watch 6h
//
// Archives come from -base (the MetaBrainz full export) or from local files
// with -archive (repeatable, mbdump.tar.bz2 first).
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/imgstore"
	"github.com/CoderLuii/holyarrs-metadata/internal/mbdump"

	_ "modernc.org/sqlite"
)

type archives []string

func (a *archives) String() string     { return strings.Join(*a, ",") }
func (a *archives) Set(v string) error { *a = append(*a, v); return nil }

func main() {
	var (
		root     = flag.String("root", "", "directory every slim copy lives under")
		base     = flag.String("base", "https://data.metabrainz.org/pub/musicbrainz/data/fullexport", "dump directory")
		date     = flag.String("date", "", "dump date to build (default: LATEST)")
		files    archives
		names    = flag.String("archives", "mbdump.tar.bz2,mbdump-derived.tar.bz2", "archive names under base/<date>")
		tables   = flag.String("tables", "", "comma-separated tables to keep (default: all kept tables)")
		tool     = flag.String("bzip", "lbzip2", "decompressor (lbzip2, bzip2, none)")
		noSwap   = flag.Bool("no-swap", false, "build and verify only; never move CURRENT")
		watch    = flag.Duration("watch", 0, "check the dump index this often and refresh by swap")
		imgDB    = flag.String("img-db", "", "picture store database, read-only, for the music picture bytes")
		picBytes = flag.Int64("picture-bytes", -1, "music picture bytes when -img-db is not given")
		capGB    = flag.Float64("cap-gb", 90, "music data cap in GB (copies plus music pictures)")
		report   = flag.String("report", "", "write the build result as JSON here")
		cache    = flag.String("cache", "", "download archives here first (resumed on a dropped connection), then read them")
	)
	flag.Var(&files, "archive", "local archive (repeatable); used instead of base")
	flag.Parse()
	if *root == "" {
		log.Fatal("mbimport: -root is required")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := os.MkdirAll(*root, 0o755); err != nil {
		log.Fatal(err)
	}
	// The build cap stays 10 GB over -cap-gb (100 by default), so raising
	// -cap-gb also lets a refresh build.
	guard := &mbdump.Guard{Root: *root, Limit: int64(*capGB * (1 << 30)), BuildLimit: int64((*capGB + 10) * (1 << 30)), Pictures: pictures(*imgDB, *picBytes)}
	var only map[string]bool
	var want []string
	if *tables != "" {
		only = map[string]bool{}
		for _, t := range strings.Split(*tables, ",") {
			only[strings.TrimSpace(t)] = true
			want = append(want, strings.TrimSpace(t))
		}
	} else {
		for t := range mbdump.Tables {
			want = append(want, t)
		}
		sort.Strings(want)
	}
	client := &http.Client{Timeout: 0}
	build := func(ctx context.Context, d, dir string) (mbdump.Result, error) {
		b, err := mbdump.NewBuilder(mbdump.Options{Dir: dir, DumpDate: d, Only: only, Guard: guard, Logf: log.Printf})
		if err != nil {
			return mbdump.Result{}, err
		}
		srcs := []string(files)
		if len(srcs) == 0 {
			for _, n := range strings.Split(*names, ",") {
				srcs = append(srcs, strings.TrimSuffix(*base, "/")+"/"+d+"/"+strings.TrimSpace(n))
			}
		}
		if *cache != "" {
			pruneCache(*cache, d)
		}
		for _, src := range srcs {
			if *cache != "" && strings.HasPrefix(src, "http") {
				local := filepath.Join(*cache, d+"-"+path.Base(src))
				if err := download(ctx, client, src, local); err != nil {
					return mbdump.Result{}, fmt.Errorf("%s: %w", src, err)
				}
				src = local
			}
			if err := readOne(ctx, b, client, src, *tool); err != nil {
				return mbdump.Result{}, fmt.Errorf("%s: %w", src, err)
			}
		}
		return b.Finish(ctx)
	}
	var index mbdump.Index = mbdump.HTTPIndex{Base: *base, Client: client}
	if *date != "" {
		index = fixed(*date)
	}
	r := &mbdump.Refresher{Root: *root, Index: index, Build: build, Want: want, Guard: guard, NoSwap: *noSwap, Logf: log.Printf}
	if *watch > 0 {
		r.Watch(ctx, *watch)
		return
	}
	start := time.Now()
	// A copy an earlier run retired goes before this one builds.
	if err := r.Sweep(); err != nil {
		log.Printf("mbimport: sweep: %v", err)
	}
	d, swapped, err := r.Once(ctx)
	if err != nil {
		log.Fatal(err)
	}
	if !swapped && !*noSwap {
		// The newest dump is already in use; a copy built before a table
		// was added would fail the check below for nothing.
		log.Printf("mbimport: %s is already current", d)
		return
	}
	dir := *root + "/" + d
	rows, err := mbdump.Verify(ctx, dir, want)
	if err != nil {
		log.Fatal(err)
	}
	size, _ := mbdump.DirBytes(dir)
	used, gerr := guard.Check(ctx)
	out := map[string]any{"date": d, "swapped": swapped, "seconds": int(time.Since(start).Seconds()), "copy_bytes": size,
		"guard_bytes": used, "guard_ok": gerr == nil, "rows": rows}
	raw, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(raw))
	if *report != "" {
		_ = os.WriteFile(*report, raw, 0o644)
	}
}

// pruneCache removes the archives (and .part leftovers) of other dump
// dates from the cache before date's download, so only one dump is kept;
// date's own files stay for a resume. Remove errors are only logged.
func pruneCache(dir, date string) {
	old, _ := filepath.Glob(filepath.Join(dir, "*-mbdump*.tar.bz2*"))
	for _, f := range old {
		if strings.HasPrefix(filepath.Base(f), date+"-") {
			continue
		}
		if err := os.Remove(f); err != nil {
			log.Printf("mbimport: cache: %v", err)
		} else {
			log.Printf("mbimport: cache: removed %s", filepath.Base(f))
		}
	}
}

// download fetches url into dst, resuming with a Range request when the
// connection drops; a finished file is kept as it is.
func download(ctx context.Context, c *http.Client, url, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(dst); err == nil {
		return nil
	}
	part := dst + ".part"
	var last error
	for try := 0; try < 20; try++ {
		if try > 0 {
			log.Printf("mbimport: download %s: %v; resuming", url, last)
			time.Sleep(time.Duration(try) * 5 * time.Second)
		}
		f, err := os.OpenFile(part, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return err
		}
		have, _ := f.Seek(0, io.SeekEnd)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			f.Close()
			return err
		}
		if have > 0 {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", have))
		}
		resp, err := c.Do(req)
		if err != nil {
			f.Close()
			last = err
			continue
		}
		switch {
		case have > 0 && resp.StatusCode == http.StatusRequestedRangeNotSatisfiable:
			resp.Body.Close()
			f.Close()
			return os.Rename(part, dst)
		case have > 0 && resp.StatusCode != http.StatusPartialContent, resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent:
			resp.Body.Close()
			f.Close()
			if resp.StatusCode == http.StatusOK { // no ranges: start over
				os.Remove(part)
				last = fmt.Errorf("no range support")
				continue
			}
			return fmt.Errorf("answered %s", resp.Status)
		}
		_, err = io.Copy(f, resp.Body)
		resp.Body.Close()
		f.Close()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err == nil {
			return os.Rename(part, dst)
		}
		last = err
	}
	return last
}

type fixed string

func (f fixed) Latest(context.Context) (string, error) { return string(f), nil }

func readOne(ctx context.Context, b *mbdump.Builder, c *http.Client, src, tool string) error {
	var in io.ReadCloser
	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
		if err != nil {
			return err
		}
		resp, err := c.Do(req)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return fmt.Errorf("answered %s", resp.Status)
		}
		in = resp.Body
	} else {
		f, err := os.Open(src)
		if err != nil {
			return err
		}
		in = f
	}
	defer in.Close()
	plain, err := mbdump.Decompress(ctx, tool, in)
	if err != nil {
		return err
	}
	if err := b.Read(ctx, plain); err != nil {
		plain.Close()
		return err
	}
	return plain.Close()
}

// pictures answers the music picture bytes: from the picture store opened
// read-only, else the given number.
func pictures(path string, n int64) func(context.Context) (int64, error) {
	if path == "" {
		if n < 0 {
			return nil
		}
		return func(context.Context) (int64, error) { return n, nil }
	}
	return func(ctx context.Context) (int64, error) {
		db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
		if err != nil {
			return 0, err
		}
		defer db.Close()
		k := imgstore.MusicKinds
		var v sql.NullInt64
		err = db.QueryRowContext(ctx, `SELECT SUM(bytes) FROM images WHERE kind IN (?,?,?,?)`, k[0], k[1], k[2], k[3]).Scan(&v)
		return v.Int64, err
	}
}
