package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"hash/fnv"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs/internal/metadata"
	musicscore "github.com/CoderLuii/holyarrs/internal/music/score"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// Explore → Music on disk (music_rows_v7): layer 1 (artist data, listeners,
// similarity) and each member's last day plan are kept in lookup_cache, so
// the first visit after a restart reads them instead of the metadata server.
// A plan read from disk is served at once and rebuilt in the background when
// it is older than musicV7SnapFresh; the rebuilt plan replaces it.

const (
	musicDiskPrefix = "music_l1:"
	musicSnapPrefix = "music_v7_plan:"
	// musicBatchParallel is how many metadata chunks are fetched at once.
	musicBatchParallel = 4
	// musicV7SnapKeep is how long a plan stays on disk.
	musicV7SnapKeep = 48 * time.Hour
	// musicV7SnapFresh is the age after which a plan read from disk is rebuilt.
	musicV7SnapFresh = 30 * time.Minute
)

// musicDiskDecode turns a kept layer-1 body back into its value.
func musicDiskDecode(prefix string, b []byte) (any, bool) {
	switch prefix {
	case "lb:d:":
		var v metadata.MusicArtistData
		if json.Unmarshal(b, &v) != nil {
			return nil, false
		}
		return &v, true
	case "lb:l:":
		var v int64
		if json.Unmarshal(b, &v) != nil {
			return nil, false
		}
		return v, true
	case "lb:s:":
		var v []metadata.MusicSimilarScore
		if json.Unmarshal(b, &v) != nil {
			return nil, false
		}
		return v, true
	}
	return nil, false
}

// musicDiskRead answers the kept layer-1 values of ids.
func (a *API) musicDiskRead(ctx context.Context, prefix string, ids []string, now time.Time) map[string]any {
	out := map[string]any{}
	if a.s.Store == nil {
		return out
	}
	switch prefix {
	case "lb:d:", "lb:l:", "lb:s:":
	default:
		return out
	}
	for len(ids) > 0 {
		part := ids[:min(400, len(ids))]
		ids = ids[len(part):]
		args := make([]any, 0, len(part)+1)
		for _, id := range part {
			args = append(args, musicDiskPrefix+prefix+id)
		}
		args = append(args, store.FormatTime(now))
		rows, err := a.s.Store.Read().QueryContext(ctx, "SELECT key, body FROM lookup_cache WHERE key IN (?"+strings.Repeat(",?", len(part)-1)+") AND expires_at > ?", args...)
		if err != nil {
			return out
		}
		for rows.Next() {
			var k string
			var b []byte
			if rows.Scan(&k, &b) != nil {
				continue
			}
			if v, ok := musicDiskDecode(prefix, b); ok {
				out[strings.TrimPrefix(k, musicDiskPrefix+prefix)] = v
			}
		}
		rows.Close()
	}
	return out
}

// musicDiskWrite keeps fetched layer-1 values for musicArtistTTL.
func (a *API) musicDiskWrite(ctx context.Context, prefix string, vals map[string]any, now time.Time) {
	if a.s.Store == nil || len(vals) == 0 || ctx.Err() != nil {
		return
	}
	at, exp := store.FormatTime(now), store.FormatTime(now.Add(musicArtistTTL))
	_ = a.s.Store.Write(ctx, func(tx *sql.Tx) error {
		for id, v := range vals {
			b, err := json.Marshal(v)
			if err != nil {
				continue
			}
			if _, err := tx.Exec(`INSERT INTO lookup_cache (key, body, fetched_at, expires_at) VALUES (?, ?, ?, ?)
				ON CONFLICT (key) DO UPDATE SET body = excluded.body, fetched_at = excluded.fetched_at, expires_at = excluded.expires_at`,
				musicDiskPrefix+prefix+id, b, at, exp); err != nil {
				return err
			}
		}
		return nil
	})
}

// musicV7Snap is one member's day plan as kept on disk: the candidates and
// cards the plan is built from (NewPlan is deterministic), before state.
type musicV7Snap struct {
	Date    string                          `json:"date"`
	Lib     string                          `json:"lib"`
	Built   time.Time                       `json:"built"`
	Hero    *MusicCard                      `json:"hero,omitempty"`
	Alts    []MusicCard                     `json:"alts"`
	Regions []string                        `json:"regions"`
	Cold    bool                            `json:"cold"`
	Rows    []musicscore.Row                `json:"rows"`
	Metas   map[string]MusicRow             `json:"metas"`
	Cards   map[string]map[string]MusicCard `json:"cards"`
	Seen    map[string]bool                 `json:"seen"`
}

func musicSnapKey(member int64, region string) string {
	return musicSnapPrefix + strconv.FormatInt(member, 10) + "|" + region
}

// musicLibPrint is a short fingerprint of the Library the plan was built
// from: a kept plan of another Library is not served.
func musicLibPrint(lib *musicLib) string {
	h := fnv.New64a()
	keys := make([]string, 0, len(lib.artists))
	for _, ar := range lib.artists {
		keys = append(keys, ar.mbid+":"+strconv.Itoa(ar.had)+":"+strconv.FormatBool(ar.hadAny))
	}
	sort.Strings(keys)
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte{0})
	}
	h.Write([]byte(strconv.Itoa(len(lib.albums)) + "|" + strconv.Itoa(len(lib.hadParts))))
	return strconv.FormatUint(h.Sum64(), 36)
}

// saveMusicV7Snap keeps a complete plan's build on disk.
func (a *API) saveMusicV7Snap(ctx context.Context, member int64, region, date string, lib *musicLib, ord *musicV2Order, now time.Time) {
	if a.s.Store == nil || ord == nil || ord.v7 == nil {
		return
	}
	s := musicV7Snap{Date: date, Lib: musicLibPrint(lib), Built: now, Hero: ord.Hero, Alts: ord.Alts, Regions: ord.Regions, Cold: ord.Cold, Rows: ord.v7.rows, Metas: ord.v7.metas, Cards: ord.v7.cards, Seen: ord.v7.seen}
	b, err := json.Marshal(s)
	if err != nil {
		return
	}
	_ = store.LookupCache{Store: a.s.Store}.Put(ctx, musicSnapKey(member, region), b, now, now.Add(musicV7SnapKeep))
}

// loadMusicV7Snap reads a member's kept plan of date for this Library.
func (a *API) loadMusicV7Snap(ctx context.Context, member int64, region, date string, lib *musicLib, now time.Time) (*musicV7Snap, bool) {
	if a.s.Store == nil {
		return nil, false
	}
	b, ok, err := store.LookupCache{Store: a.s.Store}.Get(ctx, musicSnapKey(member, region), now)
	if err != nil || !ok {
		return nil, false
	}
	var s musicV7Snap
	if json.Unmarshal(b, &s) != nil || s.Date != date || s.Lib != musicLibPrint(lib) {
		return nil, false
	}
	return &s, true
}

// order turns a kept plan back into the build the handler renders.
func (s *musicV7Snap) order() *musicV2Order {
	c := &musicV7Cand{rows: s.Rows, metas: s.Metas, cards: s.Cards, seen: s.Seen}
	if c.metas == nil {
		c.metas = map[string]MusicRow{}
	}
	if c.cards == nil {
		c.cards = map[string]map[string]MusicCard{}
	}
	return &musicV2Order{Hero: s.Hero, Alts: s.Alts, Regions: s.Regions, Cold: s.Cold, v7: c}
}

// buildMusicV7Plan builds a member's plan from scratch (layer 1 on disk or
// the metadata server) and keeps it in memory and, when complete, on disk.
func (a *API) buildMusicV7Plan(ctx context.Context, member int64, region string, key musicscore.PlanKey, lib *musicLib, acts *musicActs, now time.Time) *musicscore.Plan {
	miss := &musicMiss{}
	bctx := context.WithValue(ctx, musicMissKey{}, miss)
	ord := a.buildMusicV3(bctx, member, region, lib, acts, now)
	if ord.v7 == nil {
		ord.v7 = &musicV7Cand{metas: map[string]MusicRow{}, cards: map[string]map[string]MusicCard{}}
	}
	p := musicscore.NewPlan(ord.v7.rows, a.musicV7HeadRows(), musicV7Max, member, key.Date, ord.v7.seen)
	p.Meta = ord
	p.Partial = miss.failed.Load()
	if ctx.Err() == nil {
		a.musicv2.plans.Put(key, p, now)
		if !p.Partial {
			a.saveMusicV7Snap(ctx, member, region, key.Date, lib, ord, now)
		}
	}
	return p
}

// refreshMusicV7 rebuilds a member's plan in the background (one at a time
// per plan key); the new plan replaces the one read from disk.
func (a *API) refreshMusicV7(member int64, region string, key musicscore.PlanKey, now time.Time) {
	k := musicSnapKey(member, region) + "|" + key.Date
	if _, busy := a.musicv2.refreshing.LoadOrStore(k, true); busy {
		return
	}
	go func() {
		defer a.musicv2.refreshing.Delete(k)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		lib, err := a.musicLibrary(ctx, now)
		if err != nil {
			return
		}
		acts, err := a.musicActsOf(ctx, member)
		if err != nil {
			return
		}
		a.buildMusicV7Plan(ctx, member, region, key, lib, acts, now)
	}()
}

// forgetMusicV7Snaps drops a member's kept plans (-1: everyone's), with
// the in-memory ones marked stale (a follow, a Library change).
func (a *API) forgetMusicV7Snaps(member int64) {
	a.musicv2.plans.MarkStale(member)
	if a.s.Store == nil {
		return
	}
	like := musicSnapPrefix + "%"
	if member >= 0 {
		like = musicSnapPrefix + strconv.FormatInt(member, 10) + "|%"
	}
	_ = a.s.Store.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("DELETE FROM lookup_cache WHERE key LIKE ?", like)
		return err
	})
}
