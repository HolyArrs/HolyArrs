package api

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs/internal/api/musicscore"
	"github.com/CoderLuii/holyarrs/internal/metadata"
)

// Fresh & Trending → Music, flag music_rows_v6 (plan 2026-10-22 §2.7,
// Phase 6): one order for the whole household, from the region charts and
// the MusicBrainz release calendar. Rows: Spotlight (the hero), Fresh
// songs, Out this week, Climbing, Top albums, Coming soon; the page's
// Explore more links stay as they are. Climbing needs 7 days of chart
// ranks and stays hidden until the metadata server has them.

// musicTrendingHeroMax is how many Spotlight cards the hero shows.
const musicTrendingHeroMax = 6

type musicTrendingInput struct {
	region      string
	albums      []metadata.MusicChartItem
	songs       []metadata.MusicChartItem
	releases    []metadata.NewRelease
	listeners   map[string]int64 // artist MBID → ListenBrainz listeners
	owned       map[string]int64 // Library artist MBID → title id
	follows     map[string]bool  // album MBIDs the member follows
	ownedImages map[string]string
}

// albumRanks are a chart's ranks for musicscore.RanksReady.
func albumRanks(list []metadata.MusicChartItem) []musicscore.Ranked {
	out := make([]musicscore.Ranked, 0, len(list))
	for _, it := range list {
		r := musicscore.Ranked{Today: it.Rank}
		if it.RankAgo != nil {
			r.Ago, r.Known = *it.RankAgo, true
		}
		out = append(out, r)
	}
	return out
}

// climbNote is a Climbing card's reason line.
func climbNote(ago, today int) string {
	if ago <= 0 {
		return fmt.Sprintf("New entry · #%d", today)
	}
	return fmt.Sprintf("Up %d · #%d", ago-today, today)
}

func composeMusicTrending(in musicTrendingInput, now time.Time) ([]MusicCard, []MusicRow) {
	fold := func(title, artist string) string { return fullAlbumKey(title) + "|" + fullAlbumKey(artist) }
	seen := map[string]bool{}
	taken := func(c MusicCard) bool {
		return seen[c.Key] || (c.MBID != "" && seen["rg:"+c.MBID]) || seen["fold:"+fold(c.Name, c.Artist)]
	}
	mark := func(c MusicCard) {
		seen[c.Key], seen["fold:"+fold(c.Name, c.Artist)] = true, true
		if c.MBID != "" {
			seen["rg:"+c.MBID] = true
		}
	}
	withLibrary := func(c MusicCard) MusicCard {
		if id, ok := in.owned[c.ArtistMBID]; ok && c.ArtistMBID != "" {
			c.TitleID = id
		}
		return c
	}
	chartCard := func(it metadata.MusicChartItem) MusicCard {
		c := MusicCard{Kind: "album", MBID: strings.ToLower(it.MBID), Name: it.Title, Artist: it.Artist, ArtistMBID: strings.ToLower(it.ArtistMBID), Date: it.Date, Rank: it.Rank, Explicit: it.Explicit, Genres: append([]string{}, it.Genres...)}
		c.Key = "album:" + c.MBID
		if c.MBID == "" {
			c.Key = "chart:" + strings.ToLower(it.Artist+"|"+it.Title)
			c.Image = it.Cover
		} else {
			c.Image = "/api/v1/music-trending/art/album/" + c.MBID
		}
		c.Following = in.follows[c.MBID]
		return withLibrary(c)
	}
	var rows []MusicRow
	add := func(id, title, reason string, cards []MusicCard) {
		var items []MusicCard
		for _, c := range cards {
			if len(items) >= musicNewRowMax {
				break
			}
			if taken(c) {
				continue
			}
			items = append(items, c)
		}
		if len(items) < musicNewRowMin {
			return // a row under the gate gives nothing back
		}
		for _, c := range items {
			mark(c)
		}
		rows = append(rows, MusicRow{ID: id, Type: id, Title: title, Reason: reason, Shape: "square", Items: items})
	}
	type scoredCard struct {
		c MusicCard
		v float64
	}
	byScore := func(l []scoredCard) []MusicCard {
		sort.SliceStable(l, func(i, j int) bool {
			if l[i].v != l[j].v {
				return l[i].v > l[j].v
			}
			return l[i].c.Key < l[j].c.Key
		})
		out := make([]MusicCard, 0, len(l))
		for _, x := range l {
			out = append(out, x.c)
		}
		return out
	}

	// P per charted album (by MBID and by title + artist) and best P per artist.
	chartP := map[string]float64{}
	pcBest := map[string]float64{}
	for _, l := range [][]metadata.MusicChartItem{in.albums, in.songs} {
		for _, it := range l {
			if m := strings.ToLower(it.MBID); m != "" {
				chartP[m] = max(chartP[m], it.P)
			}
			k := fold(it.Title, it.Artist)
			chartP["fold:"+k] = max(chartP["fold:"+k], it.P)
			if m := strings.ToLower(it.ArtistMBID); m != "" {
				pcBest[m] = max(pcBest[m], it.P)
			}
		}
	}

	// Spotlight: charted albums with a release group, 0.6·P + 0.4·F7.
	var spot []scoredCard
	for _, it := range in.albums {
		c := chartCard(it)
		if c.MBID == "" {
			continue
		}
		c.ArtistImage = "/api/v1/music-trending/art/artist/" + c.ArtistMBID
		if img := in.ownedImages[c.ArtistMBID]; img != "" {
			c.ArtistImage = img
		}
		if days, ok := daysFrom(c.Date, now); ok && days <= 30 {
			c.Badge = releaseBadge("album", days < 0)
		} else {
			c.Badge = "Top album"
		}
		spot = append(spot, scoredCard{c, musicscore.ScoreTrending(it.P, musicscore.Freshness(it.Date, 7, now))})
	}
	hero := []MusicCard{}
	for _, c := range byScore(spot) {
		if len(hero) >= musicTrendingHeroMax {
			break
		}
		hero = append(hero, c)
		mark(c)
	}

	// Fresh songs: chart songs out within 14 days, one per artist,
	// 0.7·P + 0.3·F14; each line asks for its album (the artist).
	{
		var l []scoredCard
		per := map[string]bool{}
		for _, it := range in.songs {
			a := strings.ToLower(it.Artist)
			if it.Date == "" || it.Date < now.AddDate(0, 0, -14).Format("2006-01-02") || per[a] {
				continue
			}
			per[a] = true
			c := MusicCard{Kind: "song", Name: it.Title, Track: it.Title, Artist: it.Artist, ArtistMBID: strings.ToLower(it.ArtistMBID), Date: it.Date, Rank: it.Rank, Explicit: it.Explicit, Image: it.Cover, Genres: append([]string{}, it.Genres...)}
			c.Key = "song:" + strings.ToLower(it.Artist+"|"+it.Title)
			l = append(l, scoredCard{withLibrary(c), musicscore.ScoreFreshSong(it.P, musicscore.Freshness(it.Date, 14, now))})
		}
		add("np_music_tracks", "Fresh songs", "New songs on the chart; each one asks for its album", byScore(l))
	}

	// Out this week: MusicBrainz albums and EPs out in the last 7 days,
	// kept when charted or when the artist's listeners reach the pool
	// median; 0.6·P + 0.4·F7.
	{
		type cand struct {
			c       MusicCard
			p, ln   float64
			charted bool
		}
		var pool []cand
		var lis []float64
		var top int64
		for _, r := range in.releases {
			days, ok := daysFrom(r.ReleaseDate, now)
			kind := strings.ToLower(r.PrimaryType)
			if !ok || days < 0 || days > musicNewWeek || len(r.SecondaryTypes) > 0 || (kind != "album" && kind != "ep") {
				continue
			}
			top = max(top, in.listeners[strings.ToLower(r.ArtistMBID)])
		}
		for _, r := range in.releases {
			days, ok := daysFrom(r.ReleaseDate, now)
			kind := strings.ToLower(r.PrimaryType)
			if !ok || days < 0 || days > musicNewWeek || len(r.SecondaryTypes) > 0 || (kind != "album" && kind != "ep") {
				continue
			}
			id := strings.ToLower(r.MBID)
			c := withLibrary(MusicCard{Key: "album:" + id, Kind: "album", MBID: id, Name: r.Name, Artist: r.Artist, ArtistMBID: strings.ToLower(r.ArtistMBID), Date: r.ReleaseDate,
				Image: "/api/v1/music-trending/art/album/" + id, Badge: releaseBadge(kind, false), Following: in.follows[id], Genres: append([]string{}, r.Tags...)})
			p, charted := chartP[id]
			if fp, ok := chartP["fold:"+fold(r.Name, r.Artist)]; ok {
				p, charted = max(p, fp), true
			}
			ln := musicscore.Lnorm(in.listeners[c.ArtistMBID], top)
			if !charted {
				p = musicscore.Uncharted(ln)
			}
			lis = append(lis, ln)
			pool = append(pool, cand{c, p, ln, charted})
		}
		med := musicscore.Median(lis)
		var l []scoredCard
		for _, x := range pool {
			// Uncharted: kept only with listeners at or over the pool median
			// (and some listeners at all: an empty pool keeps nothing).
			if !x.charted && (x.ln <= 0 || x.ln < med) {
				continue
			}
			l = append(l, scoredCard{x.c, musicscore.ScoreTrending(x.p, musicscore.Freshness(x.c.Date, 7, now))})
		}
		add("np_music_week", "Out this week", "Albums and EPs out in the last 7 days: charting, or by artists many people listen to", byScore(l))
	}

	// Climbing: ΔR over the 7 kept chart days, biggest gain first; hidden
	// until every chart item has a rank from 7 days ago. No gain, no card.
	if musicscore.RanksReady(albumRanks(in.albums)) {
		L := len(in.albums)
		var l []scoredCard
		for _, it := range in.albums {
			g := musicscore.RankGain(*it.RankAgo, it.Rank, L)
			if g <= 0 {
				continue
			}
			c := chartCard(it)
			c.Note = climbNote(*it.RankAgo, it.Rank)
			l = append(l, scoredCard{c, g})
		}
		add("np_music_climbing", "Climbing", "Albums moving up the chart over the last 7 days", byScore(l))
	}

	// Top albums: the region chart in order.
	{
		var l []MusicCard
		for _, it := range in.albums {
			l = append(l, chartCard(it))
		}
		title := "Top albums"
		if in.region != "global" && in.region != "" {
			title = "Top albums in " + in.region
		}
		add("np_music_top", title, chartReason(in.region, "Chart"), l)
	}

	// Coming soon: albums and EPs due within 60 days, 0.7·[A > 0] + 0.3·P
	// (A > 0: a Library artist; P is the artist's best chart P).
	{
		var l []scoredCard
		for _, r := range in.releases {
			days, ok := daysFrom(r.ReleaseDate, now)
			kind := strings.ToLower(r.PrimaryType)
			if !ok || days >= 0 || -days > musicNewAhead || len(r.SecondaryTypes) > 0 || (kind != "album" && kind != "ep") {
				continue
			}
			id := strings.ToLower(r.MBID)
			artist := strings.ToLower(r.ArtistMBID)
			c := withLibrary(MusicCard{Key: "album:" + id, Kind: "album", MBID: id, Name: r.Name, Artist: r.Artist, ArtistMBID: artist, Date: r.ReleaseDate,
				Image: "/api/v1/music-trending/art/album/" + id, Upcoming: true, Badge: releaseBadge(kind, true), Following: in.follows[id], Genres: append([]string{}, r.Tags...)})
			_, owned := in.owned[artist]
			l = append(l, scoredCard{c, musicscore.ScoreComingTrending(owned, pcBest[artist])})
		}
		add("np_music_coming", "Coming soon", "Albums out in the next 60 days; your artists first, then the ones charting", byScore(l))
	}
	if rows == nil {
		rows = []MusicRow{}
	}
	return hero, rows
}

// musicTrendingRows reads the region charts and the listeners of this
// week's release artists (layer 1 caches), then composes the rows.
func (a *API) musicTrendingRows(ctx context.Context, r *http.Request, in musicNewInput, now time.Time) ([]MusicCard, []MusicRow) {
	region := strings.ToUpper(r.URL.Query().Get("region"))
	if region == "" || region == "GLOBAL" || !isCountry(region) {
		region = "global"
	}
	t := musicTrendingInput{region: region, owned: in.owned, follows: in.follows, ownedImages: in.images,
		albums: a.musicChartOf(ctx, region, "albums", now), songs: a.musicChartOf(ctx, region, "songs", now)}
	if in.data != nil {
		t.releases = in.data.releases
	}
	var ids []string
	for _, rel := range t.releases {
		if days, ok := daysFrom(rel.ReleaseDate, now); ok && days >= 0 && days <= musicNewWeek {
			ids = append(ids, strings.ToLower(rel.ArtistMBID))
		}
	}
	sort.Strings(ids)
	t.listeners = a.musicListenersOf(ctx, ids, now)
	return composeMusicTrending(t, now)
}
