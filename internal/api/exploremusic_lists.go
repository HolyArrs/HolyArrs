package api

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs/internal/metadata"
	musicscore "github.com/CoderLuii/holyarrs/internal/music/score"
)

// World-list rows, flag music_lists_v1 (plan 2026-10-24): the metadata
// server's shelves (daily song charts turned into their albums or artists,
// weekly album and artist charts, new releases) become rows on Explore →
// Music and New & Popular → Music. Titles and reason lines come only from
// the templates below; list names are never shown. Personal mixing stays
// on the install: the shelves are the same for everyone in a region.

// musicShelvesTTL is how long one region's shelves are kept in memory (the
// server refreshes them at most a few times a day).
const musicShelvesTTL = 30 * time.Minute

// musicShelvesOf reads a region's shelves (nil without the flag, the
// server call, or an answer).
func (a *API) musicShelvesOf(ctx context.Context, region string, now time.Time) *metadata.MusicShelves {
	if !a.s.MusicListsV1 || a.s.MusicShelves == nil {
		return nil
	}
	key := "shelves:" + strings.ToLower(region)
	if v, ok := a.musicv2.cand.Get(key, now); ok {
		return v.(*metadata.MusicShelves)
	}
	sh, err := a.s.MusicShelves(ctx, strings.ToLower(region))
	if err != nil {
		markMusicMiss(ctx)
		return nil
	}
	a.musicv2.cand.Put(key, &sh, musicShelvesTTL, now)
	// New shelves (container start or a list refresh on the server): every
	// album and artist they hold is fetched ahead, details and pictures, so
	// rows show art at once.
	sig := sh.AsOf + "|" + fmt.Sprint(listCount(&sh))
	a.musicv2.cursorMu.Lock()
	if a.listSeen == nil {
		a.listSeen = map[string]string{}
	}
	prev := a.listSeen[key]
	changed := prev != sig
	a.listSeen[key] = sig
	a.musicv2.cursorMu.Unlock()
	if changed {
		go a.queueAhead(listAhead(&sh))
	}
	if changed && prev != "" {
		// The server refreshed its lists: today's plans rebuild on the next
		// visit so the new chart rows show.
		go a.forgetMusicV7Snaps(-1)
	}
	return &sh
}

// listCount is the shelves' item count (part of their change signature).
func listCount(sh *metadata.MusicShelves) int {
	n := 0
	for _, l := range sh.Blocks {
		n += len(l)
	}
	return n
}

// listAhead is every shelf album and artist, top blocks first, each once.
func listAhead(sh *metadata.MusicShelves) []aheadItem {
	var out []aheadItem
	seen := map[aheadItem]bool{}
	add := func(it aheadItem) {
		if metadata.ValidMBID(it.mbid) && !seen[it] {
			seen[it] = true
			out = append(out, it)
		}
	}
	for _, d := range listTemplates {
		for _, it := range sh.Blocks[d.Block] {
			id := strings.ToLower(it.MBID)
			if it.Kind == "artist" {
				add(aheadItem{"artist", id})
				continue
			}
			add(aheadItem{"album", id})
			add(aheadItem{"artist", strings.ToLower(it.ArtistMBID)})
		}
	}
	return out
}

// listCountries names the chart regions in row titles ("Top in Brazil").
var listCountries = map[string]string{"us": "the US", "gb": "the UK", "de": "Germany", "br": "Brazil", "mx": "Mexico", "ca": "Canada", "au": "Australia", "fr": "France",
	"es": "Spain", "it": "Italy", "nl": "the Netherlands", "se": "Sweden", "jp": "Japan", "in": "India", "kr": "South Korea", "ph": "the Philippines", "id": "Indonesia",
	"ar": "Argentina", "co": "Colombia", "cl": "Chile", "pl": "Poland", "tr": "Turkey", "za": "South Africa", "ng": "Nigeria", "no": "Norway", "dk": "Denmark", "fi": "Finland",
	"be": "Belgium", "at": "Austria", "ch": "Switzerland", "ie": "Ireland", "nz": "New Zealand", "pt": "Portugal", "pe": "Peru", "ec": "Ecuador", "ve": "Venezuela",
	"do": "the Dominican Republic", "gt": "Guatemala", "cr": "Costa Rica", "pa": "Panama", "py": "Paraguay", "uy": "Uruguay", "bo": "Bolivia", "hn": "Honduras",
	"sv": "El Salvador", "ni": "Nicaragua", "cz": "Czechia", "sk": "Slovakia", "hu": "Hungary", "ro": "Romania", "bg": "Bulgaria", "gr": "Greece", "il": "Israel",
	"ae": "the UAE", "sa": "Saudi Arabia", "eg": "Egypt", "ma": "Morocco", "my": "Malaysia", "sg": "Singapore", "th": "Thailand", "vn": "Vietnam", "tw": "Taiwan",
	"hk": "Hong Kong", "pk": "Pakistan", "ua": "Ukraine", "lt": "Lithuania", "lv": "Latvia", "ee": "Estonia", "is": "Iceland", "lu": "Luxembourg", "kz": "Kazakhstan"}

// listPlace is "in Brazil" or "worldwide".
func listPlace(region string) string {
	if n, ok := listCountries[strings.ToLower(region)]; ok {
		return "in " + n
	}
	return "worldwide"
}

// listRowDef is one world-list row: its block, our title and reason
// templates ({place}, {neighbor}), and how it treats Library items.
type listRowDef struct {
	ID, Block, Title, Reason string
	// Chart rows keep had items (with the Have chip); others drop them.
	Chart bool
	// Min is the gate (default 8).
	Min int
}

// listTemplates are every row's title and reason templates; a reason is
// always one of these, rendered (TestListReasonsAreTemplates).
var listTemplates = []listRowDef{
	{ID: "lists_top_now", Block: "top_now", Title: "Top {place}", Reason: "The most-played songs {place} today, as the albums and artists behind them", Chart: true, Min: 10},
	{ID: "lists_top_world", Block: "top_world", Title: "Top {place2}", Reason: "The most-played songs {place2} today", Chart: true, Min: 10},
	{ID: "lists_new_friday", Block: "new_friday", Title: "Out this Friday", Reason: "New albums and EPs out in the last two weeks"},
	{ID: "lists_climbing", Block: "climbing", Title: "Climbing fast", Reason: "Rising on the charts {place} this week"},
	{ID: "lists_albums_week", Block: "albums_week", Title: "Albums of the week", Reason: "The most-played albums {place} this week", Chart: true, Min: 10},
	{ID: "lists_new_entries", Block: "new_entries", Title: "Taking off", Reason: "New on the charts {place} in the last 7 days"},
	{ID: "lists_artists_week", Block: "artists_week", Title: "Artists everyone is playing", Reason: "The most-played artists {place} this week", Chart: true, Min: 10},
	{ID: "lists_big_new", Block: "big_new_albums", Title: "Biggest new albums", Reason: "Out in the last 30 days and already charting {place}"},
	{ID: "lists_albums_world", Block: "albums_world", Title: "Albums of the week worldwide", Reason: "The most-played albums in the world this week", Chart: true, Min: 10},
	{ID: "lists_singles", Block: "singles_artists", Title: "Singles blowing up", Reason: "The artists behind the biggest singles {place} right now"},
	{ID: "lists_long", Block: "long_runners", Title: "Still on repeat", Reason: "On the charts {place} for weeks and weeks", Chart: true},
	{ID: "lists_number_ones", Block: "number_ones", Title: "Number ones this month", Reason: "Hit number one {place} in the last 30 days", Chart: true, Min: 6},
	{ID: "lists_comebacks", Block: "comebacks", Title: "Big comebacks", Reason: "Back on the charts {place} after time away"},
	{ID: "lists_neighbor", Block: "neighbor", Title: "Top {neighbor}", Reason: "The most-played songs {neighbor} today", Chart: true, Min: 10},
}

// listRows renders the templates for one region's shelves; rows whose
// block is absent are left out.
func listRows(sh *metadata.MusicShelves) []listRowDef {
	if sh == nil {
		return nil
	}
	place := listPlace(sh.Chart)
	place2 := "worldwide"
	if sh.Chart == "global" {
		place2 = listPlace("us")
	}
	r := strings.NewReplacer("{place2}", place2, "{place}", place, "{neighbor}", listPlace(sh.Neighbor))
	var out []listRowDef
	for _, d := range listTemplates {
		if len(sh.Blocks[d.Block]) == 0 {
			continue
		}
		d.Title, d.Reason = r.Replace(d.Title), r.Replace(d.Reason)
		if d.Min == 0 {
			d.Min = 8
		}
		out = append(out, d)
	}
	return out
}

// listCard is one shelf item as a Music card.
func listCard(it metadata.MusicShelfItem) MusicCard {
	id := strings.ToLower(it.MBID)
	if it.Kind == "artist" {
		c := MusicCard{Key: "artist:" + id, Kind: "artist", MBID: id, Name: it.Artist, Artist: it.Artist, ArtistMBID: id, Image: "/api/v1/music-trending/art/artist/" + id, Genres: []string{}}
		if it.Song != "" {
			c.Note = "Known for " + it.Song
		}
		return c
	}
	c := MusicCard{Key: "album:" + id, Kind: "album", MBID: id, Name: it.Title, Artist: it.Artist, ArtistMBID: strings.ToLower(it.ArtistMBID), Date: it.Date, Image: "/api/v1/music-trending/art/album/" + id, Genres: []string{}}
	c.ArtistImage = "/api/v1/music-trending/art/artist/" + c.ArtistMBID
	return c
}

// listScore is the row's own order: rank for chart rows, momentum for
// Climbing fast, days on chart for Still on repeat (§4); P breaks ties.
func listScore(block string, it metadata.MusicShelfItem) float64 {
	switch block {
	case "climbing":
		return it.M + it.P/100
	case "long_runners":
		return float64(it.Days)/1000 + it.P/100
	case "new_friday":
		return 1 - float64(it.Rank)/1000
	}
	return it.P
}

// listCandidates turns one row's block into cards and scored items:
// keep says whether a card may show (had, asked, explicit).
func listCandidates(d listRowDef, items []metadata.MusicShelfItem, keep func(MusicCard) bool) ([]MusicCard, []musicscore.Scored) {
	var cards []MusicCard
	var sc []musicscore.Scored
	for _, it := range items {
		if !metadata.ValidMBID(strings.ToLower(it.MBID)) {
			continue
		}
		c := listCard(it)
		if n := listNote(d.Block, it); n != "" {
			c.Note = n
		}
		if keep != nil && !keep(c) {
			continue
		}
		cards = append(cards, c)
		item := musicscore.Item{Key: c.Key, Artist: strings.ToLower(c.ArtistMBID + "|" + c.Artist), Score: listScore(d.Block, it)}
		if c.Kind == "album" {
			item.Album = c.MBID
			item.FoldKey = fullAlbumKey(c.Name) + "|" + fullAlbumKey(c.Artist)
		}
		sc = append(sc, musicscore.Scored{Item: item})
	}
	sort.SliceStable(sc, func(i, j int) bool { return sc[i].Score > sc[j].Score })
	return cards, sc
}

// distinctArtists counts the artists of a candidate list.
func distinctArtists(sc []musicscore.Scored) int {
	seen := map[string]bool{}
	for _, s := range sc {
		seen[s.Artist] = true
	}
	return len(seen)
}

// listHero is the top albums of the region's song chart, for the Spotlight
// ("Biggest right now"): P·0.6 + M·0.4, not-had first.
func listHero(sh *metadata.MusicShelves, had func(string) bool, n int) []MusicCard {
	if sh == nil {
		return nil
	}
	type cand struct {
		c MusicCard
		s float64
	}
	var list []cand
	artists := map[string]bool{}
	for _, it := range sh.Blocks["top_now"] {
		if it.Kind != "album" || !metadata.ValidMBID(strings.ToLower(it.MBID)) {
			continue
		}
		c := listCard(it)
		if artists[c.ArtistMBID] {
			continue
		}
		artists[c.ArtistMBID] = true
		s := 0.6*it.P + 0.4*it.M
		if had(c.MBID) {
			s -= 1
		}
		c.Kicker = "Biggest right now"
		list = append(list, cand{c, s})
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].s > list[j].s })
	var out []MusicCard
	for _, x := range list[:min(n, len(list))] {
		out = append(out, x.c)
	}
	return out
}

// listRowMeta is the row's MusicRow head (shape follows the cards: the UI
// draws each card by its kind).
func listRowMeta(d listRowDef) MusicRow {
	shape := "square"
	if d.Block == "artists_week" || d.Block == "singles_artists" {
		shape = "round"
	}
	return MusicRow{ID: d.ID, Type: d.ID, Title: d.Title, Reason: d.Reason, Shape: shape}
}

// listNote adds a chart line to a card where the row has one.
func listNote(block string, it metadata.MusicShelfItem) string {
	switch block {
	case "climbing":
		if it.Prev > it.Rank {
			return fmt.Sprintf("Up %d to #%d", it.Prev-it.Rank, it.Rank)
		}
	case "long_runners":
		if it.Days > 0 {
			return fmt.Sprintf("%d days on the chart", it.Days)
		}
	}
	return ""
}

// browseCovers gives each Browse tile its genre's top charted album as the
// tilted cover, never the same album on two tiles; a tile without one
// keeps its colour alone.
func (a *API) browseCovers(ctx context.Context, region string, tiles []MusicCard, now time.Time) {
	used := map[string]bool{}
	for i := range tiles {
		for _, it := range a.musicGenreChartOf(ctx, region, tiles[i].MBID, now) {
			id := strings.ToLower(it.MBID)
			if metadata.ValidMBID(id) && !used[id] {
				used[id] = true
				tiles[i].Cover = "/api/v1/music-trending/art/album/" + id
				break
			}
		}
	}
}

// listPrior is the world-list rows' prior on Explore: under the personal
// rows' 1.2, so the day's plan mixes chart and personal rows.
const listPrior = 0.85

// listNewOrder is New & Popular's row order (§6): popularity first.
var listNewOrder = []string{"lists_new_friday", "lists_top_now", "lists_top_world", "lists_climbing", "lists_new_entries", "lists_albums_week", "lists_albums_world",
	"lists_artists_week", "lists_big_new", "lists_singles", "lists_long", "lists_neighbor", "lists_comebacks", "lists_number_ones"}

// musicNewListRows leads New & Popular → Music with the world-list rows:
// the Spotlight is the region's biggest charted albums, then the list rows
// in §6 order, then the page's own rows; one seen set over the page, so no
// album shows twice.
func (a *API) musicNewListRows(ctx context.Context, r *http.Request, in musicNewInput, hero []MusicCard, rows []MusicRow, now time.Time) ([]MusicCard, []MusicRow) {
	region := strings.ToUpper(r.URL.Query().Get("region"))
	if region == "" || region == "GLOBAL" || !isCountry(region) {
		region = "global"
	}
	sh := a.musicShelvesOf(ctx, region, now)
	if sh == nil {
		return hero, rows
	}
	had := map[string]bool{}
	for _, c := range in.library {
		if c.MBID != "" {
			had[c.MBID] = true
		}
	}
	seen := map[string]bool{}
	mark := func(c MusicCard) {
		seen[c.Key] = true
		if c.Kind == "album" {
			seen["fold:"+fullAlbumKey(c.Name)+"|"+fullAlbumKey(c.Artist)] = true
		}
	}
	taken := func(c MusicCard) bool {
		return seen[c.Key] || c.Kind == "album" && seen["fold:"+fullAlbumKey(c.Name)+"|"+fullAlbumKey(c.Artist)]
	}
	lead := listHero(sh, func(id string) bool { return had[id] }, 8)
	if len(lead) >= 3 {
		hero = lead
	}
	for _, c := range hero {
		mark(c)
	}
	defs := map[string]listRowDef{}
	for _, d := range listRows(sh) {
		defs[d.ID] = d
	}
	var out []MusicRow
	for _, id := range listNewOrder {
		d, ok := defs[id]
		if !ok {
			continue
		}
		cards, sc := listCandidates(d, sh.Blocks[d.Block], nil)
		byKey := map[string]MusicCard{}
		for _, c := range cards {
			byKey[c.Key] = c
		}
		row := listRowMeta(d)
		per := map[string]int{}
		for _, s := range sc {
			c := byKey[s.Key]
			if taken(c) || per[s.Artist] >= musicscore.PerArtist || len(row.Items) >= musicscore.RowMax {
				continue
			}
			per[s.Artist]++
			row.Items = append(row.Items, c)
		}
		if len(row.Items) < d.Min || len(per) < 4 {
			continue
		}
		for _, c := range row.Items {
			mark(c)
		}
		out = append(out, row)
	}
	for _, row := range rows {
		var items []MusicCard
		for _, c := range row.Items {
			if !taken(c) {
				items = append(items, c)
			}
		}
		if len(items) < min(6, len(row.Items)) {
			continue
		}
		for _, c := range items {
			mark(c)
		}
		row.Items = items
		out = append(out, row)
	}
	return hero, out
}

// musicV7HeadRows is the plan's fixed head: with music_lists_v1 the
// region's top chart row follows New from your artists and Browse.
func (a *API) musicV7HeadRows() []string {
	if a.s.MusicListsV1 && a.s.MusicShelves != nil {
		return append(append([]string{}, musicV7Head...), "lists_top_now")
	}
	return musicV7Head
}
