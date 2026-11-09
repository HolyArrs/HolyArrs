package music

import (
	"context"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/music/rowtpl"
	musicscore "github.com/HolyArrs/holyarrs/app/backend/internal/music/score"
)

// buildMusicOrder builds layer 2 with the Phase 3 scores.
func (h *Handlers) buildMusicOrder(ctx context.Context, member int64, region string, lib *musicLib, acts *musicActs, now time.Time) *musicOrder {
	ord := &musicOrder{}
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
	// These metadata reads are independent: they run at once.
	var shelves *metadata.MusicShelves
	var data map[string]*metadata.MusicArtistData
	var albums, songs []metadata.MusicChartItem
	var reads sync.WaitGroup
	reads.Add(4)
	go func() { defer reads.Done(); shelves = h.musicShelvesOf(ctx, region, now) }()
	// Server data (layer 1): genre votes and top albums of library artists.
	go func() { defer reads.Done(); data = h.musicArtistDataOf(ctx, libIDs, now) }()
	go func() { defer reads.Done(); albums = h.musicChartOf(ctx, region, "albums", now) }()
	go func() { defer reads.Done(); songs = h.musicChartOf(ctx, region, "songs", now) }()
	reads.Wait()
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
	// first, then chart new releases, climbers and the top (the region's
	// biggest last), rowtpl.SpotlightMax at most, one card per album and
	// per artist.
	const heroMax = rowtpl.SpotlightMax
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
		c.ArtistImage = heroArtistImage(c.ArtistMBID)
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
	// With the chart lists on, the album charts can be empty: the region's
	// biggest albums fill what is left.
	for _, c := range listHero(shelves, func(id string) bool { return hadAlbum[id] }, heroMax) {
		fold := fullAlbumKey(c.Name) + "|" + fullAlbumKey(c.Artist)
		if len(heroes) >= heroMax || inHero[c.Key] || inHero[fold] || byArtist[strings.ToLower(c.Artist)] {
			continue
		}
		inHero[c.Key], inHero[fold], byArtist[strings.ToLower(c.Artist)] = true, true, true
		heroes = append(heroes, c)
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
	tiles := h.musicGenreTiles(ctx, region, now)
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
			h.browseCovers(ctx, region, cards, now)
			cards, sc = coveredTiles(cards, sc)
		}
		// Row templates: Explore's Browse row is the mix of the Browse page (genres,
		// moods, the season, decades, activities), with covers of albums whose
		// picture is stored; a tile with none is left out.
		if h.musicTplOn() {
			if b := h.musicBundle(ctx, now); b != nil {
				cards, sc = nil, nil
				tiles := rowtpl.ExploreTiles(h.tplVisibleArt(ctx, b, region, nil, now))
				h.tplRememberCovers(b, []rowtpl.BrowseSection{{Tiles: tiles}})
				for _, t := range tiles {
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
		h.personalMusicRows(ctx, member, region, date, lib, acts, now, personalIn{
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
	if h.musicTplOn() {
		if b := h.musicBundle(ctx, now); b != nil {
			h.tplPreload(b, region)
			for _, cd := range h.tplExploreCands(ctx, member, region, lib, acts, b, shelves, now) {
				meta, row, cards, sc := tplPlanRow(b, cd)
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
			cards, sc := listCandidates(d, h.listBlock(ctx, shelves, d), keep)
			if len(sc) < d.Min || distinctArtists(sc) < 4 {
				continue
			}
			addRow(listRowMeta(d), musicscore.Row{Prior: listPrior, Min: d.Min, Family: "lists:" + d.ID}, cards, sc, false, false)
		}
	}

	// v7 keeps the raw rows: the day's plan orders and fills them.
	c := &musicCand{metas: map[string]MusicRow{}, cards: map[string]map[string]MusicCard{}, seen: seen}
	for _, b := range rows {
		r := b.row
		r.Family, r.Disc = musicFamily(b.meta)
		c.rows = append(c.rows, r)
		c.metas[r.ID], c.cards[r.ID] = b.meta, b.cards
	}
	ord.v7 = c
	ord.Rows = []MusicRow{}
	if h.MusicTrending != nil {
		if l, err := h.cachedMusicTrend(ctx, "album", "global"); err == nil {
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
