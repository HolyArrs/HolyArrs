package music

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	musicscore "github.com/HolyArrs/holyarrs/app/backend/internal/music/score"
)

// Fresh & Trending → Music, flag music_rows_v6 (plan 22 October 2026 §2.7,
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
	// From the prebuilt New & Popular data: the metadata server's new
	// music windows, the fresh list, and the Library's own new releases.
	server  map[string][]metadata.MusicNewItem
	fresh   []metadata.MusicTrendItem
	library []MusicCard
	// genreCharts are the album charts of the household's top genres.
	genreCharts []genreChart
}

type genreChart struct {
	name  string
	items []metadata.MusicChartItem
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
		seen[c.Key] = true
		if c.Kind == "song" {
			return // a song line never hides its album
		}
		seen["fold:"+fold(c.Name, c.Artist)] = true
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
		inRow := map[string]bool{}
		for _, c := range cards {
			if len(items) >= musicNewRowMax {
				break
			}
			k := c.Key
			if c.MBID != "" {
				k = "rg:" + c.MBID
			}
			if taken(c) || inRow[k] || inRow["fold:"+fold(c.Name, c.Artist)] {
				continue
			}
			inRow[k], inRow["fold:"+fold(c.Name, c.Artist)] = true, true
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
		c.ArtistImage = heroArtistImage(c.ArtistMBID)
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

	// The wider pool of new releases: MusicBrainz, the metadata server's
	// windows, the fresh list and the Library, one card per release group.
	type newCand struct {
		c    MusicCard
		kind string
		v    float64
	}
	pool := map[string]*newCand{}
	var poolKeys []string
	put := func(id, name, artist, artistMBID, date, kind string, tags []string, explicit bool, v float64) {
		id, artistMBID, kind = strings.ToLower(id), strings.ToLower(artistMBID), strings.ToLower(kind)
		if !metadata.ValidMBID(id) || name == "" || len(date) != 10 {
			return
		}
		if kind != "ep" && kind != "single" {
			kind = "album"
		}
		if x := pool[id]; x != nil {
			x.v = max(x.v, v)
			x.c.Explicit = x.c.Explicit || explicit
			return
		}
		days, _ := daysFrom(date, now)
		c := withLibrary(MusicCard{Key: "album:" + id, Kind: "album", MBID: id, Name: name, Artist: artist, ArtistMBID: artistMBID, Date: date,
			Image: "/api/v1/music-trending/art/album/" + id, Upcoming: days < 0, Badge: releaseBadge(kind, days < 0), Following: in.follows[id], Explicit: explicit, Genres: append([]string{}, tags...)})
		if c.Genres == nil {
			c.Genres = []string{}
		}
		if p, ok := chartP[id]; ok {
			v = max(v, p)
		}
		if p, ok := chartP["fold:"+fold(name, artist)]; ok {
			v = max(v, p)
		}
		pool[id] = &newCand{c, kind, v}
		poolKeys = append(poolKeys, id)
	}
	for _, r := range in.releases {
		if len(r.SecondaryTypes) == 0 {
			put(r.MBID, r.Name, r.Artist, r.ArtistMBID, r.ReleaseDate, r.PrimaryType, r.Tags, false, 0)
		}
	}
	for _, w := range []string{"week", "recent", "upcoming"} {
		for _, it := range in.server[w] {
			v := 0.0
			if it.Popularity > 0 {
				v = max(0.05, 1-float64(it.Popularity)/200)
			}
			put(it.MBID, it.Title, it.Artist, it.ArtistMBID, it.ReleaseDate, it.Type, nil, it.Explicit, v)
		}
	}
	for _, it := range in.fresh {
		put(it.MBID, it.Title, it.Artist, it.ArtistMBID, it.ReleaseDate, "album", it.Tags, false, 0)
	}
	for _, c := range in.library {
		if x := pool[c.MBID]; x != nil {
			x.c.TitleID, x.c.PartID = c.TitleID, c.PartID
			continue
		}
		if c.MBID != "" {
			put(c.MBID, c.Name, c.Artist, c.ArtistMBID, c.Date, c.Badge, c.Genres, c.Explicit, 0)
			if x := pool[strings.ToLower(c.MBID)]; x != nil {
				x.c.TitleID, x.c.PartID = c.TitleID, c.PartID
			}
		}
	}
	pick := func(keep func(x *newCand, days int) bool) []MusicCard {
		var l []scoredCard
		for _, k := range poolKeys {
			x := pool[k]
			if days, ok := daysFrom(x.c.Date, now); ok && keep(x, days) {
				fr := musicscore.Freshness(x.c.Date, 30, now)
				if days < 0 {
					fr = 0
				}
				l = append(l, scoredCard{x.c, 0.6*x.v + 0.3*fr + 0.1*boolScore(x.c.TitleID > 0)})
			}
		}
		return byScore(l)
	}
	has := func(id string) bool {
		for _, r := range rows {
			if r.ID == id {
				return true
			}
		}
		return false
	}
	// Best new songs from the metadata server's track chart, when the
	// region chart had too few fresh songs.
	if !has("np_music_tracks") {
		var l []MusicCard
		for _, it := range in.server["tracks"] {
			if it.Track == "" {
				continue
			}
			id := strings.ToLower(it.MBID)
			c := withLibrary(MusicCard{Kind: "song", MBID: id, Name: it.Track, Track: it.Track, Note: "From " + it.Title, Artist: it.Artist, ArtistMBID: strings.ToLower(it.ArtistMBID), Date: it.ReleaseDate,
				Rank: it.Popularity, Explicit: it.Explicit, Image: "/api/v1/music-trending/art/album/" + id, Genres: []string{}})
			c.Key = "song:" + strings.ToLower(it.Artist+"|"+it.Track)
			l = append(l, c)
		}
		add("np_music_tracks", "Best new songs", "The most played new songs right now; each one asks for its album", l)
	}
	if !has("np_music_tracks") {
		var l []scoredCard
		per := map[string]bool{}
		for _, it := range in.songs {
			a := strings.ToLower(it.Artist)
			if it.Date == "" || it.Date < now.AddDate(0, 0, -musicNewAhead).Format("2006-01-02") || per[a] {
				continue
			}
			per[a] = true
			c := MusicCard{Kind: "song", Name: it.Title, Track: it.Title, Artist: it.Artist, ArtistMBID: strings.ToLower(it.ArtistMBID), Date: it.Date, Rank: it.Rank, Explicit: it.Explicit, Image: it.Cover, Genres: append([]string{}, it.Genres...)}
			c.Key = "song:" + strings.ToLower(it.Artist+"|"+it.Title)
			l = append(l, scoredCard{withLibrary(c), musicscore.ScoreFreshSong(it.P, musicscore.Freshness(it.Date, musicNewAhead, now))})
		}
		add("np_music_tracks", "Best new songs", "New songs on the chart from the last two months; each one asks for its album", byScore(l))
	}
	add("np_music_library", "New from your artists", "Albums, EPs and singles from artists in your Library, out in the last 90 days",
		pick(func(x *newCand, d int) bool { return x.c.TitleID > 0 && d >= 0 && d <= musicNewLibrary }))
	if !has("np_music_week") {
		add("np_music_week", "New this week", "Albums and EPs out in the last 7 days",
			pick(func(x *newCand, d int) bool { return x.kind != "single" && d >= 0 && d <= musicNewWeek }))
	}
	add("np_music_recent", "Recent releases", "Albums and EPs out in the last 30 days",
		pick(func(x *newCand, d int) bool { return x.kind != "single" && d >= 0 && d <= 30 }))

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

	add("np_music_short", "Singles & EPs", "New singles and EPs from the last 45 days",
		pick(func(x *newCand, d int) bool { return x.kind != "album" && d >= 0 && d <= musicNewRecent }))
	// New in <genre>: the household's top genres, one row each.
	{
		var all []MusicCard
		for _, k := range poolKeys {
			all = append(all, pool[k].c)
		}
		cards := map[string]MusicCard{}
		for i, c := range all {
			cards[fmt.Sprint(i)] = c
		}
		for _, g := range musicNewTopGenres(cards, 3) {
			g := g
			add("np_music_genre_"+musicSlug(g), "New in "+g, "New releases tagged "+g, pick(func(x *newCand, d int) bool {
				if d < 0 || d > musicNewRecent {
					return false
				}
				for _, t := range x.c.Genres {
					if strings.EqualFold(t, g) {
						return true
					}
				}
				return false
			}))
		}
		// Too few tagged releases: the genre's own chart, newest first.
		for _, gc := range in.genreCharts {
			id := "np_music_genre_" + musicSlug(gc.name)
			if has(id) {
				continue
			}
			var l []scoredCard
			for _, it := range gc.items {
				c := chartCard(it)
				fr := musicscore.Freshness(it.Date, 365, now)
				l = append(l, scoredCard{c, 0.5*it.P + 0.5*fr})
			}
			add(id, "New in "+gc.name, gc.name+" albums on the chart, the newest first", byScore(l))
		}
		for i := range rows {
			if strings.HasPrefix(rows[i].ID, "np_music_genre_") {
				rows[i].Type = "np_music_genre"
			}
		}
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
		for _, k := range poolKeys {
			x := pool[k]
			if days, ok := daysFrom(x.c.Date, now); ok && days < 0 && -days <= musicNewAhead && x.kind != "single" {
				_, owned := in.owned[x.c.ArtistMBID]
				l = append(l, scoredCard{x.c, musicscore.ScoreComingTrending(owned, pcBest[x.c.ArtistMBID])})
			}
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
func (h *Handlers) musicTrendingRows(ctx context.Context, r *http.Request, in musicNewInput, now time.Time) ([]MusicCard, []MusicRow) {
	region := strings.ToUpper(r.URL.Query().Get("region"))
	if region == "" || region == "GLOBAL" || !isCountry(region) {
		region = "global"
	}
	t := musicTrendingInput{region: region, owned: in.owned, follows: in.follows, ownedImages: in.images,
		albums: h.musicChartOf(ctx, region, "albums", now), songs: h.musicChartOf(ctx, region, "songs", now)}
	t.library = in.library
	if in.data != nil {
		t.releases, t.server, t.fresh = in.data.releases, in.data.server, in.data.fresh.Items
	}
	var ids []string
	for _, rel := range t.releases {
		if days, ok := daysFrom(rel.ReleaseDate, now); ok && days >= 0 && days <= musicNewWeek {
			ids = append(ids, strings.ToLower(rel.ArtistMBID))
		}
	}
	for i, g := range h.musicGenreTiles(ctx, region, now) {
		if i >= 3 {
			break
		}
		t.genreCharts = append(t.genreCharts, genreChart{g.Name, h.musicGenreChart(ctx, region, "albums", g.Slug, now)})
	}
	sort.Strings(ids)
	t.listeners = h.musicListenersOf(ctx, ids, now)
	return composeMusicTrending(t, now)
}

func boolScore(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
