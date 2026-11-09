package music

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Explore → Music (plan 22 October 2026, Phase 3): the v2
// rows with their scores (rows 1, 2, 4, 5, 6, 8), plus Since you have,
// Finish the set, Artists near your taste, genre shelves and Guest spots;
// diversity in every scored row and the exploration slot in personal
// rows. The server gives listeners, similarity and each artist's genre
// votes and top release groups (artist ids only, nothing about a member);
// A, G, M and the order are worked out here.

const (
	musicArtistTTL = 7 * 24 * time.Hour // similarity and listeners: weekly
	musicMissTTL   = time.Hour
	// musicEmptyTTL keeps an empty value (no similar artists, 0 listeners,
	// no genres or albums) only minutes (cache audit 23 October 2026).
	musicEmptyTTL = 10 * time.Minute
	musicSeedsTop = 10
	musicSimilarS = 0.3
)

// musicSeedLog keeps which artists seeded Since you have rows per member
// and day (deliberate data, from layer 2; not a view log).
type musicSeedLog struct {
	mu sync.Mutex
	m  map[int64]map[string][]string
}

func (l *musicSeedLog) put(member int64, date string, seeds []string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.m == nil {
		l.m = map[int64]map[string][]string{}
	}
	d := l.m[member]
	if d == nil {
		d = map[string][]string{}
		l.m[member] = d
	}
	for k := range d {
		if k < dayBefore(date) {
			delete(d, k)
		}
	}
	d[date] = seeds
}

func (l *musicSeedLog) get(member int64, date string) []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.m[member][date]
}

func dayBefore(date string) string {
	d, err := time.Parse("2006-01-02", date)
	if err != nil {
		return ""
	}
	return d.AddDate(0, 0, -1).Format("2006-01-02")
}

// musicBatch reads ids through layer 1, one entry per id: found answers a
// week, ids the server left out an hour, errors not kept. Layer 1 is kept on
// disk too (lookup_cache), so a restart reads it back instead of the
// metadata server; the missing chunks are fetched a few at a time.
func (h *Handlers) musicBatch(ctx context.Context, prefix string, ids []string, chunk int, now time.Time, fetch func(context.Context, []string) (map[string]any, error)) map[string]any {
	out := map[string]any{}
	var miss []string
	seen := map[string]bool{}
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		if v, ok := h.exploreCache.cand.Get(prefix+id, now); ok {
			if v != nil {
				out[id] = v
			}
			continue
		}
		miss = append(miss, id)
	}
	if len(miss) > 0 {
		disk := h.musicDiskRead(ctx, prefix, miss, now)
		rest := miss[:0:0]
		for _, id := range miss {
			if v, ok := disk[id]; ok {
				out[id] = v
				h.exploreCache.cand.Put(prefix+id, v, musicArtistTTL, now)
				continue
			}
			rest = append(rest, id)
		}
		miss = rest
	}
	var parts [][]string
	for len(miss) > 0 {
		part := miss[:min(chunk, len(miss))]
		miss = miss[len(part):]
		parts = append(parts, part)
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, musicBatchParallel)
	fetched := map[string]any{}
	for _, part := range parts {
		wg.Add(1)
		sem <- struct{}{}
		go func(part []string) {
			defer func() { <-sem; wg.Done() }()
			got, err := fetch(ctx, part)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				markMusicMiss(ctx)
				return
			}
			for _, id := range part {
				if v, ok := got[id]; ok && v != nil && musicEmpty(v) {
					// Empty is not final: used now, asked again soon, never on disk.
					out[id] = v
					h.exploreCache.cand.Put(prefix+id, v, musicEmptyTTL, now)
				} else if ok && v != nil {
					out[id] = v
					fetched[id] = v
					h.exploreCache.cand.Put(prefix+id, v, musicArtistTTL, now)
				} else {
					h.exploreCache.cand.Put(prefix+id, nil, musicMissTTL, now)
				}
			}
		}(part)
	}
	wg.Wait()
	h.musicDiskWrite(ctx, prefix, fetched, now)
	return out
}

// musicEmpty says a layer-1 value holds nothing yet.
func musicEmpty(v any) bool {
	switch x := v.(type) {
	case int64:
		return x == 0
	case []metadata.MusicSimilarScore:
		return len(x) == 0
	case *metadata.MusicArtistData:
		return x == nil || len(x.Genres) == 0 && len(x.Albums) == 0
	}
	return false
}

func (h *Handlers) musicListenersOf(ctx context.Context, ids []string, now time.Time) map[string]int64 {
	out := map[string]int64{}
	if h.MusicListeners == nil {
		return out
	}
	for id, v := range h.musicBatch(ctx, "lb:l:", ids, 60, now, func(ctx context.Context, ids []string) (map[string]any, error) {
		m, err := h.MusicListeners(ctx, ids)
		r := map[string]any{}
		for k, v := range m {
			r[k] = v
		}
		return r, err
	}) {
		out[id] = v.(int64)
	}
	return out
}

func (h *Handlers) musicArtistDataOf(ctx context.Context, ids []string, now time.Time) map[string]*metadata.MusicArtistData {
	out := map[string]*metadata.MusicArtistData{}
	if h.MusicArtistData == nil {
		return out
	}
	for id, v := range h.musicBatch(ctx, "lb:d:", ids, 30, now, func(ctx context.Context, ids []string) (map[string]any, error) {
		l, err := h.MusicArtistData(ctx, ids)
		r := map[string]any{}
		for i := range l {
			r[strings.ToLower(l[i].MBID)] = &l[i]
		}
		return r, err
	}) {
		out[id] = v.(*metadata.MusicArtistData)
	}
	return out
}

func (h *Handlers) musicSimilarOf(ctx context.Context, seeds []string, now time.Time) map[string][]metadata.MusicSimilarScore {
	out := map[string][]metadata.MusicSimilarScore{}
	if h.MusicSimilarSet == nil {
		return out
	}
	for id, v := range h.musicBatch(ctx, "lb:s:", seeds, 12, now, func(ctx context.Context, ids []string) (map[string]any, error) {
		m, err := h.MusicSimilarSet(ctx, ids)
		r := map[string]any{}
		for k, v := range m {
			r[k] = v
		}
		return r, err
	}) {
		out[id] = v.([]metadata.MusicSimilarScore)
	}
	return out
}

func (h *Handlers) musicGenreChartOf(ctx context.Context, region, slug string, now time.Time) []metadata.MusicChartItem {
	if h.MusicCharts == nil {
		return nil
	}
	key := "chart:" + region + ":albums:" + slug
	if v, ok := h.exploreCache.cand.Get(key, now); ok {
		return v.([]metadata.MusicChartItem)
	}
	c, err := h.MusicCharts(ctx, region, "albums", slug)
	if err != nil {
		markMusicMiss(ctx)
		return nil
	}
	h.exploreCache.cand.Put(key, c.Items, spread(musicChartTTL, 0.25), now)
	return c.Items
}

// sinkAsked moves asked and approved cards to the end of each discover
// row (layer 3, at render; the stored order is never changed).
func sinkAsked(rows []MusicRow) {
	for i := range rows {
		if rows[i].ID == "your_asks" || rows[i].ID == "genres" {
			continue
		}
		items := rows[i].Items
		sort.SliceStable(items, func(x, y int) bool {
			ax := items[x].State == "asked" || items[x].State == "approved"
			ay := items[y].State == "asked" || items[y].State == "approved"
			return !ax && ay
		})
	}
}
