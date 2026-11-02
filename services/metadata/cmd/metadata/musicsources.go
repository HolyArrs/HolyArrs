package main

import (
	"context"
	"database/sql"
	"log"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/api"
	"github.com/CoderLuii/holyarrs-metadata/internal/imgstore"
	"github.com/CoderLuii/holyarrs-metadata/internal/srctag"
)

// musicSourcesEvery is how long the per-source music counts are reused
// before the next count (one count scans the picture table once).
const musicSourcesEvery = 2 * time.Minute

// sourceRow is one source's line: stored values per kind.
type sourceRow struct {
	Source  string `json:"source"`
	Artists int64  `json:"artists"`
	Albums  int64  `json:"albums"`
	Songs   int64  `json:"songs"`
	Covers  int64  `json:"covers"`
	Photos  int64  `json:"photos"`
	// Wide is the wide background pictures, Logos the artist logos.
	Wide  int64 `json:"wide"`
	Logos int64 `json:"logos"`
	// Charts counts chart, new-release and list rows (each holds many entries).
	Charts int64 `json:"charts"`
}

// storeTable is a store table whose rows carry src and fetched_at
// (srctag.Stamp); kind is the SQL that buckets a row as artists, albums,
// songs or charts.
type storeTable struct{ file, table, body, kind string }

var storeTables = []storeTable{
	{"discog.sqlite", "artists", "body", "'artists'"},
	{"discog.sqlite", "albums", "body", "'albums'"},
	{"musicdata.sqlite", "answers", "body", "CASE WHEN k LIKE 'top:%' THEN 'songs' WHEN k LIKE 'dz-artist:%' THEN 'artists' ELSE 'albums' END"},
	{"music.sqlite", "chart_days", "body", "'charts'"},
	{"music.sqlite", "lb_charts", "body", "'charts'"},
	{"music.sqlite", "lists", "body", "'charts'"},
	{"newmusic.sqlite", "windows", "body", "'charts'"},
	{"lists.sqlite", "lists_day", "body", "'charts'"},
	{"similar.sqlite", "similar", "body", "'artists'"},
	{"similar.sqlite", "artist_data", "body", "'artists'"},
}

// storeCounts counts the stored values of the store tables per kind and
// source, opening each store file read-only. A store or table not there
// yet counts nothing.
func storeCounts(ctx context.Context, dir string) (rows []imgstore.SourceCount) {
	dbs := map[string]*sql.DB{}
	defer func() {
		for _, d := range dbs {
			d.Close()
		}
	}()
	for _, t := range storeTables {
		db, ok := dbs[t.file]
		if !ok {
			db, _ = sql.Open("sqlite", "file:"+filepath.Join(dir, t.file)+"?mode=ro&_pragma=busy_timeout(5000)")
			dbs[t.file] = db
		}
		if db == nil {
			continue
		}
		rs, err := db.QueryContext(ctx, `SELECT `+t.kind+`, src, COUNT(*), SUM(CASE WHEN src IN ('', '`+srctag.Untagged+`') OR fetched_at <= 0 THEN 1 ELSE 0 END) FROM `+t.table+` WHERE length(`+t.body+`) > 0 GROUP BY 1, 2`)
		if err != nil {
			continue
		}
		for rs.Next() {
			var c imgstore.SourceCount
			if rs.Scan(&c.Kind, &c.Source, &c.N, &c.Untagged) == nil {
				rows = append(rows, c)
			}
		}
		rs.Close()
	}
	return rows
}

type musicSourceCensus struct {
	imgs *imgstore.Store
	dir  string
	dump func(context.Context) (string, map[string]int64)
	now  func() time.Time

	mu   sync.Mutex
	at   time.Time
	last map[string]any
}

// openMusicSources adds the "music_sources" block to the status answer
// HolyDash reads: stored music values per source, and the share that has
// a source and a fetch date. Counts are cached for musicSourcesEvery.
func openMusicSources(sp *speed, dir string, dump func(context.Context) (string, map[string]int64), o *api.Options) {
	if sp == nil || sp.imgs == nil {
		return
	}
	mc := &musicSourceCensus{imgs: sp.imgs, dir: dir, dump: dump, now: time.Now}
	prev := o.StatusExtra
	o.StatusExtra = func(ctx context.Context, window time.Duration) map[string]any {
		m := map[string]any{}
		if prev != nil {
			m = prev(ctx, window)
		}
		m["music_sources"] = mc.get(ctx)
		return m
	}
}

func (mc *musicSourceCensus) get(ctx context.Context) map[string]any {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	if mc.last != nil && mc.now().Sub(mc.at) < musicSourcesEvery {
		return mc.last
	}
	started := mc.now()
	if res, err := mc.compute(ctx); err == nil {
		mc.last, mc.at = res, mc.now()
		log.Printf("music sources: counted %v values in %v", res["total"], mc.now().Sub(started).Round(time.Millisecond))
	} else {
		log.Printf("music sources: count failed: %v", err)
		if mc.last != nil {
			return mc.last
		}
		return map[string]any{"error": "count_unavailable"}
	}
	return mc.last
}

func (mc *musicSourceCensus) compute(ctx context.Context) (map[string]any, error) {
	by := map[string]*sourceRow{}
	row := func(s string) *sourceRow {
		if by[s] == nil {
			by[s] = &sourceRow{Source: s}
		}
		return by[s]
	}
	var total, untagged int64
	date := ""
	if mc.dump != nil {
		var tables map[string]int64
		date, tables = mc.dump(ctx)
		if date != "" {
			r := row("mb-dump")
			r.Artists, r.Albums, r.Songs = tables["artist"], tables["release_group"], tables["recording"]
			total += r.Artists + r.Albums + r.Songs
		}
	}
	counts, err := mc.imgs.MusicSourceCounts(ctx)
	if err != nil {
		return nil, err
	}
	for _, c := range counts {
		addPictureCount(row(c.Source), c)
		total += c.N
		untagged += c.Untagged
	}
	if mc.dir != "" {
		for _, c := range storeCounts(ctx, mc.dir) {
			r := row(c.Source)
			switch c.Kind {
			case "artists":
				r.Artists += c.N
			case "albums":
				r.Albums += c.N
			case "songs":
				r.Songs += c.N
			default:
				r.Charts += c.N
			}
			total += c.N
			untagged += c.Untagged
		}
	}
	rows := make([]sourceRow, 0, len(by))
	for _, r := range by {
		rows = append(rows, *r)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Source < rows[j].Source })
	pct := 100.0
	if total > 0 {
		pct = float64(total-untagged) / float64(total) * 100
	}
	return map[string]any{
		"as_of": mc.now().UTC().Format(time.RFC3339), "dump_date": date, "sources": rows,
		"total": total, "untagged": untagged, "with_source_and_fetched_pct": pct,
	}, nil
}

// addPictureCount adds one line of the picture census to its source's row:
// photos, wide pictures, logos and covers each in their own column; the
// live MusicBrainz tags as artists, albums or songs.
func addPictureCount(r *sourceRow, c imgstore.SourceCount) {
	switch c.Kind {
	case "artist":
		if c.Source == "mb-live" {
			r.Artists += c.N
		} else {
			r.Photos += c.N
		}
	case "wide":
		r.Wide += c.N
	case "logo":
		r.Logos += c.N
	case "album", "release-group", "release":
		if c.Source == "mb-live" {
			r.Albums += c.N
		} else {
			r.Covers += c.N
		}
	case "recording":
		r.Songs += c.N
	}
}
