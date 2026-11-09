package music

import (
	"context"
	"encoding/json"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	musicscore "github.com/HolyArrs/holyarrs/app/backend/internal/music/score"
)

// Explore → Music, flag music_rows_v2 (plan 22 October 2026, Phase 1): eight
// rows with plain sort keys, one shared dedup, and three cache layers.
// Layer 1 keeps chart and library candidates, layer 2 the member's row and
// item order for the day, layer 3 (state chips and the asks row) is read
// live on every answer, so an ask moves a chip and never the order.

const (
	musicChartTTL     = 6 * time.Hour
	musicGenreListTTL = 15 * time.Minute
	musicLibraryTTL   = 5 * time.Minute
)

// exploreState is the flag's cache layers.
type exploreState struct {
	cand musicscore.Candidates
	days musicscore.Days
	// seedLog keeps each member's Since you have seeds per day (v3).
	seedLog musicSeedLog
	// plans holds the v7 row plans; cursorKey signs their cursors.
	plans     musicscore.PlanStore
	cursorKey []byte
	cursorMu  sync.Mutex
	// clearing is set while a debounced day-order clear is pending.
	clearing atomic.Bool
	// refreshing holds the plans being rebuilt behind a plan read from disk.
	refreshing sync.Map
	// asked holds the member and region pairs asked for lately (prewarm).
	asked sync.Map
	// now is the clock (tests); nil is time.Now.
	now func() time.Time
}

// musicOrder is layer 2: one member's answer for one day, before state.
type musicOrder struct {
	Hero    *MusicCard
	Alts    []MusicCard
	Rows    []MusicRow
	Regions []string
	Cold    bool
	// v7 is the raw candidate rows (flag music_rows_v7).
	v7 *musicCand
}

// musicLib is the library snapshot the rows are built from (layer 1).
type musicLib struct {
	artists  []musicLibArtist
	byMBID   map[string]int // artist mbid → index
	albums   []musicLibAlbum
	hadParts map[int64]bool
}

type musicLibArtist struct {
	id     int64
	name   string
	mbid   string
	image  string
	hadAny bool
	had    int
}

type musicLibAlbum struct {
	partID, titleID int64
	name, mbid      string
	kind            string
	secondary       bool
	date            string
}

func (h *Handlers) musicLibrary(ctx context.Context, now time.Time) (*musicLib, error) {
	if v, ok := h.exploreCache.cand.Get("lib:snapshot", now); ok {
		return v.(*musicLib), nil
	}
	db := h.Store.Read()
	lib := &musicLib{byMBID: map[string]int{}, hadParts: map[int64]bool{}}
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT i.part_id FROM items i JOIN slots s ON s.item_id = i.id
		WHERE i.kind = 'track' AND s.file_id IS NOT NULL AND i.part_id IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			lib.hadParts[id] = true
		}
	}
	rows.Close()
	ps, err := db.QueryContext(ctx, `SELECT p.id, p.title_id, p.name, coalesce(json_extract(p.source_ids, '$.musicbrainz'), ''), lower(coalesce(p.release_type, '')),
		coalesce(p.secondary_types, '[]') NOT IN ('[]', 'null', ''), coalesce(substr(p.release_date, 1, 10), '')
		FROM parts p JOIN titles t ON t.id = p.title_id
		WHERE t.kind = 'artist' AND t.state != 'removed' AND p.kind = 'album' AND p.dropped_at IS NULL ORDER BY p.id`)
	if err != nil {
		return nil, err
	}
	had := map[int64]int{}
	for ps.Next() {
		var al musicLibAlbum
		if ps.Scan(&al.partID, &al.titleID, &al.name, &al.mbid, &al.kind, &al.secondary, &al.date) == nil {
			al.mbid = strings.ToLower(al.mbid)
			// A title that names a demo, bootleg or outtake set is not a
			// studio album, whatever MusicBrainz's types say.
			al.secondary = al.secondary || nonStudioTitle(al.name)
			lib.albums = append(lib.albums, al)
			if lib.hadParts[al.partID] {
				had[al.titleID]++
			}
		}
	}
	ps.Close()
	ts, err := db.QueryContext(ctx, `SELECT id, name, lower(coalesce(json_extract(source_ids, '$.musicbrainz'), '')), artwork FROM titles WHERE kind = 'artist' AND state != 'removed' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	for ts.Next() {
		var ar musicLibArtist
		var art string
		if ts.Scan(&ar.id, &ar.name, &ar.mbid, &art) == nil {
			ar.had, ar.hadAny = had[ar.id], had[ar.id] > 0
			if l := artworkLinksJSON(art); len(l) > 0 && strings.HasPrefix(l[0], "/api/v1/artwork/") {
				ar.image = l[0]
			}
			if ar.mbid != "" {
				lib.byMBID[ar.mbid] = len(lib.artists)
			}
			lib.artists = append(lib.artists, ar)
		}
	}
	ts.Close()
	h.exploreCache.cand.Put("lib:snapshot", lib, musicLibraryTTL, now)
	return lib, nil
}

// musicActs reads the household's follows and music asks, live.
type musicActs struct {
	follows map[int64]map[int64]bool // title id → user ids
	// asked: artist mbid → user ids on an artist ask (any albums).
	asked map[string]map[int64]bool
	// albumState: release group mbid → asked or approved (household).
	albumState map[string]string
	// artistState: artist mbid → asked or approved, for whole-artist asks.
	artistState map[string]string
	// mine: the member's open album asks, newest first.
	mine []musicAsk
}

type musicAsk struct {
	artist, artistMBID string
	album, albumMBID   string
	state, at          string
}

func (h *Handlers) musicActsOf(ctx context.Context, member int64) (*musicActs, error) {
	db := h.Store.Read()
	x := &musicActs{follows: map[int64]map[int64]bool{}, asked: map[string]map[int64]bool{}, albumState: map[string]string{}, artistState: map[string]string{}}
	rows, err := db.QueryContext(ctx, `SELECT user_id, title_id FROM follows`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var u, t int64
		if rows.Scan(&u, &t) == nil {
			if x.follows[t] == nil {
				x.follows[t] = map[int64]bool{}
			}
			x.follows[t][u] = true
		}
	}
	rows.Close()
	rows, err = db.QueryContext(ctx, `SELECT a.id, a.name, lower(coalesce(json_extract(a.source_ids, '$.musicbrainz'), '')), a.albums, a.state, a.asked_at,
		coalesce((SELECT group_concat(user_id) FROM ask_people p WHERE p.ask_id = a.id), ''), a.asked_by
		FROM asks a WHERE a.kind = 'artist' AND a.state IN ('asked', 'approved') ORDER BY a.asked_at DESC, a.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, by int64
		var name, mbid, albums, state, at, people string
		if rows.Scan(&id, &name, &mbid, &albums, &state, &at, &people, &by) != nil {
			continue
		}
		users := map[int64]bool{by: true}
		for _, p := range strings.Split(people, ",") {
			if u, err := strconv.ParseInt(p, 10, 64); err == nil {
				users[u] = true
			}
		}
		if mbid != "" {
			if x.asked[mbid] == nil {
				x.asked[mbid] = map[int64]bool{}
			}
			for u := range users {
				x.asked[mbid][u] = true
			}
		}
		var list []struct {
			MBID string `json:"mbid"`
			Name string `json:"name"`
		}
		_ = json.Unmarshal([]byte(albums), &list)
		if len(list) == 0 && mbid != "" {
			x.artistState[mbid] = state
		}
		for _, al := range list {
			x.albumState[strings.ToLower(al.MBID)] = state
		}
		if users[member] {
			if len(list) == 0 {
				x.mine = append(x.mine, musicAsk{artist: name, artistMBID: mbid, state: state, at: at})
			}
			for _, al := range list {
				x.mine = append(x.mine, musicAsk{artist: name, artistMBID: mbid, album: al.Name, albumMBID: strings.ToLower(al.MBID), state: state, at: at})
			}
		}
	}
	return x, rows.Err()
}

func artworkLinksJSON(raw string) []string {
	var v any
	if json.Unmarshal([]byte(raw), &v) != nil {
		return nil
	}
	var out []string
	var walk func(any)
	walk = func(x any) {
		switch t := x.(type) {
		case string:
			out = append(out, t)
		case []any:
			for _, e := range t {
				walk(e)
			}
		case map[string]any:
			keys := make([]string, 0, len(t))
			for k := range t {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				walk(t[k])
			}
		}
	}
	walk(v)
	return out
}

func (h *Handlers) musicChartOf(ctx context.Context, region, kind string, now time.Time) []metadata.MusicChartItem {
	if h.MusicCharts == nil {
		return nil
	}
	key := "chart:" + region + ":" + kind
	if v, ok := h.exploreCache.cand.Get(key, now); ok {
		return v.([]metadata.MusicChartItem)
	}
	c, err := h.MusicCharts(ctx, region, kind, "")
	if err != nil {
		markMusicMiss(ctx)
		return nil
	}
	h.exploreCache.cand.Put(key, c.Items, spread(musicChartTTL, 0.25), now)
	return c.Items
}

func (h *Handlers) musicGenreTiles(ctx context.Context, region string, now time.Time) []metadata.MusicGenre {
	if h.MusicGenres == nil {
		return nil
	}
	key := "genres:" + region
	if v, ok := h.exploreCache.cand.Get(key, now); ok {
		return v.([]metadata.MusicGenre)
	}
	g, err := h.MusicGenres(ctx, region)
	if err != nil {
		markMusicMiss(ctx)
		return nil
	}
	// Short-lived, so new tile art from the metadata server shows soon.
	h.exploreCache.cand.Put(key, g.Genres, musicGenreListTTL, now)
	return g.Genres
}

// nonStudioWords are title words that mark a release as demos, bootlegs,
// outtakes or other non-studio sets.
var nonStudioWords = regexp.MustCompile(`(?i)\b(demos?|bootlegs?|outtakes?|unreleased|rehearsals?|b-sides|leaked|snippets?)\b`)

// nonStudioTitle reports whether an album title is clearly not a studio
// album (MusicBrainz sometimes leaves such release groups untyped).
func nonStudioTitle(name string) bool { return nonStudioWords.MatchString(name) }

// musicMiss records that a build lost a source; layer 2 does not keep it.
type musicMiss struct{ failed atomic.Bool }

type musicMissKey struct{}

func markMusicMiss(ctx context.Context) {
	if m, ok := ctx.Value(musicMissKey{}).(*musicMiss); ok {
		m.failed.Store(true)
	}
}

func dayAt(now time.Time, d int) string { return now.UTC().AddDate(0, 0, d).Format("2006-01-02") }

func dayOfText(s string) string {
	if len(s) >= 10 {
		return s[:10]
	}
	return ""
}
