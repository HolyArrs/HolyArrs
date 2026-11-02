package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/api"
	"github.com/CoderLuii/holyarrs-metadata/internal/fetchq"
	"github.com/CoderLuii/holyarrs-metadata/internal/imgstore"
	"github.com/CoderLuii/holyarrs-metadata/internal/lists"
	"github.com/CoderLuii/holyarrs-metadata/internal/mbread"
	"github.com/CoderLuii/holyarrs-metadata/internal/musictrend"
)

// Charts and new releases (music data plan S5): one source order
// (spotify81, Apple Music, ListenBrainz), genre pages from the slim copy's
// tags plus ListenBrainz, and the picture prefetch that gates them.

// slimTags answers a release group's or artist's genre names from the
// slim copy (its tags when it has no genre); no call.
func slimTags(slim *mbread.Reader) func(ctx context.Context, kind, mbid string) []string {
	return func(ctx context.Context, kind, mbid string) []string {
		b, _, ok := slim.Stored(ctx, kind, mbid)
		if !ok {
			return nil
		}
		var v struct {
			Data struct {
				Genres []struct{ Name string } `json:"genres"`
				Tags   []struct{ Name string } `json:"tags"`
			} `json:"source_data"`
		}
		if json.Unmarshal(b, &v) != nil {
			return nil
		}
		var out []string
		for _, g := range v.Data.Genres {
			out = append(out, g.Name)
		}
		if len(out) == 0 {
			for _, t := range v.Data.Tags {
				out = append(out, t.Name)
			}
		}
		return out
	}
}

// slimAlbums answers an artist's albums (primary type Album, no secondary
// type) from the slim copy; no call.
func slimAlbums(slim *mbread.Reader) func(ctx context.Context, mbid string) []string {
	return func(ctx context.Context, mbid string) []string {
		a, ok := slim.Artist(ctx, mbid)
		if !ok {
			return nil
		}
		var out []string
		for _, g := range a.Groups {
			if strings.EqualFold(g.PrimaryType, "Album") && len(g.SecondaryTypes) == 0 {
				out = append(out, strings.ToLower(g.ID))
			}
		}
		return out
	}
}

// slimAlbumByTitle answers an artist's release group whose title names a
// charted one, from the slim copy (no call). Each artist's titles are
// read once per copy and kept, an artist the copy lacks too (copyMemo; at
// most 5,000 artists).
func slimAlbumByTitle(slim *mbread.Reader) func(ctx context.Context, artist, title string) string {
	type group struct {
		id, title string
		album     bool // a plain album (primary Album, no secondary type)
	}
	memo := newCopyMemo[[]group](slim.Date, 5000)
	return func(ctx context.Context, artist, title string) string {
		gs := memo.get(ctx, artist, func() []group {
			a, _ := slim.Artist(ctx, artist)
			var gs []group
			for _, g := range a.Groups {
				gs = append(gs, group{strings.ToLower(g.ID), g.Title, strings.EqualFold(g.PrimaryType, "Album") && len(g.SecondaryTypes) == 0})
			}
			return gs
		})
		for _, g := range gs {
			if musictrend.SlimTitleMatch(g.title, title) {
				return g.id
			}
		}
		// A charted edition ("The Life of a Showgirl: The Encore") is
		// filed under its album: taken only when one group has that title,
		// or one plain album among the groups that do (an "Other" group
		// often shares the album's title).
		if stem := musictrend.EditionStem(title); stem != "" {
			var one, album string
			nAny, nAlbum := 0, 0
			for _, g := range gs {
				if musictrend.SlimTitleMatch(g.title, stem) {
					one, nAny = g.id, nAny+1
					if g.album {
						album, nAlbum = g.id, nAlbum+1
					}
				}
			}
			switch {
			case nAny == 1:
				return one
			case nAlbum == 1:
				return album
			}
		}
		return ""
	}
}

// listsNew is the list source's new releases and global song chart as a
// new-release source (stored lists, no call).
func listsNew(ls *lists.Service) musictrend.NewSource {
	return musictrend.NewSource{Name: musictrend.SrcLists, Read: func(ctx context.Context) (albums, tracks []musictrend.NewItem, err error) {
		if c, ok := ls.ChartList(ctx, "us", "new"); ok {
			for _, e := range c.Entries {
				albums = append(albums, musictrend.NewItem{MBID: e.MBID, ArtistMBID: e.ArtistMBID, Title: e.Name, Artist: e.Artist, ReleaseDate: e.Date, Type: "album"})
			}
		}
		if c, ok := ls.ChartList(ctx, "global", "songs"); ok {
			for i, e := range c.Entries {
				tracks = append(tracks, musictrend.NewItem{ArtistMBID: e.ArtistMBID, Title: e.Name, Artist: e.Artist, Track: e.Name, ReleaseDate: e.Date, Popularity: i + 1})
			}
		}
		return albums, tracks, nil
	}}
}

// appleNew is Apple Music's kept US chart as a new-release source: its
// albums out in the windows and its songs (stored charts, no call).
func appleNew(music *musictrend.Service) musictrend.NewSource {
	return musictrend.NewSource{Name: musictrend.SrcApple, Read: func(ctx context.Context) (albums, tracks []musictrend.NewItem, err error) {
		now := time.Now()
		if items, ok := music.AppleCharts(ctx, "global", "albums", now); ok {
			for _, it := range items {
				albums = append(albums, musictrend.NewItem{MBID: it.MBID, ArtistMBID: it.ArtistMBID, Title: it.Title, Artist: it.Artist, ReleaseDate: it.Date, Cover: it.Cover, Explicit: it.Explicit, Popularity: it.Rank, Type: "album"})
			}
		}
		if items, ok := music.AppleCharts(ctx, "global", "songs", now); ok {
			for _, it := range items {
				tracks = append(tracks, musictrend.NewItem{MBID: it.MBID, ArtistMBID: it.ArtistMBID, Title: it.Title, Artist: it.Artist, Track: it.Title, ReleaseDate: it.Date, Cover: it.Cover, Explicit: it.Explicit, Popularity: it.Rank})
			}
		}
		return albums, tracks, nil
	}}
}

// storePictures is the picture store as the prefetch sees it.
type storePictures struct{ s *imgstore.Store }

func (p storePictures) Provider(ctx context.Context, kind, id string) (string, bool) {
	r, ok := p.s.Record(ctx, kind, id)
	return r.Provider, ok
}
func (p storePictures) Missed(ctx context.Context, kind, id string) bool {
	return p.s.Missed(ctx, kind, id)
}
func (p storePictures) Fetch(ctx context.Context, kind, id string) error {
	return p.s.Fetch(ctx, kind, id)
}

// openChartPrefetch wires the source order, the genre pages and the
// picture prefetch with its gate. Without the picture store or the slim
// copy the gate stays open (nothing to check against).
func openChartPrefetch(music *musictrend.Service, newMusic *musictrend.NewMusic, ls *lists.Service, slim *mbread.Reader, sp *speed, options *api.Options, dataDir string) *musictrend.Prefetch {
	if music == nil {
		return nil
	}
	if newMusic != nil {
		var src []musictrend.NewSource
		if ls != nil {
			src = append(src, listsNew(ls))
		}
		src = append(src, appleNew(music))
		newMusic.SetSources(src...)
	}
	if slim == nil {
		return nil
	}
	music.SetTags(slimTags(slim))
	music.SetSlimAlbum(slimAlbumByTitle(slim))
	// A chart row's own artist id (or its album's barcode) beats the name.
	var imgs *imgstore.Store
	if sp != nil {
		imgs = sp.imgs
	}
	exact := newChartArtists(slim, imgs, dataDir)
	music.SetExactArtist(exact.resolve)
	// Never the wrong artist: a shared name without exact evidence links to
	// none.
	shared := sharedName(slim.NameHolders, slim.Date)
	music.SetSharedName(shared)
	go func() {
		for range time.Tick(10 * time.Minute) {
			log.Printf("chart artists by own id: %v", exact.Counts())
		}
	}()
	if ls != nil {
		ls.SetExactArtist(exact.resolve)
		ls.SetSharedName(shared)
	}
	if sp == nil || sp.imgs == nil {
		return nil
	}
	pf := &musictrend.Prefetch{Pictures: storePictures{sp.imgs}, Albums: slimAlbums(slim), Log: log.Printf,
		LiveFile: filepath.Join(filepath.Dir(sp.cfg.ControlFile), "charts-live"),
		Paused: func(err error) bool {
			return errors.Is(err, imgstore.ErrSourcePaused) || strings.Contains(err.Error(), "source paused") || strings.Contains(err.Error(), "answered 429")
		},
		Artists: func(ctx context.Context) []string {
			out := music.AllChartArtists(ctx)
			if newMusic != nil {
				_, artists := newMusic.SharedMembers(time.Now())
				out = append(out, artists...)
			}
			return out
		}}
	if sim, ok := options.Similar.(*musictrend.Similar); ok {
		// The prefetch asks for similar artists side by side: ListenBrainz
		// (one MetaBrainz gateway, which cut the VM off at 4/s) is held to
		// one a second here, on top of its hourly budget. Apple's similar
		// goes through the picture resolver, paced at 10/s on one address.
		sim.HTTP = (&hostPace{gap: func(host string) time.Duration {
			if strings.HasSuffix(strings.ToLower(host), "listenbrainz.org") {
				return time.Second
			}
			return 0
		}}).paced(sim.HTTP)
		pf.Similar = func(ctx context.Context, mbid string) []string {
			b, _, err := sim.Artists(ctx, mbid)
			if err != nil {
				return nil
			}
			var v struct {
				Artists []musictrend.SimilarArtist `json:"artists"`
			}
			if json.Unmarshal(b, &v) != nil {
				return nil
			}
			var out []string
			for _, a := range v.Artists {
				out = append(out, strings.ToLower(a.MBID))
			}
			return out
		}
	}
	pf.Enqueue = func(kind, mbid string) {
		ctx := context.Background()
		key := "img:" + kind + ":" + mbid
		if st, _ := sp.imgq.State(ctx, key); st == "dead" && !sp.imgs.Missed(ctx, kind, mbid) {
			// Died without a recorded miss (waits or a busy source ran out):
			// the chart still wants it.
			sp.imgq.Revive(ctx, key, fetchq.P2Shared)
			return
		}
		sp.imgq.Enqueue(ctx, key, fetchq.P2Shared, false)
	}
	options.ChartsReady = pf.Ready
	prev := options.StatusExtra
	options.StatusExtra = func(ctx context.Context, window time.Duration) map[string]any {
		m := map[string]any{}
		if prev != nil {
			m = prev(ctx, window)
		}
		m["chart_prefetch"] = pf.Last()
		return m
	}
	return pf
}
