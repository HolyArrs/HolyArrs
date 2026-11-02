package main

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/api"
	"github.com/CoderLuii/holyarrs-metadata/internal/budget"
	"github.com/CoderLuii/holyarrs-metadata/internal/lists"
	"github.com/CoderLuii/holyarrs-metadata/internal/musictags"
	"github.com/CoderLuii/holyarrs-metadata/internal/musictrend"
	"github.com/CoderLuii/holyarrs-metadata/internal/providers"
)

// openMusicTags starts the music catalog job (the row templates' bundle)
// with the music job on: ListenBrainz through the upstream budget,
// MusicBrainz through the shared pacing slot, charts from the music job.
//
// idle (nil: always) holds its MetaBrainz calls while the MusicBrainz
// queue has P0 to P2 work: the catalog is the server's own slow work.
func openMusicTags(c config, music *musictrend.Service, ls *lists.Service, direct *providers.Client, upstream *budget.Budget, userAgent string, options *api.Options, idle func(context.Context) bool) (*musictags.Service, error) {
	if !c.MusicTrending || music == nil {
		return nil, nil
	}
	mb := budget.Client(upstream, 30*time.Second)
	base := mb.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	mb.Transport = idleTransport{next: pacedTransport{next: base, client: direct}, idle: idle, held: &musictagsHeld}
	lb := budget.Client(upstream, time.Minute)
	lb.Transport = idleTransport{next: lb.Transport, idle: idle, held: &musictagsHeld}
	charts := func(ctx context.Context) []musictags.ChartRow {
		var out []musictags.ChartRow
		now := time.Now()
		for _, st := range musictrend.Storefronts() {
			for _, kind := range []string{"albums", "songs"} {
				if items, ok := music.AppleCharts(ctx, st, kind, now); ok {
					out = append(out, musictags.ChartRow{Country: st, Kind: kind, Items: items})
				}
			}
		}
		return out
	}
	var shelves func(ctx context.Context) []musictags.ShelfItem
	if ls != nil {
		shelves = func(context.Context) []musictags.ShelfItem { return listShelfItems(ls) }
	}
	s, err := musictags.Open(filepath.Join(c.DataDir, "musictags.sqlite"), musictags.Options{HTTP: lb, MBHTTP: mb, UserAgent: userAgent, Charts: charts, Shelves: shelves, MBGap: time.Second})
	if err != nil {
		return nil, err
	}
	options.Pools = s
	return s, nil
}

// listShelfItems answers every region's shelf albums and artists.
func listShelfItems(ls *lists.Service) []musictags.ShelfItem {
	var out []musictags.ShelfItem
	seen := map[string]bool{}
	for _, region := range append([]string{"global"}, ls.Regions()...) {
		body, _, ok := ls.Shelves(region)
		if !ok {
			continue
		}
		var sh lists.Shelves
		if json.Unmarshal(body, &sh) != nil {
			continue
		}
		for _, items := range sh.Blocks {
			for _, it := range items {
				if seen[it.Kind+it.MBID] {
					continue
				}
				seen[it.Kind+it.MBID] = true
				out = append(out, musictags.ShelfItem{Kind: it.Kind, MBID: it.MBID, ArtistMBID: it.ArtistMBID, Title: it.Title, Artist: it.Artist, Date: it.Date})
			}
		}
	}
	return out
}

// musictagsHeld counts the catalog job's idle checks that found the
// MusicBrainz queue busy (each one a 2 s wait).
var musictagsHeld atomic.Int64
