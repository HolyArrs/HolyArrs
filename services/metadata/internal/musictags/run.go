package musictags

import (
	"bytes"
	"context"
	"encoding/json"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/musictrend"
)

// Fetcher answers GET and POST calls (with its own cache and pacing).
type Fetcher interface {
	Get(ctx context.Context, u string) ([]byte, error)
	Post(ctx context.Context, u string, body []byte) ([]byte, error)
}

// ChartRow is one country's chart (kind "albums" or "songs").
type ChartRow struct {
	Country, Kind string
	Items         []musictrend.ChartItem
}

// ShelfItem is one album or artist on a list shelf.
type ShelfItem struct {
	Kind, MBID, ArtistMBID, Title, Artist, Date string
}

// Config says where Collect reads from.
type Config struct {
	LB, MB string // ListenBrainz API root (…/1) and MusicBrainz ws/2 root
	Keep   int    // albums kept per artist (10)
	// MaxArtists caps the artists browsed on MusicBrainz in one run (0: all).
	MaxArtists int
	Charts     func(ctx context.Context) []ChartRow
	Log        func(format string, args ...any)
	// Shelves answers the list shelves' albums and artists (their covers
	// are fetched ahead by the server), so list rows find them.
	Shelves func(ctx context.Context) []ShelfItem
	// SearchTags answers tags to search MusicBrainz release groups for
	// (keys whose pools are small after the main pass); nil skips it.
	SearchTags func(src *Source) []string
}

func (c Config) logf(format string, args ...any) {
	if c.Log != nil {
		c.Log(format, args...)
	}
}

// Collect gathers the catalog: listen ranks (artists and albums, top 1,000
// per range), the charts, fresh releases, every seed artist's studio
// albums (MusicBrainz browse), their listen counts, and tags for the kept
// albums. A failed call skips its part; only a context end stops the run.
func Collect(ctx context.Context, lb, mb Fetcher, cfg Config) (*Source, error) {
	if cfg.Keep <= 0 {
		cfg.Keep = 10
	}
	src := NewSource()
	known := map[string]bool{}
	var seeds []string
	seed := func(id string) {
		id = strings.ToLower(id)
		if id != "" && !known[id] {
			known[id] = true
			seeds = append(seeds, id)
		}
	}
	for ri, rng := range RangeNames {
		if b, err := lb.Get(ctx, cfg.LB+"/stats/sitewide/artists?range="+rng+"&count=1000"); err == nil {
			if es, _, err := musictrend.ParseLBArtists(bytes.NewReader(b)); err == nil {
				for i, e := range es {
					a := Artist{ID: e.MBID, Name: e.Name}
					a.Ranks[ri] = i + 1
					src.Artist(a)
					seed(e.MBID)
				}
			}
		}
		if b, err := lb.Get(ctx, cfg.LB+"/stats/sitewide/release-groups?range="+rng+"&count=1000"); err == nil {
			if es, _, err := musictrend.ParseLBAlbums(bytes.NewReader(b)); err == nil {
				for i, e := range es {
					a := Album{ID: e.MBID, Title: e.Name, Artist: e.Artist, ArtistID: e.ArtistMBID, Art: e.CAARelease != ""}
					a.Ranks[ri] = i + 1
					src.Album(a)
					seed(e.ArtistMBID)
				}
			}
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
	if cfg.Charts != nil {
		for _, ch := range cfg.Charts(ctx) {
			for _, it := range ch.Items {
				if ch.Kind == "albums" && it.MBID != "" {
					src.Album(Album{ID: it.MBID, Title: it.Title, Artist: it.Artist, ArtistID: it.ArtistMBID, Date: it.Date, Art: it.Cover != ""})
					src.Chart(ch.Country, it.MBID, it.Rank, it.Genres...)
					if it.RankGain != nil {
						src.ChartGain(ch.Country, it.MBID, *it.RankGain)
					}
				}
				if ch.Kind == "songs" && it.ArtistMBID != "" {
					src.Artist(Artist{ID: it.ArtistMBID, Name: it.Artist, Art: true})
					src.ArtistChart(ch.Country, it.ArtistMBID, it.Rank)
				}
				seed(it.ArtistMBID)
			}
		}
	}
	if cfg.Shelves != nil {
		for _, it := range cfg.Shelves(ctx) {
			if it.Kind == "artist" {
				src.Artist(Artist{ID: it.MBID, Name: it.Artist, Art: true})
				seed(it.MBID)
				continue
			}
			src.Album(Album{ID: it.MBID, Title: it.Title, Artist: it.Artist, ArtistID: it.ArtistMBID, Date: it.Date, Art: true, Listed: true})
			seed(it.ArtistMBID)
		}
	}
	if b, err := lb.Get(ctx, cfg.LB+"/explore/fresh-releases/?days=90&sort=release_date&past=true&future=false"); err == nil {
		if es, dates, tg, err := musictrend.ParseLBFresh(bytes.NewReader(b)); err == nil {
			for _, e := range es {
				var ts []Tag
				for _, t := range tg[e.MBID] {
					ts = append(ts, Tag{Name: t, Votes: 1})
				}
				src.Album(Album{ID: e.MBID, Title: e.Name, Artist: e.Artist, ArtistID: e.ArtistMBID, Date: dates[e.MBID], Tags: ts, Art: e.CAARelease != ""})
			}
		}
	}
	cfg.logf("musictags: %d seed artists", len(seeds))

	if cfg.MaxArtists > 0 && len(seeds) > cfg.MaxArtists {
		seeds = seeds[:cfg.MaxArtists]
	}
	byArtist := map[string][]Album{}
	all := src.AlbumIDs()
	for _, id := range seeds {
		b, err := mb.Get(ctx, cfg.MB+"/release-group?artist="+id+"&type=album%7Cep&limit=100&fmt=json")
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err != nil {
			continue
		}
		gs, _, err := ParseMBGroups(bytes.NewReader(b))
		if err != nil {
			continue
		}
		for j := range gs {
			gs[j].ArtistID, gs[j].Artist = id, src.ArtistName(id)
			all = append(all, gs[j].ID)
		}
		byArtist[id] = gs
	}
	pop := map[string][2]int64{}
	if err := popularity(ctx, lb, cfg, all, pop); err != nil {
		return nil, err
	}
	for id, gs := range byArtist {
		for j := range gs {
			gs[j].Listens, gs[j].Listeners = pop[gs[j].ID][0], pop[gs[j].ID][1]
		}
		sort.SliceStable(gs, func(a, b int) bool { return gs[a].Listens > gs[b].Listens })
		var sum int64
		for j, g := range gs {
			sum += g.Listens
			if j < cfg.Keep && g.Listens > 0 {
				src.Album(g)
			}
		}
		src.Artist(Artist{ID: id, Listens: sum, Art: len(gs) > 0})
	}
	for _, id := range src.AlbumIDs() {
		if v, ok := pop[id]; ok {
			src.Album(Album{ID: id, Listens: v[0], Listeners: v[1]})
		}
	}
	tagged, err := tagAlbums(ctx, lb, cfg, src)
	if err != nil {
		return nil, err
	}
	searched := 0
	if cfg.SearchTags != nil {
		var found []Album
		for _, tag := range cfg.SearchTags(src) {
			for _, off := range []string{"0", "100"} {
				q := "tag:\"" + tag + "\" AND (primarytype:album OR primarytype:ep)"
				b, err := mb.Get(ctx, cfg.MB+"/release-group?query="+url.QueryEscape(q)+"&limit=100&offset="+off+"&fmt=json")
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				if err != nil {
					break
				}
				gs, err := ParseMBSearch(bytes.NewReader(b))
				if err != nil || len(gs) == 0 {
					break
				}
				found = append(found, gs...)
			}
		}
		var ids []string
		for _, g := range found {
			if !src.HasAlbum(g.ID) {
				ids = append(ids, g.ID)
			}
		}
		more := map[string][2]int64{}
		if err := popularity(ctx, lb, cfg, ids, more); err != nil {
			return nil, err
		}
		for _, g := range found {
			if v := more[g.ID]; v[0] > 0 && !src.HasAlbum(g.ID) {
				g.Listens, g.Listeners = v[0], v[1]
				src.Album(g)
				searched++
			}
		}
		n, err := tagAlbums(ctx, lb, cfg, src)
		if err != nil {
			return nil, err
		}
		tagged += n
	}
	cfg.logf("musictags: %d artists browsed, %d release groups counted, %d albums tagged, %d found by tag search", len(byArtist), len(pop), tagged, searched)
	return src, nil
}

// popularity reads listen counts for ids in batches of 300.
func popularity(ctx context.Context, lb Fetcher, cfg Config, ids []string, pop map[string][2]int64) error {
	all := append([]string{}, ids...)
	sort.Strings(all)
	all = dedupe(all)
	for i := 0; i < len(all); i += 300 {
		part := all[i:min(i+300, len(all))]
		body, _ := json.Marshal(map[string][]string{"release_group_mbids": part})
		b, err := lb.Post(ctx, cfg.LB+"/popularity/release-group", body)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			continue
		}
		if m, err := ParsePopularity(bytes.NewReader(b)); err == nil {
			for k, v := range m {
				pop[k] = v
			}
		}
	}
	return nil
}

// tagAlbums reads tags, dates and cover flags for albums with no tags, 25
// at a time, and answers how many it looked up.
func tagAlbums(ctx context.Context, lb Fetcher, cfg Config, src *Source) (int, error) {
	missing := src.Untagged()
	sort.Strings(missing)
	for i := 0; i < len(missing); i += 25 {
		part := missing[i:min(i+25, len(missing))]
		q := url.Values{"release_group_mbids": {strings.Join(part, ",")}, "inc": {"tag"}}
		b, err := lb.Get(ctx, cfg.LB+"/metadata/release_group/?"+q.Encode())
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		if err != nil {
			continue
		}
		if m, err := ParseGroupTags(bytes.NewReader(b)); err == nil {
			for _, a := range m {
				src.Album(a)
			}
		}
	}
	return len(missing), nil
}

func dedupe(sorted []string) []string {
	out := sorted[:0]
	for i, s := range sorted {
		if i == 0 || s != sorted[i-1] {
			out = append(out, s)
		}
	}
	return out
}

// SmallKeyTags answers tags to search for: the core tags (weight 1, at
// most perKey each, own tags first) of every genre, mood, season and
// activity key with fewer than minAlbums albums in a provisional build.
func SmallKeyTags(src *Source, rs []Recipe, now time.Time, minAlbums, perKey int) []string {
	b := Build(src, rs, now)
	var out []string
	seen := map[string]bool{}
	for _, r := range rs {
		if r.Kind != KindGenre && r.Kind != KindMood && r.Kind != KindSeason && r.Kind != KindActivity {
			continue
		}
		n := 0
		for _, m := range b.Keys[r.Key] {
			if b.Items[m.I].Kind == "a" {
				n++
			}
		}
		if n >= minAlbums {
			continue
		}
		var core []string
		core = append(core, r.Own...)
		var rest []string
		for tag, w := range r.Tags {
			if w >= 1 {
				rest = append(rest, tag)
			}
		}
		sort.Strings(rest)
		core = append(core, rest...)
		k := 0
		for _, tag := range core {
			if k == perKey {
				break
			}
			if !seen[tag] {
				seen[tag] = true
				out = append(out, tag)
				k++
			}
		}
	}
	return out
}
