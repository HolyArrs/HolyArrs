package musictrend

import (
	"context"
	"strings"
	"time"
)

// ListCharts is the paid list source's charts and Browse genre pages (the
// lists package), already matched to MusicBrainz ids.
type ListCharts interface {
	// ChartList answers a region's chart ("songs", "albums" or "artists"):
	// entries best first, with MusicBrainz ids where matched (MBID for an
	// album or artist, ArtistMBID for a song's or album's artist), and the
	// same chart RankDays ago when kept. False when the region has none.
	ChartList(ctx context.Context, region, kind string) (ListChart, bool)
	// GenreTiles answers the genre pages that hold items, in the source's
	// budget order (biggest genres first).
	GenreTiles() []GenreTile
	// Regions answers the regions with a kept song chart ("global" first).
	Regions() []string
	// Built answers when the source's lists were last built.
	Built() time.Time
}

// ListChart is one chart of the list source.
type ListChart struct {
	Entries []Entry
	// Ago is the chart RankDays ago; HaveAgo is false when not kept.
	Ago     []Entry
	HaveAgo bool
}

// GenreTile is one genre page of the list source, with its artists for
// the tile art (MusicBrainz ids as ID).
type GenreTile struct {
	Slug, Name string
	Count      int
	Artists    []NamedArtist
}

// listRegions are the regions the list source keeps a song chart for.
func (s *Service) listRegions() []string {
	if s.opts.Lists == nil {
		return nil
	}
	return s.opts.Lists.Regions()
}

// listChart answers a region's albums or songs chart from the list source
// as chart items (no genres, explicit marks or covers: the app reads
// covers by MusicBrainz id); false when the region has none.
func (s *Service) listChart(ctx context.Context, region, kind string, now time.Time) ([]ChartItem, bool) {
	if s.opts.Lists == nil {
		return nil, false
	}
	r := strings.ToLower(region)
	if r == "" {
		r = "global"
	}
	c, ok := s.opts.Lists.ChartList(ctx, r, kind)
	if !ok || len(c.Entries) == 0 {
		return nil, false
	}
	ago := map[string]int{}
	for i, e := range c.Ago {
		if _, seen := ago[entryKey(e)]; !seen {
			ago[entryKey(e)] = i + 1
		}
	}
	L := len(c.Entries)
	out := make([]ChartItem, 0, L)
	for i, e := range c.Entries {
		it := ChartItem{Rank: i + 1, ChartRank: i + 1, MBID: e.MBID, ArtistMBID: e.ArtistMBID, Unlinked: e.Unlinked, Title: e.Name, Artist: e.Artist, Date: e.Date, Genres: []string{}, Source: SrcLists}
		if kind == "songs" {
			it.MBID = ""
		}
		it.P = Popularity(it.Rank, L)
		it.F7, it.F14, it.F60 = Freshness(e.Date, 7, now), Freshness(e.Date, 14, now), Freshness(e.Date, 60, now)
		if c.HaveAgo {
			r := ago[entryKey(e)]
			g := RankGain(r, it.Rank, L)
			it.RankAgo, it.RankGain = &r, &g
		}
		out = append(out, it)
	}
	return out, true
}
