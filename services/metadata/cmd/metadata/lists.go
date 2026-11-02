package main

import (
	"context"
	"net/http"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/api"
	"github.com/CoderLuii/holyarrs-metadata/internal/budget"
	"github.com/CoderLuii/holyarrs-metadata/internal/kworb"
	"github.com/CoderLuii/holyarrs-metadata/internal/lists"
	"github.com/CoderLuii/holyarrs-metadata/internal/musictrend"
	"github.com/CoderLuii/holyarrs-metadata/internal/providers"
	"github.com/CoderLuii/holyarrs-metadata/internal/sharedfill"
)

// openLists starts the world-lists collector when LISTS_KEY is set (with
// the music job on). LISTS_MONTH_CAP is the monthly request cap (default
// 900, the free plan; about 6000 turns on the full set on a paid plan).
func openLists(c config, env func(string) string, search musictrend.Searcher, link func(context.Context, string, string) (string, error), kw *kworb.Service, sp *speed, options *api.Options) (*lists.Service, error) {
	key := strings.TrimSpace(env("LISTS_KEY"))
	if !c.MusicTrending || key == "" {
		return nil, nil
	}
	capMonth, _ := strconv.Atoi(strings.TrimSpace(env("LISTS_MONTH_CAP")))
	// LISTS_PLAN_LIMIT is the plan's requests per period (default 1000);
	// the cap is held to half of it.
	planLimit, _ := strconv.Atoi(strings.TrimSpace(env("LISTS_PLAN_LIMIT")))
	o := lists.Options{Key: key, MonthCap: capMonth, PlanLimit: planLimit, Search: search, Link: link}
	if kw != nil && sp != nil {
		// Song charts: the kworb reader's first, this source's as backup.
		o.Songs, o.Tracks = kworbSongs(kw), spotifyTracks(sp)
	}
	ls, err := lists.Open(filepath.Join(c.DataDir, "lists.sqlite"), o)
	if err != nil {
		return nil, err
	}
	options.Shelves = ls
	prev := options.StatusExtra
	options.StatusExtra = func(ctx context.Context, window time.Duration) map[string]any {
		m := map[string]any{}
		if prev != nil {
			m = prev(ctx, window)
		}
		m["lists"] = ls.Status(ctx)
		return m
	}
	return ls, nil
}

// attachLists feeds the shelves into the shared fill (pictures, artist
// details and videos ahead, on start and on each pass) and queues members
// new after each list refresh.
func attachLists(ls *lists.Service, sp *speed) {
	if ls == nil || sp == nil || sp.fill == nil {
		return
	}
	sp.fill.Lists["lists"] = func(ctx context.Context) []sharedfill.Member {
		var out []sharedfill.Member
		for _, m := range ls.Members(ctx) {
			out = append(out, sharedfill.Member{Kind: m.Kind, MBID: m.MBID})
		}
		return out
	}
	// Albums' first release dates from stored MusicBrainz answers (the
	// shared fill keeps every shelf album's), never a call.
	ls.SetRGInfo(lists.RGInfo(func(ctx context.Context, kind, mbid string) ([]byte, bool) {
		if sp.handler == nil {
			return nil, false
		}
		return sp.handler.StoredData(ctx, kind, mbid)
	}))
	waiting := ls.Backlog
	sp.listsWaiting.Store(&waiting)
	ls.SetPrefetch(func(kind, mbid string) {
		sp.enqueueShared(context.Background(), sharedfill.Member{Kind: kind, MBID: mbid})
	})
}

// kworbSongs reads the kworb reader's kept daily charts for the lists.
func kworbSongs(kw *kworb.Service) func(region string) (lists.SongChart, bool) {
	return func(region string) (lists.SongChart, bool) {
		st, ok := kw.SongChart(region, false)
		if !ok {
			return lists.SongChart{}, false
		}
		c := lists.SongChart{Day: st.Chart.Day, At: st.At, Failed: st.Failed}
		for _, r := range st.Chart.Rows {
			c.Rows = append(c.Rows, lists.SongRow{Rank: r.Rank, Move: r.Move, Title: r.Title, Artist: r.Artist, ArtistID: r.ArtistID, TrackID: r.TrackID, Days: r.Days, Peak: r.Peak, Streams: r.Streams})
		}
		return c, true
	}
}

// spotifyTracks answers songs' albums from the Spotify API (GET
// /v1/tracks, 50 a call) through the picture chains' Spotify client.
func spotifyTracks(sp *speed) func(ctx context.Context, ids []string) (map[string]lists.TrackAlbum, error) {
	return func(ctx context.Context, ids []string) (map[string]lists.TrackAlbum, error) {
		var d struct {
			Tracks []*struct {
				ID    string `json:"id"`
				Album struct {
					ID     string `json:"id"`
					Name   string `json:"name"`
					Type   string `json:"album_type"`
					Date   string `json:"release_date"`
					Images []struct {
						URL string `json:"url"`
					} `json:"images"`
				} `json:"album"`
			} `json:"tracks"`
		}
		out := map[string]lists.TrackAlbum{}
		ok, err := sp.res.SpotifyJSON(ctx, "/v1/tracks?ids="+strings.Join(ids, ",")+"&market=US", &d)
		if err != nil || !ok {
			return out, err
		}
		for _, t := range d.Tracks {
			if t == nil || t.Album.Name == "" {
				continue
			}
			a := lists.TrackAlbum{ID: t.Album.ID, Name: t.Album.Name, Type: t.Album.Type, ReleaseDate: t.Album.Date}
			if len(t.Album.Images) > 0 {
				a.Image = lists.ImageID(t.Album.Images[0].URL)
			}
			out[t.ID] = a
		}
		return out, nil
	}
}

// listsLink is the MusicBrainz link lookup for list artists, paced with
// every other MusicBrainz call.
func listsLink(direct *providers.Client, upstream *budget.Budget, userAgent string) func(context.Context, string, string) (string, error) {
	hc := budget.Client(upstream, 30*time.Second)
	base := hc.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	hc.Transport = pacedTransport{next: base, client: direct}
	return lists.MBLink(hc, "https://musicbrainz.org/ws/2", userAgent)
}

// freshFrom reads ListenBrainz's fresh releases out in the last 14 days
// from the new-music windows (no call), for the new-release shelf.
func freshFrom(n *musictrend.NewMusic) func() []lists.FreshAlbum {
	return func() []lists.FreshAlbum {
		now := time.Now().UTC()
		var out []lists.FreshAlbum
		for _, it := range n.Releases(now.AddDate(0, 0, -14).Format("2006-01-02"), now.Format("2006-01-02"), nil, now) {
			if slices.Contains(it.Sources, "listenbrainz") {
				out = append(out, lists.FreshAlbum{MBID: it.MBID, ArtistMBID: it.ArtistMBID, Title: it.Title, Artist: it.Artist, Date: it.ReleaseDate, Listens: it.Listens})
			}
		}
		return out
	}
}
