package music

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// New & Popular → Music: new albums, EPs and singles to ask for (or add).
// MusicBrainz release dates say what is new, the trending lists what is
// big; the lists are built in the background and kept 12 hours, so a
// visit never waits on MusicBrainz.

const (
	// Per-kind windows, in days: this week, recently out, singles, coming.
	musicNewWeek    = 7
	musicNewRecent  = 45
	musicNewLibrary = 90
	musicNewAhead   = 60
	// musicNewRowMin is the fewest cards a row shows; musicNewRowMax the most.
	musicNewRowMin = 6
	musicNewRowMax = 20
	musicNewTTL    = 12 * time.Hour
	// musicNewArtists caps the artists one MusicBrainz search names.
	musicNewArtists = 40
)

// musicNewData is one build of the lists every member shares.
type musicNewData struct {
	at       time.Time
	releases []metadata.NewRelease // albums, EPs and singles, out or coming
	fresh    metadata.MusicTrend   // notable new albums, with listens
	albums   metadata.MusicTrend   // this week's album chart
	artists  metadata.MusicTrend   // this week's artist chart
	// server is the metadata server's new music windows (ListenBrainz fresh
	// releases and Deezer's editorial and charts), by window.
	server map[string][]metadata.MusicNewItem
}

type musicNewStore struct {
	mu       sync.Mutex
	data     *musicNewData
	building bool
	landed   time.Time
}

var musicNew musicNewStore

type releaseSource interface {
	ReleasesBetween(ctx context.Context, from, to string, types, artists []string, limit int) ([]metadata.NewRelease, error)
}

// warmMusicNew starts a build when there is none or it is older than the
// TTL; it never waits.
func (h *Handlers) warmMusicNew() *musicNewData {
	if p, _ := parts.ForTitleKind("artist"); !h.PartIsOn(p) {
		return nil
	}
	h.loadMusicNew()
	musicNew.mu.Lock()
	d := musicNew.data
	stale := (d == nil || time.Since(d.at) > musicNewTTL) && !musicNew.building
	musicNew.mu.Unlock()
	if stale {
		go h.refreshMusicNew(context.Background())
	}
	return d
}

// buildMusicNew reads the trending lists, then asks MusicBrainz (paced by
// its shared limiter) for releases by notable artists in each window.
func (h *Handlers) buildMusicNew(ctx context.Context, now time.Time) *musicNewData {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	d := &musicNewData{at: now}
	if h.MusicTrending != nil {
		d.fresh, _ = h.cachedMusicTrend(ctx, "fresh", "global")
		d.albums, _ = h.cachedMusicTrend(ctx, "album", "global")
		d.artists, _ = h.cachedMusicTrend(ctx, "artist", "global")
	}
	if h.MusicNew != nil {
		d.server = map[string][]metadata.MusicNewItem{}
		for _, w := range []string{"tracks", "week", "recent", "upcoming"} {
			if l, err := h.MusicNew(ctx, w); err == nil {
				d.server[w] = l.Items
			}
		}
	}
	var src releaseSource
	if h.lib() != nil && h.lib().Artists != nil {
		src, _ = h.lib().Artists.(releaseSource)
	}
	if src == nil {
		return d
	}
	// Notable artists: the charts' only. The Library's artists are never
	// sent to the server; their releases come from the Library's own
	// albums (see exploreNewMusic).
	var ids []string
	seen := map[string]bool{}
	addID := func(id string) {
		id = strings.ToLower(id)
		if metadata.ValidMBID(id) && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for _, l := range []metadata.MusicTrend{d.artists, d.fresh, d.albums} {
		for _, it := range l.Items {
			if l.Kind == "artist" {
				addID(it.MBID)
			} else {
				addID(it.ArtistMBID)
			}
		}
	}
	if len(ids) > 3*musicNewArtists {
		ids = ids[:3*musicNewArtists]
	}
	day := func(n int) string { return now.AddDate(0, 0, n).Format("2006-01-02") }
	windows := []struct {
		from, to string
		types    []string
	}{
		{day(-musicNewLibrary), day(0), []string{"album", "ep"}},
		{day(-musicNewLibrary), day(0), []string{"single"}},
		{day(1), day(musicNewAhead), []string{"album", "ep"}},
	}
	have := map[string]bool{}
	for _, w := range windows {
		for i := 0; i < len(ids); i += musicNewArtists {
			list, err := src.ReleasesBetween(ctx, w.from, w.to, w.types, ids[i:min(i+musicNewArtists, len(ids))], 100)
			if err != nil {
				if errors.Is(err, metadata.ErrServiceBusy) || ctx.Err() != nil {
					return d
				}
				continue
			}
			for _, r := range list {
				if !have[r.MBID] {
					have[r.MBID] = true
					d.releases = append(d.releases, r)
				}
			}
		}
	}
	return d
}

// musicNewInput is what one member's page is built from.
type musicNewInput struct {
	data    *musicNewData
	owned   map[string]int64 // artist MBID → title id
	images  map[string]string
	library []MusicCard // Library releases of the last 90 days and the next 60
	follows map[string]bool
}

func daysFrom(date string, now time.Time) (int, bool) {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return 0, false
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return int(today.Sub(t).Hours() / 24), true
}

func releaseBadge(kind string, upcoming bool) string {
	switch {
	case upcoming && kind == "ep":
		return "EP coming"
	case upcoming:
		return "Album coming"
	case kind == "ep":
		return "New EP"
	case kind == "single":
		return "New single"
	}
	return "New album"
}

// musicNewTopGenres are the most common tags of the Library artists'
// new releases, else of all new releases; ties by name, so stable.
func musicNewTopGenres(cards map[string]MusicCard, n int) []string {
	count := func(lib bool) []string {
		hits := map[string]int{}
		name := map[string]string{}
		for _, c := range cards {
			if lib && c.TitleID == 0 {
				continue
			}
			for _, t := range c.Genres {
				k := strings.ToLower(strings.TrimSpace(t))
				if k == "" {
					continue
				}
				hits[k]++
				if name[k] == "" {
					name[k] = strings.ToUpper(k[:1]) + k[1:]
				}
			}
		}
		var ks []string
		for k := range hits {
			ks = append(ks, k)
		}
		sort.Slice(ks, func(i, j int) bool {
			if hits[ks[i]] != hits[ks[j]] {
				return hits[ks[i]] > hits[ks[j]]
			}
			return ks[i] < ks[j]
		})
		if len(ks) > n {
			ks = ks[:n]
		}
		for i := range ks {
			ks[i] = name[ks[i]]
		}
		return ks
	}
	if g := count(true); len(g) > 0 {
		return g
	}
	return count(false)
}

// musicNewTopics are the Explore more chips: the new releases' top genres,
// then decades and moods, each a Search → Music genre link.
func musicNewTopics(rows []MusicRow, hero []MusicCard) []string {
	count := map[string]int{}
	for _, c := range hero {
		for _, g := range c.Genres {
			count[g]++
		}
	}
	for _, r := range rows {
		for _, c := range r.Items {
			for _, g := range c.Genres {
				count[g]++
			}
		}
	}
	// Only short, plain tags: some releases carry whole sentences as tags.
	for g := range count {
		if len(g) > 24 || strings.ContainsAny(g, ":—,;()") {
			delete(count, g)
		}
	}
	return topGenres(count, 8)
}
