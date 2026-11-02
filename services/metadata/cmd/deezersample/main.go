// Command deezersample runs the Deezer ID matcher on a random sample of
// albums from the slim MusicBrainz copy and writes one JSON line per album
// (the MusicBrainz side, the match and the matched Deezer album) for the
// AC 7 match rate and hand check. It only reads the copy and asks Deezer's
// public API; matches are kept in memory, never in a store.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/imgstore"
	"github.com/CoderLuii/holyarrs-metadata/internal/mbread"
	"github.com/CoderLuii/holyarrs-metadata/internal/srctag"
	"github.com/CoderLuii/holyarrs-metadata/internal/upstream"
	_ "modernc.org/sqlite"
)

// streamingLink is an artist URL that shows the artist is in streaming
// catalogs (the "requestable" sample).
const streamingLink = `(u.url LIKE '%deezer.com/%' OR u.url LIKE '%open.spotify.com/%' OR u.url LIKE '%music.apple.com/%' OR u.url LIKE '%tidal.com/%')`

func main() {
	root := flag.String("slim", "/music", "slim copy root")
	n := flag.Int("n", 500, "albums")
	mode := flag.String("mode", "streaming", "streaming: albums by artists with a streaming link; notable: also a Wikidata link; random: any album")
	seed := flag.Uint64("seed", 20261028, "random seed")
	out := flag.String("out", "-", "output JSONL")
	pool := flag.String("egress", "", "egress paths for Deezer (HOLYARRS_MB_EGRESS format); empty: one address")
	idsFile := flag.String("ids", "", "file of release-group MBIDs, one a line (skips the random sample)")
	flag.Parse()
	ctx := context.Background()

	cur, err := os.ReadFile(filepath.Join(*root, "CURRENT"))
	if err != nil {
		log.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+filepath.Join(*root, strings.TrimSpace(string(cur)), "mb.sqlite")+"?mode=ro")
	if err != nil {
		log.Fatal(err)
	}
	var ids []string
	if *idsFile != "" {
		b, err := os.ReadFile(*idsFile)
		if err != nil {
			log.Fatal(err)
		}
		ids = strings.Fields(string(b))
		log.Printf("read %d albums from %s", len(ids), *idsFile)
	} else {
		ids = sample(ctx, db, *n, *mode, *seed)
		log.Printf("sampled %d albums (%s)", len(ids), *mode)
	}

	rd, err := mbread.Open(*root)
	if err != nil {
		log.Fatal(err)
	}
	var poolHTTP *http.Client
	if *pool != "" {
		e, err := upstream.ParseEgress(*pool)
		if err != nil {
			log.Fatal(err)
		}
		poolHTTP = e.Client(20 * time.Second)
		log.Printf("Deezer over %d egress paths", e.N())
	}
	var mu sync.Mutex
	kept := map[string][2]string{}
	lim := imgstore.NewLimits(nil)
	lim.SetInterval("deezer", 150*time.Millisecond)
	r := &imgstore.Resolver{HTTP: &http.Client{Timeout: 20 * time.Second}, UserAgent: "HolyArrs-metadata/sample", Limits: lim, MB: rd.Stored, MBTitles: rd.GroupTitles, PoolHTTP: poolHTTP,
		Keep: func(kind, mbid string, t srctag.Tag, store, ext string) {
			mu.Lock()
			kept[kind+mbid] = [2]string{store, ext}
			mu.Unlock()
		},
		Kept: func(kind, mbid, source string) (string, string, time.Time, bool) {
			mu.Lock()
			defer mu.Unlock()
			k, ok := kept[kind+mbid]
			return k[0], k[1], time.Now(), ok
		}}

	w := os.Stdout
	if *out != "-" {
		if w, err = os.Create(*out); err != nil {
			log.Fatal(err)
		}
		defer w.Close()
	}
	enc := json.NewEncoder(w)
	matched := 0
	for i, gid := range ids {
		row := map[string]any{"mbid": gid}
		var g struct {
			Data struct {
				Title string `json:"title"`
				Date  string `json:"first-release-date"`
				AC    []struct {
					Artist struct{ ID, Name string } `json:"artist"`
				} `json:"artist-credit"`
				Media []struct {
					Tracks []any `json:"tracks"`
				} `json:"media"`
			} `json:"source_data"`
		}
		if b, _, ok := rd.Stored(ctx, "release-group", gid); ok {
			_ = json.Unmarshal(b, &g)
		}
		row["mb_title"], row["mb_date"] = g.Data.Title, g.Data.Date
		if len(g.Data.AC) > 0 {
			row["mb_artist"], row["mb_artist_id"] = g.Data.AC[0].Artist.Name, g.Data.AC[0].Artist.ID
		}
		nt := 0
		for _, m := range g.Data.Media {
			nt += len(m.Tracks)
		}
		row["mb_tracks"] = nt
		t0 := time.Now()
		id, how, err := r.DeezerMatch(ctx, imgstore.KindAlbum, gid)
		// A refused call (403, a pause) is not an answer: wait and ask
		// again with fresh limits, as the server's queue would.
		for try := 0; err != nil && try < 3; try++ {
			row["retries"] = try + 1
			time.Sleep(20 * time.Second)
			r.Limits = imgstore.NewLimits(nil)
			r.Limits.SetInterval("deezer", 150*time.Millisecond)
			id, how, err = r.DeezerMatch(ctx, imgstore.KindAlbum, gid)
		}
		row["ms"] = time.Since(t0).Milliseconds()
		if err != nil {
			row["error"] = err.Error()
		}
		row["dz_id"], row["how"] = id, how
		if aid := g.Data.AC; len(aid) > 0 {
			mu.Lock()
			row["dz_artist_match"] = kept[imgstore.KindArtist+aid[0].Artist.ID]
			mu.Unlock()
		}
		if id != "" {
			matched++
			var a struct {
				Title  string `json:"title"`
				Date   string `json:"release_date"`
				Tracks int    `json:"nb_tracks"`
				Artist struct {
					ID   int64  `json:"id"`
					Name string `json:"name"`
				} `json:"artist"`
			}
			if ok, err := r.DeezerJSON(ctx, "/album/"+id, nil, &a); ok && err == nil {
				row["dz_title"], row["dz_artist"], row["dz_date"], row["dz_tracks"] = a.Title, a.Artist.Name, a.Date, a.Tracks
				row["dz_artist_id"] = strconv.FormatInt(a.Artist.ID, 10)
			}
		}
		_ = enc.Encode(row)
		if (i+1)%50 == 0 {
			log.Printf("%d/%d done, %d matched", i+1, len(ids), matched)
		}
	}
	fmt.Fprintf(os.Stderr, "matched %d of %d (%.1f%%)\n", matched, len(ids), 100*float64(matched)/float64(max(len(ids), 1)))
}

// sample picks n random official albums (primary type Album, no secondary
// type, one credited artist) by probing random ids.
func sample(ctx context.Context, db *sql.DB, n int, mode string, seed uint64) []string {
	var top int64
	if err := db.QueryRowContext(ctx, `SELECT MAX(id) FROM release_group`).Scan(&top); err != nil {
		log.Fatal(err)
	}
	q := `SELECT rg.gid FROM release_group rg
JOIN release_group_primary_type pt ON pt.id = rg.type AND pt.name = 'Album'
JOIN artist_credit_name acn ON acn.artist_credit = rg.artist_credit
WHERE rg.id = ?
 AND NOT EXISTS (SELECT 1 FROM release_group_secondary_type_join s WHERE s.release_group = rg.id)
 AND (SELECT COUNT(*) FROM artist_credit_name x WHERE x.artist_credit = rg.artist_credit) = 1
 AND EXISTS (SELECT 1 FROM release r WHERE r.release_group = rg.id)`
	if mode == "notable" {
		q += ` AND EXISTS (SELECT 1 FROM l_artist_url l JOIN url u ON u.id = l.entity1 WHERE l.entity0 = acn.artist AND u.url LIKE '%wikidata.org/%')`
	}
	if mode == "streaming" || mode == "notable" {
		q += ` AND EXISTS (SELECT 1 FROM l_artist_url l JOIN url u ON u.id = l.entity1 WHERE l.entity0 = acn.artist AND ` + streamingLink + `)`
	}
	rng := rand.New(rand.NewPCG(seed, 1))
	seen := map[string]bool{}
	var out []string
	for tries := 0; len(out) < n && tries < n*2000; tries++ {
		var gid string
		if db.QueryRowContext(ctx, q, rng.Int64N(top)+1).Scan(&gid) == nil && !seen[gid] {
			seen[gid] = true
			out = append(out, gid)
		}
	}
	return out
}
