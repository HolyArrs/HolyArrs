// Command mbreadbench times lookups on a slim MusicBrainz copy, read-only,
// in its own process: it samples random items of each kind from the copy
// in use and answers each the way the server does (internal/mbread), then
// prints p50/p95/p99 and the slowest per kind.
//
//	mbreadbench -root /data/music -n 500 -workers 4
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/mbdump"
	"github.com/CoderLuii/holyarrs-metadata/internal/mbread"
	_ "modernc.org/sqlite"
)

func main() {
	root := flag.String("root", "", "slim copy root (holds CURRENT)")
	n := flag.Int("n", 500, "items sampled per kind")
	workers := flag.Int("workers", 4, "parallel readers")
	show := flag.String("show", "", "print one answer (kind:mbid) and stop")
	flag.Parse()
	if *root == "" {
		log.Fatal("-root is required")
	}
	dir, err := mbdump.Current(*root)
	if err != nil || dir == "" {
		log.Fatalf("no copy in use under %s: %v", *root, err)
	}
	sample, err := sql.Open("sqlite", "file:"+filepath.Join(dir, mbdump.DBName)+"?mode=ro&immutable=1")
	if err != nil {
		log.Fatal(err)
	}
	r, err := mbread.Open(*root)
	if err != nil {
		log.Fatal(err)
	}
	defer r.Close()
	ctx := context.Background()
	if kind, id, ok := strings.Cut(*show, ":"); ok {
		b, _, found := r.Stored(ctx, kind, id)
		fmt.Printf("found=%v %s\n", found, b)
		return
	}
	fmt.Printf("copy %s, %d per kind, %d workers\n", r.Date(), *n, *workers)
	kinds := []struct {
		name, table string
		do          func(string) bool
	}{
		{"artist", "artist", func(id string) bool { _, _, ok := r.Stored(ctx, "artist", id); return ok }},
		{"release-group", "release_group", func(id string) bool { _, _, ok := r.Stored(ctx, "release-group", id); return ok }},
		{"release", "release", func(id string) bool { _, _, ok := r.Stored(ctx, "release", id); return ok }},
		{"recording", "recording", func(id string) bool { _, _, ok := r.Stored(ctx, "recording", id); return ok }},
		{"discog-artist", "artist", func(id string) bool { _, ok := r.Artist(ctx, id); return ok }},
		{"discog-album", "release_group", func(id string) bool { _, ok := r.Album(ctx, id); return ok }},
	}
	var all []time.Duration
	for _, k := range kinds {
		ids := pick(sample, k.table, *n)
		ds := run(ids, *workers, k.do)
		all = append(all, ds...)
		report(k.name, ds)
	}
	report("all", all)
	sample.Close()
	os.Exit(0)
}

// pick samples n gids spread over the table's id range.
func pick(db *sql.DB, table string, n int) []string {
	var max int64
	if err := db.QueryRow(`SELECT MAX(id) FROM "` + table + `"`).Scan(&max); err != nil || max == 0 {
		log.Fatalf("%s: %v", table, err)
	}
	var out []string
	for len(out) < n {
		var gid string
		if db.QueryRow(`SELECT gid FROM "`+table+`" WHERE id >= abs(random()) % ? ORDER BY id LIMIT 1`, max).Scan(&gid) == nil {
			out = append(out, gid)
		}
	}
	return out
}

func run(ids []string, workers int, do func(string) bool) []time.Duration {
	out := make([]time.Duration, len(ids))
	var wg sync.WaitGroup
	next := make(chan int)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				t := time.Now()
				if !do(ids[i]) {
					log.Printf("miss %s", ids[i])
				}
				out[i] = time.Since(t)
			}
		}()
	}
	for i := range ids {
		next <- i
	}
	close(next)
	wg.Wait()
	return out
}

func report(name string, ds []time.Duration) {
	s := append([]time.Duration(nil), ds...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	at := func(p float64) time.Duration { return s[min(len(s)-1, int(p*float64(len(s))))] }
	fmt.Printf("%-14s n=%-5d p50=%-10s p95=%-10s p99=%-10s max=%s\n", name, len(s), at(.50).Round(10*time.Microsecond),
		at(.95).Round(10*time.Microsecond), at(.99).Round(10*time.Microsecond), s[len(s)-1].Round(10*time.Microsecond))
}
