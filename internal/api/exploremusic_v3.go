package api

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/music/rowtpl"
	musicscore "github.com/CoderLuii/holyarrs/internal/music/score"
)

// Explore → Music, flag music_rows_v3 (plan 2026-10-22, Phase 3): the v2
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
	// no genres or albums) only minutes (cache audit 2026-10-23).
	musicEmptyTTL = 10 * time.Minute
	musicSeedsTop  = 10
	musicSimilarS  = 0.3
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
func (a *API) musicBatch(ctx context.Context, prefix string, ids []string, chunk int, now time.Time, fetch func(context.Context, []string) (map[string]any, error)) map[string]any {
	out := map[string]any{}
	var miss []string
	seen := map[string]bool{}
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		if v, ok := a.musicv2.cand.Get(prefix+id, now); ok {
			if v != nil {
				out[id] = v
			}
			continue
		}
		miss = append(miss, id)
	}
	if len(miss) > 0 {
		disk := a.musicDiskRead(ctx, prefix, miss, now)
		rest := miss[:0:0]
		for _, id := range miss {
			if v, ok := disk[id]; ok {
				out[id] = v
				a.musicv2.cand.Put(prefix+id, v, musicArtistTTL, now)
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
					a.musicv2.cand.Put(prefix+id, v, musicEmptyTTL, now)
				} else if ok && v != nil {
					out[id] = v
					fetched[id] = v
					a.musicv2.cand.Put(prefix+id, v, musicArtistTTL, now)
				} else {
					a.musicv2.cand.Put(prefix+id, nil, musicMissTTL, now)
				}
			}
		}(part)
	}
	wg.Wait()
	a.musicDiskWrite(ctx, prefix, fetched, now)
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

func (a *API) musicListenersOf(ctx context.Context, ids []string, now time.Time) map[string]int64 {
	out := map[string]int64{}
	if a.s.MusicListeners == nil {
		return out
	}
	for id, v := range a.musicBatch(ctx, "lb:l:", ids, 60, now, func(ctx context.Context, ids []string) (map[string]any, error) {
		m, err := a.s.MusicListeners(ctx, ids)
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

func (a *API) musicArtistDataOf(ctx context.Context, ids []string, now time.Time) map[string]*metadata.MusicArtistData {
	out := map[string]*metadata.MusicArtistData{}
	if a.s.MusicArtistData == nil {
		return out
	}
	for id, v := range a.musicBatch(ctx, "lb:d:", ids, 30, now, func(ctx context.Context, ids []string) (map[string]any, error) {
		l, err := a.s.MusicArtistData(ctx, ids)
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

func (a *API) musicSimilarOf(ctx context.Context, seeds []string, now time.Time) map[string][]metadata.MusicSimilarScore {
	out := map[string][]metadata.MusicSimilarScore{}
	if a.s.MusicSimilarSet == nil {
		return out
	}
	for id, v := range a.musicBatch(ctx, "lb:s:", seeds, 12, now, func(ctx context.Context, ids []string) (map[string]any, error) {
		m, err := a.s.MusicSimilarSet(ctx, ids)
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

func (a *API) musicGenreChartOf(ctx context.Context, region, slug string, now time.Time) []metadata.MusicChartItem {
	if a.s.MusicCharts == nil {
		return nil
	}
	key := "chart:" + region + ":albums:" + slug
	if v, ok := a.musicv2.cand.Get(key, now); ok {
		return v.([]metadata.MusicChartItem)
	}
	c, err := a.s.MusicCharts(ctx, region, "albums", slug)
	if err != nil {
		markMusicMiss(ctx)
		return nil
	}
	a.musicv2.cand.Put(key, c.Items, spread(musicChartTTL, 0.25), now)
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

// buildMusicV3 builds layer 2 with the Phase 3 scores.
func (a *API) buildMusicV3(ctx context.Context, member int64, region string, lib *musicLib, acts *musicActs, now time.Time) *musicV2Order {
	ord := &musicV2Order{}
	date := musicscore.Day(now)
	today := dayAt(now, 0)

	// A per library artist (and by mbid), and whether the member has own acts.
	aff := map[int64]float64{}
	affMBID := map[string]float64{}
	ownFollows, haveAny := 0, false
	var all []musicscore.Acts
	for _, ar := range lib.artists {
		x := musicscore.Acts{HaveAny: ar.hadAny, AlbumsHad: ar.had}
		for u := range acts.follows[ar.id] {
			if u == member {
				x.Follows = true
				ownFollows++
			} else {
				x.OtherFollows = true
			}
		}
		for u := range acts.asked[ar.mbid] {
			if u == member {
				x.Asked = true
			} else {
				x.OtherAsked = true
			}
		}
		all = append(all, x)
		aff[ar.id] = musicscore.Affinity(x)
		if ar.mbid != "" {
			affMBID[ar.mbid] = aff[ar.id]
		}
		haveAny = haveAny || ar.hadAny
	}
	own := musicscore.OwnOnly(all)
	ord.Cold = ownFollows < 3 && !haveAny
	artistOf := map[int64]musicLibArtist{}
	var libIDs []string
	hadMBID := map[string]bool{}
	for _, ar := range lib.artists {
		artistOf[ar.id] = ar
		if ar.mbid != "" {
			libIDs = append(libIDs, ar.mbid)
			if ar.hadAny {
				hadMBID[ar.mbid] = true
			}
		}
	}
	hadAlbum := map[string]bool{}
	for _, al := range lib.albums {
		if al.mbid != "" && lib.hadParts[al.partID] {
			hadAlbum[al.mbid] = true
		}
	}
	// World-list shelves (music_lists_v1); nil when off.
	shelves := a.musicShelvesOf(ctx, region, now)

	// Server data (layer 1): genre votes and top albums of library artists.
	data := a.musicArtistDataOf(ctx, libIDs, now)
	shares := map[string]map[string]float64{}
	primary := map[string]string{} // artist mbid → top-vote genre key
	sharesOf := func(d *metadata.MusicArtistData) (map[string]float64, string) {
		votes := map[string]int{}
		top, topN := "", 0
		for _, g := range d.Genres {
			votes[g.Name] += g.Count
			if g.Count > topN {
				top, topN = musicscore.GenreKey(g.Name), g.Count
			}
		}
		return musicscore.Shares(votes), top
	}
	for id, d := range data {
		shares[id], primary[id] = sharesOf(d)
	}
	G := musicscore.GenreAffinity(affMBID, shares)
	gOfArtist := func(id string) float64 {
		best := 0.0
		for g := range shares[id] {
			best = max(best, G[g])
		}
		return best
	}
	albumListeners := map[string]int64{}
	for _, d := range data {
		for _, al := range d.Albums {
			albumListeners[al.MBID] = max(albumListeners[al.MBID], al.Listeners)
		}
	}

	// Charts: P per album, best chart P per artist.
	albums := a.musicChartOf(ctx, region, "albums", now)
	songs := a.musicChartOf(ctx, region, "songs", now)
	chartP := map[string]float64{}
	pcBest := map[string]float64{}
	for _, l := range [][]metadata.MusicChartItem{albums, songs} {
		for _, it := range l {
			if m := strings.ToLower(it.MBID); m != "" {
				chartP[m] = max(chartP[m], it.P)
			}
			if m := strings.ToLower(it.ArtistMBID); m != "" {
				pcBest[m] = max(pcBest[m], it.P)
			}
		}
	}
	// albumP is P for album candidates of one row: charted P, else
	// min(0.3, 0.3·Lnorm) over the row's pool.
	albumP := func(ids []string) map[string]float64 {
		var top int64
		for _, id := range ids {
			top = max(top, albumListeners[id])
		}
		out := map[string]float64{}
		for _, id := range ids {
			if p, ok := chartP[id]; ok {
				out[id] = p
			} else {
				out[id] = musicscore.Uncharted(musicscore.Lnorm(albumListeners[id], top))
			}
		}
		return out
	}

	albumCard := func(al musicLibAlbum) MusicCard {
		ar := artistOf[al.titleID]
		c := MusicCard{Kind: "album", MBID: al.mbid, Name: al.name, Artist: ar.name, ArtistMBID: ar.mbid, TitleID: ar.id, PartID: al.partID, Date: al.date, Genres: []string{}}
		c.Key = "album:" + al.mbid
		if al.mbid == "" {
			c.Key = "part:" + strconv.FormatInt(al.partID, 10)
		} else {
			c.Image = "/api/v1/music-trending/art/album/" + al.mbid
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
		if i, ok := lib.byMBID[c.ArtistMBID]; ok && c.ArtistMBID != "" {
			c.TitleID = lib.artists[i].id
		}
		return c
	}
	scored := func(c MusicCard, score, A, gItem float64, genre string) musicscore.Scored {
		return musicscore.Scored{Item: musicscore.Item{Key: c.Key, Album: c.MBID, FoldKey: fullAlbumKey(c.Name) + "|" + fullAlbumKey(c.Artist), Artist: strings.ToLower(c.ArtistMBID + "|" + c.Artist), Score: score}, A: A, GItem: gItem, Genre: genre}
	}
	type built struct {
		row   musicscore.Row
		cards map[string]MusicCard
		meta  MusicRow
	}
	var rows []built
	// addRow ranks, diversifies, and (personal rows) fills the exploration slot.
	addRow := func(meta MusicRow, row musicscore.Row, cards []MusicCard, list []musicscore.Scored, rank, personal bool) {
		b := built{row: row, cards: map[string]MusicCard{}, meta: meta}
		b.row.ID = meta.ID
		if rank {
			musicscore.Rank(list, member, date)
			list = musicscore.Diversify(list)
			if personal {
				list = musicscore.Explore(list)
			}
		}
		b.row.Items = musicscore.Items(list)
		for _, c := range cards {
			b.cards[c.Key] = c
		}
		rows = append(rows, b)
	}
	studio := func(al musicLibAlbum) bool {
		return !al.secondary && (al.kind == "album" || al.kind == "ep" || al.kind == "single")
	}

	// Rows 1 and 2: new by A > 0 artists, −30..+7 days, 0.6·A + 0.4·F14.
	from, to := dayAt(now, -30), dayAt(now, 7)
	var fresh []MusicCard
	var freshS []musicscore.Scored
	for _, al := range lib.albums {
		if aff[al.titleID] > 0 && studio(al) && al.date >= from && al.date <= to {
			c := albumCard(al)
			c.Upcoming = c.Date > today
			fresh = append(fresh, c)
			ar := artistOf[al.titleID]
			freshS = append(freshS, scored(c, musicscore.ScoreNewFrom(aff[al.titleID], musicscore.Freshness(c.Date, 14, now)), aff[al.titleID], gOfArtist(ar.mbid), primary[ar.mbid]))
		}
	}
	musicscore.Rank(freshS, member, date)
	freshBy := map[string]MusicCard{}
	for _, c := range fresh {
		freshBy[c.Key] = c
	}
	seen := map[string]bool{}
	var heroes []MusicCard
	if !ord.Cold {
		for _, s := range freshS {
			if c := freshBy[s.Key]; !c.Upcoming && len(heroes) < 6 {
				c.ArtistImage = artistOf[c.TitleID].image
				heroes = append(heroes, c)
			}
		}
	}
	// The Spotlight row (Apple "New" style): your artists' new releases
	// first, then chart new releases, climbers and the top, 8 at most,
	// one card per album and per artist.
	const heroMax = 8
	kindOf := map[string]string{}
	for _, al := range lib.albums {
		if al.mbid != "" {
			kindOf["album:"+al.mbid] = al.kind
		}
	}
	for i := range heroes {
		heroes[i].Kicker = "New " + map[string]string{"single": "single", "ep": "EP"}[kindOf[heroes[i].Key]]
		if heroes[i].Kicker == "New " {
			heroes[i].Kicker = "New album"
		}
	}
	if shelves != nil {
		// Biggest right now leads the Spotlight: the region's top charted
		// albums, then the member's own new releases.
		lead := listHero(shelves, func(id string) bool { return hadAlbum[id] }, 3)
		taken := map[string]bool{}
		for _, c := range lead {
			taken[strings.ToLower(c.Artist)] = true
		}
		for _, c := range heroes {
			if !taken[strings.ToLower(c.Artist)] {
				lead = append(lead, c)
			}
		}
		heroes = lead
	}
	if len(heroes) > 5 {
		heroes = heroes[:5]
	}
	inHero := map[string]bool{}
	byArtist := map[string]bool{}
	for _, c := range heroes {
		inHero[c.Key] = true
		inHero[fullAlbumKey(c.Name)+"|"+fullAlbumKey(c.Artist)] = true
		byArtist[strings.ToLower(c.Artist)] = true
	}
	addHero := func(it metadata.MusicChartItem, kicker string) {
		c := chartCard(it)
		fold := fullAlbumKey(c.Name) + "|" + fullAlbumKey(c.Artist)
		if len(heroes) >= heroMax || c.MBID == "" || inHero[c.Key] || inHero[fold] || byArtist[strings.ToLower(c.Artist)] {
			return
		}
		c.ArtistImage = "/api/v1/music-trending/art/artist/" + c.ArtistMBID
		c.Kicker = kicker
		inHero[c.Key], inHero[fold], byArtist[strings.ToLower(c.Artist)] = true, true, true
		heroes = append(heroes, c)
	}
	for _, it := range albums {
		if it.Date >= from && it.Date <= today {
			addHero(it, "New album")
		}
	}
	for _, it := range albums {
		if it.RankAgo != nil && (*it.RankAgo == 0 || *it.RankAgo-it.Rank >= 5) {
			addHero(it, "Climbing")
		}
	}
	for _, it := range albums {
		addHero(it, "Top album")
	}
	// "New" only within 60 days of release; older cards say why they show.
	honestLabels(heroes, now)
	if len(heroes) > 0 {
		ord.Hero, ord.Alts = &heroes[0], heroes[1:]
		seen[heroes[0].Key] = true
		if heroes[0].MBID != "" {
			seen["rg:"+heroes[0].MBID] = true
		}
	}
	if !ord.Cold {
		reason := "Out in the last 30 days or this week, from artists you have or follow"
		if !own {
			reason = "Followed in your household: out in the last 30 days or this week"
		}
		addRow(MusicRow{ID: "new_from", Type: "new_from", Title: "New from your artists", Reason: reason, Shape: "square"},
			musicscore.Row{Anchor: true, Prior: musicscore.PriorPersonal, ArtistRow: true}, fresh, append([]musicscore.Scored{}, freshS...), true, true)
	}

	// Row 4: genre tiles, top 12 by G then region order.
	tiles := a.musicGenreTiles(ctx, region, now)
	{
		type tg struct {
			g metadata.MusicGenre
			v float64
		}
		var list []tg
		for _, g := range tiles {
			list = append(list, tg{g, musicscore.ItemGenre(G, []string{g.Name, g.Slug})})
		}
		sort.SliceStable(list, func(i, j int) bool { return list[i].v > list[j].v })
		var cards []MusicCard
		var sc []musicscore.Scored
		for _, x := range list {
			if len(cards) >= 12 {
				break
			}
			hue := x.g.Hue
			c := MusicCard{Key: "genre:" + x.g.Slug, Kind: "genre", MBID: x.g.Slug, Name: x.g.Name, Hue: &hue, Image: musicGenreArt(x.g), Images: musicGenreArts(x.g), Genres: []string{x.g.Slug}}
			cards = append(cards, c)
			sc = append(sc, musicscore.Scored{Item: musicscore.Item{Key: c.Key, Score: 1}})
		}
		meta := MusicRow{ID: "genres", Type: "genres", Title: "Genres", Reason: "Your genres first, then the ones charting in your region", Shape: "tile"}
		if shelves != nil {
			meta.Title, meta.Shape = "Browse", "grid"
			a.browseCovers(ctx, region, cards, now)
		}
		// Row templates: Explore's Browse row is 4 genres, the season, 2 moods
		// and a decade from the Browse page, with their covers.
		if a.musicTplOn() {
			if b := a.musicBundle(ctx, now); b != nil {
				cards, sc = nil, nil
				for _, t := range rowtpl.ExploreTiles(a.tplVisible(ctx, b, region, nil, now)) {
					hue := t.Hue
					c := MusicCard{Key: t.Key, Kind: "genre", MBID: strings.TrimPrefix(t.Href, "/music/genre/"), Name: t.Name, Hue: &hue, Href: t.Href, Genres: []string{}}
					if len(t.Covers) > 0 {
						c.Cover = t.Covers[0]
					}
					cards = append(cards, c)
					sc = append(sc, musicscore.Scored{Item: musicscore.Item{Key: c.Key, Score: 1}})
				}
				meta.Title, meta.Shape, meta.Reason = "Browse", "grid", "Genres, the season, moods and decades"
			}
		}
		addRow(meta,
			musicscore.Row{Anchor: true, Prior: musicscore.PriorMixed, ArtistRow: true}, cards, sc, false, false)
	}

	// Row 5: songs out in the last 14 days, one per artist, 0.7·P + 0.3·F14.
	{
		var cards []MusicCard
		var sc []musicscore.Scored
		per := map[string]bool{}
		for _, it := range songs {
			if it.Date == "" || it.Date < dayAt(now, -14) || per[strings.ToLower(it.Artist)] {
				continue
			}
			per[strings.ToLower(it.Artist)] = true
			c := MusicCard{Kind: "song", Name: it.Title, Track: it.Title, Artist: it.Artist, ArtistMBID: strings.ToLower(it.ArtistMBID), Date: it.Date, Rank: it.Rank, Explicit: it.Explicit, Image: it.Cover, Genres: append([]string{}, it.Genres...)}
			c.Key = "song:" + strings.ToLower(it.Artist+"|"+it.Title)
			cards = append(cards, c)
			genre := ""
			if len(it.Genres) > 0 {
				genre = musicscore.GenreKey(it.Genres[0])
			}
			sc = append(sc, scored(c, musicscore.ScoreFreshSong(it.P, musicscore.Freshness(it.Date, 14, now)), affMBID[c.ArtistMBID], musicscore.ItemGenre(G, it.Genres), genre))
		}
		addRow(MusicRow{ID: "fresh_songs", Type: "fresh_songs", Title: "Fresh songs", Reason: "New songs on the chart; each one asks for its album", Shape: "song"},
			musicscore.Row{Prior: musicscore.PriorChart}, cards, sc, true, false)
	}

	// latest: the newest released studio album of each artist.
	latest := map[int64]string{}
	released := map[int64]int{}
	for _, al := range lib.albums {
		if al.kind == "album" && !al.secondary && al.date != "" && al.date <= today {
			released[al.titleID]++
			if al.date > latest[al.titleID] {
				latest[al.titleID] = al.date
			}
		}
	}

	// Row 6: missing studio albums of had artists, 0.5·A + 0.3·P + 0.2·latest.
	if !ord.Cold {
		var list []musicLibAlbum
		var ids []string
		for _, al := range lib.albums {
			ar := artistOf[al.titleID]
			if !ar.hadAny || al.kind != "album" || al.secondary || al.date == "" || al.date > today || lib.hadParts[al.partID] || acts.albumState[al.mbid] != "" {
				continue
			}
			list = append(list, al)
			ids = append(ids, al.mbid)
		}
		P := albumP(ids)
		var cards []MusicCard
		var sc []musicscore.Scored
		for _, al := range list {
			c := albumCard(al)
			ar := artistOf[al.titleID]
			cards = append(cards, c)
			sc = append(sc, scored(c, musicscore.ScoreMissing(aff[al.titleID], P[al.mbid], musicscore.Bool(latest[al.titleID] == al.date)), aff[al.titleID], gOfArtist(ar.mbid), primary[ar.mbid]))
		}
		addRow(MusicRow{ID: "missing", Type: "missing", Title: "Missing from your artists", Reason: "Studio albums by artists you have, not in your Library or asked for yet", Shape: "square"},
			musicscore.Row{Prior: musicscore.PriorPersonal}, cards, sc, true, true)
	}

	// Row 7: the region's top albums, chart order.
	{
		var cards []MusicCard
		var sc []musicscore.Scored
		for _, it := range albums {
			c := chartCard(it)
			cards = append(cards, c)
			sc = append(sc, scored(c, it.P, 0, 0, ""))
		}
		title := "Top albums"
		if region != "global" {
			title = "Top albums in " + region
		}
		addRow(MusicRow{ID: "top_albums", Type: "top_albums", Title: title, Reason: chartReason(region, "Chart"), Shape: "square"},
			musicscore.Row{Prior: musicscore.PriorChart}, cards, sc, false, false)
	}

	// Row 8: coming within 90 days from Library artists, 0.7·A + 0.3·P.
	{
		var list []musicLibAlbum
		var ids []string
		for _, al := range lib.albums {
			if studio(al) && al.date > today && al.date <= dayAt(now, 90) {
				list = append(list, al)
				ids = append(ids, al.mbid)
			}
		}
		P := albumP(ids)
		var cards []MusicCard
		var sc []musicscore.Scored
		for _, al := range list {
			c := albumCard(al)
			c.Upcoming = true
			ar := artistOf[al.titleID]
			cards = append(cards, c)
			sc = append(sc, scored(c, musicscore.ScoreComingSoon(aff[al.titleID], P[al.mbid]), aff[al.titleID], gOfArtist(ar.mbid), primary[ar.mbid]))
		}
		addRow(MusicRow{ID: "coming_soon", Type: "coming_soon", Title: "Coming soon", Reason: "Out within 90 days, from artists in your Library", Shape: "square"},
			musicscore.Row{Prior: musicscore.PriorMixed}, cards, sc, true, false)
	}

	if !ord.Cold {
		a.personalMusicRows(ctx, member, region, date, lib, acts, now, personalIn{
			aff: aff, affMBID: affMBID, artistOf: artistOf, hadMBID: hadMBID, hadAlbum: hadAlbum, G: G, shares: shares, primary: primary,
			data: data, chartP: chartP, pcBest: pcBest, explicit: chartExplicit(albums), albumListeners: albumListeners, tiles: tiles, latest: latest, released: released,
			albumCard: albumCard, chartCard: chartCard, scored: scored, sharesOf: sharesOf,
			add: func(meta MusicRow, row musicscore.Row, cards []MusicCard, list []musicscore.Scored, rank, personal bool) {
				addRow(meta, row, cards, list, rank, personal)
			},
		})
	}

	// The row template engine replaces the list rows when it has its
	// catalog (kill switch music_rows_tpl_off keeps them).
	tplDone := false
	if a.musicTplOn() {
		if b := a.musicBundle(ctx, now); b != nil {
			for _, cd := range a.tplExploreCands(ctx, member, region, lib, acts, b, shelves, now) {
				meta, row, cards, sc := tplV7Row(b, cd)
				addRow(meta, row, cards, sc, false, false)
			}
			tplDone = true
		}
	}
	if shelves != nil && !tplDone {
		for _, d := range listRows(shelves) {
			keep := func(c MusicCard) bool {
				return d.Chart || c.Kind != "album" || !hadAlbum[c.MBID] && acts.albumState[c.MBID] == ""
			}
			cards, sc := listCandidates(d, a.listBlock(ctx, shelves, d), keep)
			if len(sc) < d.Min || distinctArtists(sc) < 4 {
				continue
			}
			addRow(listRowMeta(d), musicscore.Row{Prior: listPrior, Min: d.Min, Family: "lists:" + d.ID}, cards, sc, false, false)
		}
	}

	// Order (anchors first, then R), fill with one seen set, gate.
	byID := map[string]built{}
	var raw []musicscore.Row
	for _, b := range rows {
		byID[b.meta.ID] = b
		raw = append(raw, b.row)
	}
	if a.s.MusicRowsV7 {
		// v7 keeps the raw rows: the day's plan orders and fills them.
		c := &musicV7Cand{metas: map[string]MusicRow{}, cards: map[string]map[string]MusicCard{}, seen: seen}
		for _, b := range rows {
			r := b.row
			r.Family, r.Disc = musicV7Family(b.meta)
			c.rows = append(c.rows, r)
			c.metas[r.ID], c.cards[r.ID] = b.meta, b.cards
		}
		ord.v7 = c
		ord.Rows = []MusicRow{}
		if a.s.MusicTrending != nil {
			if l, err := a.cachedMusicTrend(ctx, "album", "global"); err == nil {
				ord.Regions = l.Regions
			}
		}
		return ord
	}
	for _, r := range musicscore.Dedup(musicscore.Order(raw, member, date), seen) {
		b := byID[r.ID]
		m := b.meta
		for _, it := range r.Items {
			m.Items = append(m.Items, b.cards[it.Key])
		}
		ord.Rows = append(ord.Rows, m)
	}
	if ord.Rows == nil {
		ord.Rows = []MusicRow{}
	}
	if a.s.MusicTrending != nil {
		if l, err := a.cachedMusicTrend(ctx, "album", "global"); err == nil {
			ord.Regions = l.Regions
		}
	}
	return ord
}

// personalIn is what the personal rows (9–12, 14) read from the main build.
type personalIn struct {
	aff            map[int64]float64
	affMBID        map[string]float64
	artistOf       map[int64]musicLibArtist
	hadMBID        map[string]bool
	hadAlbum       map[string]bool
	G              map[string]float64
	shares         map[string]map[string]float64
	primary        map[string]string
	data           map[string]*metadata.MusicArtistData
	chartP         map[string]float64
	pcBest         map[string]float64
	albumListeners map[string]int64
	tiles          []metadata.MusicGenre
	latest         map[int64]string
	released       map[int64]int
	albumCard      func(musicLibAlbum) MusicCard
	chartCard      func(metadata.MusicChartItem) MusicCard
	scored         func(c MusicCard, score, A, gItem float64, genre string) musicscore.Scored
	sharesOf       func(*metadata.MusicArtistData) (map[string]float64, string)
	add            func(meta MusicRow, row musicscore.Row, cards []MusicCard, list []musicscore.Scored, rank, personal bool)
	// explicit marks charted albums with explicit lyrics (v7 families).
	explicit map[string]bool
}

// nearTasteOK reports whether a similar artist may be suggested for a
// seed: not the seed, not in the Library (had or followed), not linked by a
// MusicBrainz relation to a Library artist (members, own groups, charity
// and collective credits), and not a collective credit by name.
func nearTasteOK(id, name, seed string, lib map[string]int, related map[string]bool) bool {
	if id == "" || id == seed || related[id] {
		return false
	}
	if _, ok := lib[id]; ok {
		return false
	}
	return !collectiveName.MatchString(name)
}

// collectiveName matches charity and various-artists credits.
var collectiveName = regexp.MustCompile(`(?i)^(various artists|artists (stand up|for|against|united)\b)|\b(all[- ]stars|aid)$`)

// musicGuestTypes are the release group kinds a guest spot can be.
var musicGuestTypes = map[string]bool{"album": true, "ep": true, "single": true}

func (a *API) personalMusicRows(ctx context.Context, member int64, region, date string, lib *musicLib, acts *musicActs, now time.Time, in personalIn) {
	today := dayAt(now, 0)
	asked := func(mbid string) bool { return mbid != "" && acts.albumState[mbid] != "" }

	// Seeds: the top-10 A artists (ties by the day's hash).
	type seed struct {
		ar musicLibArtist
		a  float64
	}
	var seeds []seed
	for _, ar := range lib.artists {
		if ar.mbid != "" && in.aff[ar.id] > 0 {
			seeds = append(seeds, seed{ar, in.aff[ar.id]})
		}
	}
	sort.SliceStable(seeds, func(i, j int) bool {
		if seeds[i].a != seeds[j].a {
			return seeds[i].a > seeds[j].a
		}
		return musicscore.Tie(member, date, seeds[i].ar.mbid) < musicscore.Tie(member, date, seeds[j].ar.mbid)
	})
	seeds = seeds[:min(musicSeedsTop, len(seeds))]
	var seedIDs []string
	for _, s := range seeds {
		seedIDs = append(seedIDs, s.ar.mbid)
	}
	sim := a.musicSimilarOf(ctx, seedIDs, now)
	// MusicBrainz relations (members, the artist's own groups, side
	// projects, charity and collective credits) are not taste: they only
	// rule artists out. S comes from ListenBrainz similarity alone, and
	// never names an artist already in the Library (had or followed).
	var libIDs []string
	for _, ar := range lib.artists {
		if ar.mbid != "" {
			libIDs = append(libIDs, ar.mbid)
		}
	}
	related := map[string]bool{}
	for _, l := range a.linksOf(libIDs) {
		for _, x := range l {
			related[strings.ToLower(x.MBID)] = true
		}
	}
	// S per seed and artist: ListenBrainz (normalised).
	S := map[string]map[string]float64{}
	names := map[string]string{}
	for _, s := range seeds {
		m := map[string]float64{}
		for _, x := range sim[s.ar.mbid] {
			id := strings.ToLower(x.MBID)
			if !nearTasteOK(id, x.Name, s.ar.mbid, lib.byMBID, related) {
				continue
			}
			m[id] = musicscore.Similarity(x.S, false)
			names[id] = x.Name
		}
		S[s.ar.mbid] = m
	}

	// Similar artists not had: their data (top album, genres) and listeners.
	var pool []string
	inPool := map[string]bool{}
	for _, s := range seeds {
		for id := range S[s.ar.mbid] {
			if !in.hadMBID[id] && !inPool[id] {
				inPool[id] = true
				pool = append(pool, id)
			}
		}
	}
	sort.Strings(pool)
	listeners := a.musicListenersOf(ctx, pool, now)
	// Artist data for the similar artists of the Since-you-have seeds only.
	yesterday := map[string]bool{}
	for _, id := range a.musicv2.seedLog.get(member, dayBefore(date)) {
		yesterday[id] = true
	}
	var since []seed
	for _, s := range seeds {
		if !yesterday[s.ar.mbid] && len(since) < 2 {
			since = append(since, s)
		}
	}
	var sinceIDs, want []string
	for _, s := range since {
		sinceIDs = append(sinceIDs, s.ar.mbid)
		var ids []string
		for id, v := range S[s.ar.mbid] {
			if v >= musicSimilarS && !in.hadMBID[id] {
				ids = append(ids, id)
			}
		}
		sort.Slice(ids, func(i, j int) bool {
			if S[s.ar.mbid][ids[i]] != S[s.ar.mbid][ids[j]] {
				return S[s.ar.mbid][ids[i]] > S[s.ar.mbid][ids[j]]
			}
			return ids[i] < ids[j]
		})
		want = append(want, ids[:min(15, len(ids))]...)
	}
	a.musicv2.seedLog.put(member, date, sinceIDs)
	simData := a.musicArtistDataOf(ctx, want, now)
	topAlbum := func(id string) *metadata.MusicTopAlbum {
		d := simData[id]
		if d == nil {
			return nil
		}
		for i, al := range d.Albums {
			if al.Type == "album" && !nonStudioTitle(al.Title) && len(al.Artists) > 0 && al.Artists[0] == id && !in.hadAlbum[al.MBID] {
				return &d.Albums[i]
			}
		}
		return nil
	}
	pOf := func(ids []string) map[string]float64 {
		var top int64
		for _, id := range ids {
			top = max(top, in.albumListeners[id])
		}
		out := map[string]float64{}
		for _, id := range ids {
			if p, ok := in.chartP[id]; ok {
				out[id] = p
			} else {
				out[id] = musicscore.Uncharted(musicscore.Lnorm(in.albumListeners[id], top))
			}
		}
		return out
	}
	topCard := func(id string, al *metadata.MusicTopAlbum) MusicCard {
		c := MusicCard{Kind: "album", MBID: al.MBID, Name: al.Title, Artist: names[id], ArtistMBID: id, Date: al.Date, Image: "/api/v1/music-trending/art/album/" + al.MBID, Genres: []string{}}
		if len(al.ArtistNames) > 0 && al.ArtistNames[0] != "" {
			c.Artist = al.ArtistNames[0]
		}
		c.Key = "album:" + al.MBID
		if i, ok := lib.byMBID[id]; ok {
			c.TitleID = lib.artists[i].id
		}
		return c
	}

	// Row 9: Since you have {X}, 0.5·S + 0.3·P + 0.2·G_item (v7: the
	// Because you have rows take its place).
	if a.s.MusicRowsV7 {
		since = nil
	}
	for _, s := range since {
		type cand struct {
			id string
			al *metadata.MusicTopAlbum
		}
		var list []cand
		var ids []string
		for id, v := range S[s.ar.mbid] {
			if v < musicSimilarS || in.hadMBID[id] {
				continue
			}
			if al := topAlbum(id); al != nil && !asked(al.MBID) {
				list = append(list, cand{id, al})
				ids = append(ids, al.MBID)
			}
		}
		for _, c := range list {
			in.albumListeners[c.al.MBID] = max(in.albumListeners[c.al.MBID], c.al.Listeners)
		}
		P := pOf(ids)
		var cards []MusicCard
		var sc []musicscore.Scored
		for _, x := range list {
			c := topCard(x.id, x.al)
			sh, top := map[string]float64{}, ""
			if d := simData[x.id]; d != nil {
				sh, top = in.sharesOf(d)
			}
			g := 0.0
			for k := range sh {
				g = max(g, in.G[k])
			}
			cards = append(cards, c)
			sc = append(sc, in.scored(c, musicscore.ScoreSinceYouHave(S[s.ar.mbid][x.id], P[x.al.MBID], g), in.affMBID[x.id], g, top))
		}
		in.add(MusicRow{ID: "since:" + s.ar.mbid, Type: "since_have", Title: "Since you have " + s.ar.name, Reason: "Artists close to " + s.ar.name + ": their best-known album", Shape: "square"},
			musicscore.Row{Prior: musicscore.PriorPersonal, Seed: s.ar.mbid}, cards, sc, true, true)
	}

	// Row 10: Finish the set, artists with at least half their studio
	// albums had; fewest missing first, then A.
	{
		type fin struct {
			ar      musicLibArtist
			missing int
			al      musicLibAlbum
		}
		missingOf := map[int64][]musicLibAlbum{}
		for _, al := range lib.albums {
			if al.kind == "album" && !al.secondary && al.date != "" && al.date <= today && !lib.hadParts[al.partID] && !asked(al.mbid) {
				missingOf[al.titleID] = append(missingOf[al.titleID], al)
			}
		}
		var list []fin
		for _, ar := range lib.artists {
			n := in.released[ar.id]
			miss := missingOf[ar.id]
			had := 0
			for _, al := range lib.albums {
				if al.titleID == ar.id && al.kind == "album" && !al.secondary && al.date != "" && al.date <= today && lib.hadParts[al.partID] {
					had++
				}
			}
			if n == 0 || len(miss) == 0 || had*2 < n {
				continue
			}
			sort.SliceStable(miss, func(i, j int) bool { return miss[i].date > miss[j].date })
			list = append(list, fin{ar, len(miss), miss[0]})
		}
		sort.SliceStable(list, func(i, j int) bool {
			if list[i].missing != list[j].missing {
				return list[i].missing < list[j].missing
			}
			if in.aff[list[i].ar.id] != in.aff[list[j].ar.id] {
				return in.aff[list[i].ar.id] > in.aff[list[j].ar.id]
			}
			return musicscore.Tie(member, date, list[i].ar.mbid) < musicscore.Tie(member, date, list[j].ar.mbid)
		})
		var cards []MusicCard
		var sc []musicscore.Scored
		for i, f := range list {
			c := in.albumCard(f.al)
			c.Note = fmt.Sprintf("%s · %d left", f.ar.name, f.missing)
			cards = append(cards, c)
			sc = append(sc, in.scored(c, 1-float64(i)/float64(len(list)), in.aff[f.ar.id], 0, ""))
		}
		in.add(MusicRow{ID: "finish_set", Type: "finish_set", Title: "Finish the set", Reason: "Artists you have most of: the studio albums still missing", Shape: "square"},
			musicscore.Row{Prior: musicscore.PriorPersonal}, cards, sc, false, false)
	}

	// Row 11: Artists near your taste, norm(Σ A_i·S_i) + 0.2·P, capped 1.
	{
		sum := map[string]float64{}
		for _, s := range seeds {
			for id, v := range S[s.ar.mbid] {
				if !in.hadMBID[id] {
					sum[id] += s.a * v
				}
			}
		}
		ids := make([]string, 0, len(sum))
		for id := range sum {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		vals := make([]float64, len(ids))
		var top int64
		for i, id := range ids {
			vals[i] = sum[id]
			top = max(top, listeners[id])
		}
		normed := musicscore.Norm(vals)
		var cards []MusicCard
		var sc []musicscore.Scored
		for i, id := range ids {
			_, charted := in.pcBest[id]
			p := musicscore.ArtistPopularity(in.pcBest[id], charted, musicscore.Lnorm(listeners[id], top))
			c := MusicCard{Key: "artist:" + id, Kind: "artist", MBID: id, Name: names[id], Artist: names[id], ArtistMBID: id, Image: "/api/v1/music-trending/art/artist/" + id, Genres: []string{}}
			if c.Name == "" {
				continue
			}
			if j, ok := lib.byMBID[id]; ok {
				c.TitleID = lib.artists[j].id
			}
			cards = append(cards, c)
			sc = append(sc, in.scored(c, musicscore.ScoreNearTaste(normed[i], p), in.affMBID[id], 0, ""))
		}
		in.add(MusicRow{ID: "near_taste", Type: "near_taste", Title: "Artists near your taste", Reason: "Close to the artists you have and follow", Shape: "round"},
			musicscore.Row{Prior: musicscore.PriorPersonal, Min: 5, ArtistRow: true}, cards, sc, true, true)
	}

	// Row 12: genre shelves for the top-3 G genres, 0.5·P + 0.3·G + 0.2·M.
	{
		type gv struct {
			g string
			v float64
		}
		var gs []gv
		for g, v := range in.G {
			gs = append(gs, gv{g, v})
		}
		sort.Slice(gs, func(i, j int) bool {
			if gs[i].v != gs[j].v {
				return gs[i].v > gs[j].v
			}
			return gs[i].g < gs[j].g
		})
		for _, x := range gs[:min(3, len(gs))] {
			if x.g == "" {
				continue
			}
			name, slug := x.g, ""
			for _, t := range in.tiles {
				if musicscore.ItemGenre(map[string]float64{x.g: 1}, []string{t.Name, t.Slug}) > 0 {
					name, slug = t.Name, t.Slug
					break
				}
			}
			var cards []MusicCard
			var sc []musicscore.Scored
			var ids []string
			type pending struct {
				c MusicCard
				m float64
			}
			var list []pending
			if slug != "" {
				for _, it := range a.musicGenreChartOf(ctx, region, slug, now) {
					c := in.chartCard(it)
					if in.hadAlbum[c.MBID] {
						continue
					}
					list = append(list, pending{c, musicscore.Bool(!asked(c.MBID))})
					ids = append(ids, c.MBID)
				}
			}
			for _, al := range lib.albums {
				ar := in.artistOf[al.titleID]
				if in.shares[ar.mbid][x.g] <= 0 || al.kind != "album" || al.secondary || al.date == "" || al.date > today || lib.hadParts[al.partID] {
					continue
				}
				list = append(list, pending{in.albumCard(al), musicscore.Bool(!asked(al.mbid))})
				ids = append(ids, al.mbid)
			}
			P := pOf(ids)
			for _, p := range list {
				cards = append(cards, p.c)
				sc = append(sc, in.scored(p.c, musicscore.ScoreGenreShelf(P[p.c.MBID], x.v, p.m), in.affMBID[p.c.ArtistMBID], x.v, ""))
			}
			if name == "" {
				continue
			}
			label := musicscore.Label(name)
			if slug == "" {
				label = musicscore.GenreName(x.g)
			}
			in.add(MusicRow{ID: "shelf:" + x.g, Type: "genre_shelf", Title: label + " for you", Reason: "One of your top genres: its chart and your artists in it", Shape: "square"},
				musicscore.Row{Prior: musicscore.PriorMixed}, cards, sc, true, false)
		}
	}

	// Row 13 (Phase 6, flag music_rows_v6): Rising in your genres.
	if a.s.MusicRowsV6 {
		a.risingRow(ctx, region, now, in)
	}

	// Row 14: Guest spots, release groups crediting a Library artist who is
	// not the primary artist, not had; 0.6·A + 0.4·P.
	{
		type guest struct {
			lib musicLibArtist
			al  metadata.MusicTopAlbum
		}
		var list []guest
		var ids []string
		got := map[string]bool{}
		for _, ar := range lib.artists {
			d := in.data[ar.mbid]
			if d == nil {
				continue
			}
			for _, al := range d.Albums {
				if !musicGuestTypes[al.Type] || len(al.Artists) < 2 || al.Artists[0] == ar.mbid || in.hadAlbum[al.MBID] || asked(al.MBID) || got[al.MBID] {
					continue
				}
				credited := false
				for _, x := range al.Artists[1:] {
					credited = credited || x == ar.mbid
				}
				if !credited {
					continue
				}
				got[al.MBID] = true
				list = append(list, guest{ar, al})
				ids = append(ids, al.MBID)
			}
		}
		P := pOf(ids)
		var cards []MusicCard
		var sc []musicscore.Scored
		for _, g := range list {
			al := g.al
			c := MusicCard{Key: "album:" + al.MBID, Kind: "album", MBID: al.MBID, Name: al.Title, ArtistMBID: al.Artists[0], Date: al.Date, Image: "/api/v1/music-trending/art/album/" + al.MBID, Note: "With " + g.lib.name, Genres: []string{}}
			if len(al.ArtistNames) > 0 && al.ArtistNames[0] != "" {
				c.Artist = al.ArtistNames[0]
				c.Note = c.Artist + " · with " + g.lib.name
			}
			cards = append(cards, c)
			sc = append(sc, in.scored(c, musicscore.ScoreGuest(in.aff[g.lib.id], P[al.MBID]), in.aff[g.lib.id], 0, in.primary[g.lib.mbid]))
		}
		in.add(MusicRow{ID: "guest_spots", Type: "guest_spots", Title: "Guest spots", Reason: "Releases your artists appear on", Shape: "square"},
			musicscore.Row{Prior: musicscore.PriorPersonal}, cards, sc, true, true)
	}

	if a.s.MusicRowsV7 {
		a.musicV7Rows(ctx, member, date, lib, acts, now, in, yesterday)
	}
}
