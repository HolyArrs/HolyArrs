package music

import (
	"context"
	"database/sql"
	"encoding/json"
	"hash/fnv"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	musicscore "github.com/HolyArrs/holyarrs/app/backend/internal/music/score"
)

// Explore → Music on disk: layer 1 (artist data, listeners,
// similarity) and each member's last day plan are kept in lookup_cache, so
// the first visit after a restart reads them instead of the metadata server.
// A plan read from disk is served at once and rebuilt in the background when
// it is older than musicSnapFresh; the rebuilt plan replaces it.

const (
	musicDiskPrefix = "music_l1:"
	musicSnapPrefix = "music_v7_plan:"
	// musicBatchParallel is how many metadata chunks are fetched at once.
	musicBatchParallel = 6
	// musicSnapKeep is how long a plan stays on disk.
	musicSnapKeep = 48 * time.Hour
	// musicSnapFresh is the age after which a plan read from disk is rebuilt.
	musicSnapFresh = 30 * time.Minute
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
func (h *Handlers) musicDiskRead(ctx context.Context, prefix string, ids []string, now time.Time) map[string]any {
	out := map[string]any{}
	if h.Store == nil {
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
		args = append(args, database.FormatTime(now))
		rows, err := h.Store.Read().QueryContext(ctx, "SELECT key, body FROM lookup_cache WHERE key IN (?"+strings.Repeat(",?", len(part)-1)+") AND expires_at > ?", args...)
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
func (h *Handlers) musicDiskWrite(ctx context.Context, prefix string, vals map[string]any, now time.Time) {
	if h.Store == nil || len(vals) == 0 || ctx.Err() != nil {
		return
	}
	at, exp := database.FormatTime(now), database.FormatTime(now.Add(musicArtistTTL))
	_ = h.Store.Write(ctx, func(tx *sql.Tx) error {
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

// musicSnap is one member's day plan as kept on disk: the candidates and
// cards the plan is built from (NewPlan is deterministic), before state.
type musicSnap struct {
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
	hv := fnv.New64a()
	keys := make([]string, 0, len(lib.artists))
	for _, ar := range lib.artists {
		keys = append(keys, ar.mbid+":"+strconv.Itoa(ar.had)+":"+strconv.FormatBool(ar.hadAny))
	}
	sort.Strings(keys)
	for _, k := range keys {
		hv.Write([]byte(k))
		hv.Write([]byte{0})
	}
	hv.Write([]byte(strconv.Itoa(len(lib.albums)) + "|" + strconv.Itoa(len(lib.hadParts))))
	return strconv.FormatUint(hv.Sum64(), 36)
}

// saveMusicSnap keeps a complete plan's build on disk.
func (h *Handlers) saveMusicSnap(ctx context.Context, member int64, region, date string, lib *musicLib, ord *musicOrder, now time.Time) {
	if h.Store == nil || ord == nil || ord.v7 == nil {
		return
	}
	s := musicSnap{Date: date, Lib: musicLibPrint(lib), Built: now, Hero: ord.Hero, Alts: ord.Alts, Regions: ord.Regions, Cold: ord.Cold, Rows: ord.v7.rows, Metas: ord.v7.metas, Cards: ord.v7.cards, Seen: ord.v7.seen}
	b, err := json.Marshal(s)
	if err != nil {
		return
	}
	_ = metadata.LookupCache{Store: h.Store}.Put(ctx, musicSnapKey(member, region), b, now, now.Add(musicSnapKeep))
}

// musicStalePrefix marks kept plans stale (a follow, a Library or list
// change) instead of deleting them: a stale plan is still served while its
// replacement is built.
const musicStalePrefix = "music_v7_stale:"

// loadMusicSnap reads a member's last kept plan. current says it is of
// date, of this Library, not marked stale since and not older than
// musicSnapFresh; any other kept plan is still the last good one.
func (h *Handlers) loadMusicSnap(ctx context.Context, member int64, region, date string, lib *musicLib, now time.Time) (s *musicSnap, current, ok bool) {
	if h.Store == nil {
		return nil, false, false
	}
	lc := metadata.LookupCache{Store: h.Store}
	b, ok, err := lc.Get(ctx, musicSnapKey(member, region), now)
	if err != nil || !ok {
		return nil, false, false
	}
	s = &musicSnap{}
	if json.Unmarshal(b, s) != nil {
		return nil, false, false
	}
	current = s.Date == date && s.Lib == musicLibPrint(lib) && now.Sub(s.Built) < musicSnapFresh && !now.Before(s.Built)
	for _, who := range []string{"all", strconv.FormatInt(member, 10)} {
		if m, ok, err := lc.Get(ctx, musicStalePrefix+who, now); err == nil && ok {
			if at, err := time.Parse(time.RFC3339Nano, string(m)); err == nil && !s.Built.After(at) {
				current = false
			}
		}
	}
	return s, current, true
}

// order turns a kept plan back into the build the handler renders.
func (s *musicSnap) order() *musicOrder {
	c := &musicCand{rows: s.Rows, metas: s.Metas, cards: s.Cards, seen: s.Seen}
	if c.metas == nil {
		c.metas = map[string]MusicRow{}
	}
	if c.cards == nil {
		c.cards = map[string]map[string]MusicCard{}
	}
	return &musicOrder{Hero: s.Hero, Alts: s.Alts, Regions: s.Regions, Cold: s.Cold, v7: c}
}

// buildMusicPlan builds a member's plan from scratch (layer 1 on disk or
// the metadata server) and keeps it in memory and, when complete, on disk.
func (h *Handlers) buildMusicPlan(ctx context.Context, member int64, region string, key musicscore.PlanKey, lib *musicLib, acts *musicActs, now time.Time) *musicscore.Plan {
	miss := &musicMiss{}
	bctx := context.WithValue(ctx, musicMissKey{}, miss)
	ord := h.buildMusicOrder(bctx, member, region, lib, acts, now)
	if ord.v7 == nil {
		ord.v7 = &musicCand{metas: map[string]MusicRow{}, cards: map[string]map[string]MusicCard{}}
	}
	p := musicscore.NewPlan(ord.v7.rows, h.musicHeadRows(), musicPlanMax, member, key.Date, ord.v7.seen)
	p.Meta = ord
	p.Partial = miss.failed.Load()
	if ctx.Err() == nil {
		// On disk first: a visitor waiting on the memory copy then finds
		// the disk copy in place too.
		if !p.Partial {
			h.saveMusicSnap(ctx, member, region, key.Date, lib, ord, now)
		}
		h.exploreCache.plans.Put(key, p, now)
	}
	return p
}

// refreshMusicPlan rebuilds a member's plan in the background (one at a time
// per plan key); the new plan replaces the one being served. The channel
// closes when the build is done.
func (h *Handlers) refreshMusicPlan(member int64, region string, key musicscore.PlanKey, now time.Time) <-chan struct{} {
	k := musicSnapKey(member, region) + "|" + key.Date
	done := make(chan struct{})
	if v, busy := h.exploreCache.refreshing.LoadOrStore(k, done); busy {
		return v.(chan struct{})
	}
	go func() {
		defer close(done)
		defer h.exploreCache.refreshing.Delete(k)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		lib, err := h.musicLibrary(ctx, now)
		if err != nil {
			return
		}
		acts, err := h.musicActsOf(ctx, member)
		if err != nil {
			return
		}
		h.buildMusicPlan(ctx, member, region, key, lib, acts, now)
	}()
	return done
}

// forgetMusicSnaps marks a member's plans stale (-1: everyone's), in
// memory and on disk (a follow, a Library change). Nothing is deleted: the
// stale plan is served while the next visit's rebuild runs behind.
// followsChanged drops a person's music rows built from their follows.
func (h *Handlers) followsChanged(userID int64) {
	h.exploreCache.days.Invalidate(userID)
	h.forgetMusicSnaps(userID)
}

func (h *Handlers) forgetMusicSnaps(member int64) {
	h.exploreCache.plans.MarkStale(member)
	if h.Store == nil {
		return
	}
	who := "all"
	if member >= 0 {
		who = strconv.FormatInt(member, 10)
	}
	now := time.Now().UTC()
	if h.exploreCache.now != nil {
		now = h.exploreCache.now().UTC()
	}
	_ = metadata.LookupCache{Store: h.Store}.Put(context.Background(), musicStalePrefix+who, []byte(now.Format(time.RFC3339Nano)), now, now.Add(musicSnapKeep))
}
