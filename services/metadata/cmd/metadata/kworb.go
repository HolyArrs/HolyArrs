package main

import (
	"context"
	"net/http"
	"path/filepath"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/api"
	"github.com/CoderLuii/holyarrs-metadata/internal/budget"
	"github.com/CoderLuii/holyarrs-metadata/internal/kworb"
	"github.com/CoderLuii/holyarrs-metadata/internal/lists"
	"github.com/CoderLuii/holyarrs-metadata/internal/providers"
)

// openKworb starts the kworb reader (artist popularity, the rising row and
// the song charts) with the music job and the speed system: its kill
// switch is the control file's "kworb" provider, it reads the lists'
// regions and crawls the catalog's artists, its MusicBrainz lookups share
// the MusicBrainz pace, and the picture chains' link matches and the
// stored artist answers' links are used before any call.
func openKworb(c config, sp *speed, direct *providers.Client, upstream *budget.Budget, userAgent string, o *api.Options) (*kworb.Service, error) {
	if !c.MusicTrending || sp == nil {
		return nil, nil
	}
	mb := budget.Client(upstream, 30*time.Second)
	base := mb.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	mb.Transport = pacedTransport{next: base, client: direct}
	k, err := kworb.Open(filepath.Join(c.DataDir, "kworb.sqlite"), kworb.Options{HTTP: budget.Client(upstream, 30*time.Second), UserAgent: userAgent,
		On:      func() bool { return sp.ctl.Provider("kworb") },
		Regions: lists.TopRegions,
		Catalog: func() []string {
			if sp.catalog == nil {
				return nil
			}
			return sp.catalog.Artists()
		},
		MBHTTP: mb, WDHTTP: budget.Client(upstream, 30*time.Second),
		KeptSpotify: sp.imgs.LinkedSpotify, KeptArtist: sp.imgs.LinkedArtist,
		StoredLinks: sp.links})
	if err != nil {
		return nil, err
	}
	o.Popularity = k
	prev := o.StatusExtra
	o.StatusExtra = func(ctx context.Context, window time.Duration) map[string]any {
		m := map[string]any{}
		if prev != nil {
			m = prev(ctx, window)
		}
		m["kworb"] = k.Stats()
		return m
	}
	return k, nil
}
