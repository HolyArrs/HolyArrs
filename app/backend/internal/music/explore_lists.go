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

// World-list rows (plan 24 October 2026): the metadata
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
func (h *Handlers) musicShelvesOf(ctx context.Context, region string, now time.Time) *metadata.MusicShelves {
	if h.MusicShelves == nil {
		return nil
	}
	key := "shelves:" + strings.ToLower(region)
	if v, ok := h.exploreCache.cand.Get(key, now); ok {
		return v.(*metadata.MusicShelves)
	}
	sh, err := h.MusicShelves(ctx, strings.ToLower(region))
	if err != nil {
		markMusicMiss(ctx)
		return nil
	}
	h.exploreCache.cand.Put(key, &sh, musicShelvesTTL, now)
	// New shelves (container start or a list refresh on the server): every
	// album and artist they hold is fetched ahead, details and pictures, so
	// rows show art at once.
	sig := sh.AsOf + "|" + fmt.Sprint(listCount(&sh))
	h.exploreCache.cursorMu.Lock()
	if h.listSeen == nil {
		h.listSeen = map[string]string{}
	}
	prev := h.listSeen[key]
	h.listSeen[key] = sig
	h.exploreCache.cursorMu.Unlock()
	first := prev == ""
	if first && h.Store != nil {
		// After a restart: the shelves the kept plans were built from.
		if b, ok, _ := (metadata.LookupCache{Store: h.Store}).Get(ctx, listSigPrefix+key, now); ok {
			prev = string(b)
		}
	}
	changed := prev != sig
	if changed {
		go h.queueAhead(listAhead(&sh))
		go h.warmListVideos(&sh)
		if h.Store != nil {
			_ = metadata.LookupCache{Store: h.Store}.Put(context.WithoutCancel(ctx), listSigPrefix+key, []byte(sig), now, now.Add(30*24*time.Hour))
		}
	}
	if changed && (prev != "" || first) {
		// The server refreshed its lists (also while this install was
		// down): today's plans rebuild on the next visit so the new rows show.
		go h.forgetMusicSnaps(-1)
	}
	return &sh
}

// listVideoWarm is how many video-row artists get their video list asked
// ahead when shelves change (one server call each, one at a time).
const listVideoWarm = 30

// warmListVideos asks the video row's artists for their music videos
// ahead, so the row shows once their videos are known.
func (h *Handlers) warmListVideos(sh *metadata.MusicShelves) {
	if h.ArtistVideos == nil {
		return
	}
	ctx := context.Background()
	for i, it := range sh.Blocks["video_artists"] {
		if i >= listVideoWarm {
			break
		}
		id := strings.ToLower(it.MBID)
		if !metadata.ValidMBID(id) {
			continue
		}
		if _, ok := h.keptVideos(ctx, id); !ok {
			h.videosOf(ctx, id)
		}
	}
}

// listBlock is a row's shelf items; the video row keeps only artists
// whose music videos are already known (the card shows its picture until
// the stream is ready).
func (h *Handlers) listBlock(ctx context.Context, sh *metadata.MusicShelves, d listRowDef) []metadata.MusicShelfItem {
	items := sh.Blocks[d.Block]
	if d.Block == "new_friday" || d.Block == "big_new_albums" {
		// Rows meant to be new: releases of the last 90 days only.
		var out []metadata.MusicShelfItem
		for _, it := range items {
			if listRecent(it.Date, listRecentDays, time.Now()) {
				out = append(out, it)
			}
		}
		return out
	}
	if d.Block != "video_artists" {
		return items
	}
	var out []metadata.MusicShelfItem
	for _, it := range items {
		if e, ok := h.keptVideos(ctx, strings.ToLower(it.MBID)); ok && len(e.list) > 0 {
			out = append(out, it)
		}
	}
	return out
}

// listSigPrefix keys the kept shelves signature per region (disk), so a
// restart knows whether the kept plans predate the current shelves.
const listSigPrefix = "listsig|"

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
	names := make([]string, 0, len(sh.Blocks))
	for _, d := range listTemplates {
		if !strings.HasPrefix(d.Block, "tile:") {
			names = append(names, d.Block)
		}
	}
	for _, t := range listTiles {
		names = append(names, "tile:"+t.Key)
	}
	for _, name := range names {
		for _, it := range sh.Blocks[name] {
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
	{ID: "lists_videos", Block: "video_artists", Title: "Music videos people watch", Reason: "Artists charting {place} this week, with their music videos", Min: 6},
	// Tile rows: one per Browse tile with editorial lists ({tile} is our
	// tile name; the block is "tile:" + the tile's key).
	{ID: "lists_tile_", Block: "tile:genre", Title: "{tile} right now", Reason: "Picked from this week's {tile} lists, as the albums and artists behind the songs"},
	{ID: "lists_tile_", Block: "tile:mood", Title: "{tile} picks", Reason: "Albums and artists from this week's {tile} lists"},
	{ID: "lists_tile_", Block: "tile:decades", Title: "Throwback hits", Reason: "Older favourites from this week's throwback lists"},
	{ID: "lists_tile_", Block: "tile:fresh", Title: "Fresh faces", Reason: "New artists picked on this week's discovery lists"},
	{ID: "lists_big_", Block: "tile:week", Title: "Big this week in {tile}", Reason: "The albums and artists behind this week's {tile} lists"},
}

// listTile is one Browse tile with editorial lists: the server's block
// key, our name, its template group and the genre slugs it stands for.
type listTile struct {
	Key, Name, Group string
	Slugs            []string
}

// listTiles are the tiles in Explore order (biggest genres, then moods).
var listTiles = []listTile{
	{"pop", "Pop", "genre", []string{"pop"}},
	{"latin", "Latin", "genre", []string{"latin", "reggaeton", "latin-pop"}},
	{"rock", "Rock", "genre", []string{"rock"}},
	{"country", "Country", "genre", []string{"country"}},
	{"dance", "Dance & electronic", "genre", []string{"dance", "electronic", "edm", "house", "dance-pop"}},
	{"rnb", "R&B", "genre", []string{"r-and-b", "rnb", "randb", "contemporary-r-and-b"}},
	{"chill", "Chill", "mood", nil},
	{"party", "Party", "mood", nil},
	{"indie", "Indie", "genre", []string{"indie", "indie-pop", "indie-rock"}},
	{"decades", "Throwback", "decades", nil},
	{"fresh_faces", "Fresh faces", "fresh", nil},
	{"kpop", "K-pop", "genre", []string{"k-pop", "kpop"}},
	{"metal", "Metal", "genre", []string{"metal", "heavy-metal"}},
	{"jazz", "Jazz", "genre", []string{"jazz"}},
	{"workout", "Workout", "mood", nil},
	{"love", "Love songs", "mood", nil},
	{"alternative", "Alternative", "genre", []string{"alternative", "alternative-rock"}},
	{"folk", "Folk & acoustic", "genre", []string{"folk", "acoustic"}},
	{"soul", "Soul", "genre", []string{"soul"}},
	{"classical", "Classical", "genre", []string{"classical"}},
	{"punk", "Punk", "genre", []string{"punk", "pop-punk"}},
	{"blues", "Blues", "genre", []string{"blues"}},
	{"funk", "Funk & disco", "genre", []string{"funk", "disco"}},
	{"ambient", "Ambient", "genre", []string{"ambient"}},
	{"focus", "Focus", "mood", nil},
	{"sleep", "Sleep", "mood", nil},
	{"feelings", "Feelings", "mood", nil},
	{"afro", "Afro", "genre", []string{"afrobeats", "afrobeat", "afro"}},
	{"brazil", "Brazil", "genre", []string{"brazilian", "mpb", "sertanejo", "funk-carioca"}},
}

// listTileOf is the tile a genre page slug stands for.
func listTileOf(slug string) (listTile, bool) {
	for _, t := range listTiles {
		if t.Key == slug {
			return t, true
		}
		for _, s := range t.Slugs {
			if s == slug {
				return t, true
			}
		}
	}
	return listTile{}, false
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
		if strings.HasPrefix(d.Block, "tile:") {
			// Explore's tile rows; "Big this week" rows are New & Popular's.
			if d.Block != "tile:week" {
				out = append(out, listTileRows(sh, d, d.Block[len("tile:"):])...)
			}
			continue
		}
		if len(sh.Blocks[d.Block]) == 0 || d.Block == "albums_world" && sh.Chart == "global" {
			// Worldwide visitors: the week's album chart is already the world's.
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

// listTileRows renders one tile template for every tile of its group
// with a block on the shelves.
func listTileRows(sh *metadata.MusicShelves, d listRowDef, group string) []listRowDef {
	var out []listRowDef
	for _, t := range listTiles {
		if group != "week" && t.Group != group || group == "week" && t.Group != "genre" || len(sh.Blocks["tile:"+t.Key]) == 0 {
			continue
		}
		e := d
		r := strings.NewReplacer("{tile}", t.Name)
		e.ID, e.Block, e.Title, e.Reason = d.ID+t.Key, "tile:"+t.Key, r.Replace(d.Title), r.Replace(d.Reason)
		if e.Min == 0 {
			e.Min = 8
		}
		out = append(out, e)
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
	c.ArtistImage = heroArtistImage(c.ArtistMBID)
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
	switch d.Block {
	case "artists_week", "singles_artists":
		shape = "round"
	case "video_artists":
		shape = "wide"
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
// tilted cover, never the same album on two tiles. Only an album whose
// picture is stored or known to load is taken (the check is off until a
// preload has stored a picture); a tile without one has no cover, and
// coveredTiles leaves it out.
func (h *Handlers) browseCovers(ctx context.Context, region string, tiles []MusicCard, now time.Time) {
	used := map[string]bool{}
	hasArt := h.albumHasArt()
	for i := range tiles {
		for _, it := range h.musicGenreChartOf(ctx, region, tiles[i].MBID, now) {
			id := strings.ToLower(it.MBID)
			if metadata.ValidMBID(id) && !used[id] && (hasArt == nil || hasArt(id)) {
				used[id] = true
				tiles[i].Cover = "/api/v1/music-trending/art/album/" + id
				break
			}
		}
	}
}

// coveredTiles keeps the Browse tiles that have an album cover, with the
// scores that go with them. A tile shows an album cover or nothing, so the
// genre's artist pictures are dropped from the ones that stay.
func coveredTiles(cards []MusicCard, sc []musicscore.Scored) ([]MusicCard, []musicscore.Scored) {
	var keepCards []MusicCard
	var keepScores []musicscore.Scored
	for i, c := range cards {
		if c.Cover == "" {
			continue
		}
		c.Image, c.Images = "", nil
		keepCards = append(keepCards, c)
		if i < len(sc) {
			keepScores = append(keepScores, sc[i])
		}
	}
	return keepCards, keepScores
}

// listPrior is the world-list rows' prior on Explore: under the personal
// rows' 1.2, so the day's plan mixes chart and personal rows.
const listPrior = 0.85

// listNewOrder is New & Popular's row order (§6): popularity first.
var listNewOrder = []string{"lists_new_friday", "lists_top_now", "lists_top_world", "lists_climbing", "lists_new_entries", "lists_albums_week", "lists_albums_world",
	"lists_artists_week", "lists_big_new", "lists_singles", "lists_long", "lists_neighbor", "lists_comebacks", "lists_number_ones"}

// listNewWeekRows is how many "Big this week in {genre}" rows New &
// Popular shows (§6 rows 15–20).
const listNewWeekRows = 6

// listNewDays: a card says "New" only within 60 days of its first release.
const listNewDays = 60

// listRecentDays: rows meant to be new take releases of the last 90 days.
const listRecentDays = 90

// listAge is the days since a release date ("2 October 2026", "October 2026" or
// "2026"; a partial date counts from its first day).
func listAge(date string, now time.Time) (int, bool) {
	for _, layout := range []string{"2006-01-02", "2006-01", "2006"} {
		if len(date) >= len(layout) {
			if t, err := time.Parse(layout, date[:len(layout)]); err == nil {
				today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
				return int(today.Sub(t).Hours() / 24), true
			}
		}
	}
	return 0, false
}

// listRecent says a release is out (or due within a week) and at most
// days old.
func listRecent(date string, days int, now time.Time) bool {
	d, ok := listAge(date, now)
	return ok && d >= -7 && d <= days
}

// listKicker is a chart card's label from its real reason: "New album"
// only within 60 days of release, else its chart place or why it shows.
func listKicker(block, place string, it metadata.MusicShelfItem, now time.Time) string {
	if it.Kind == "album" && listRecent(it.Date, listNewDays, now) {
		return "New album"
	}
	switch block {
	case "top_now", "top_world", "neighbor":
		if it.Rank > 0 && it.Rank <= 50 {
			return fmt.Sprintf("#%d %s", it.Rank, place)
		}
	case "climbing", "new_entries":
		return "Climbing"
	case "long_runners":
		return "Still huge"
	case "albums_week", "albums_world":
		if it.Rank > 0 && it.Rank <= 50 {
			return fmt.Sprintf("#%d album this week", it.Rank)
		}
	}
	return "Popular now"
}

// honestLabels takes "New" off every card released more than 60 days ago
// (or with no date): the label then says why the card is there.
func honestLabels(cards []MusicCard, now time.Time) {
	for i := range cards {
		c := &cards[i]
		if listRecent(c.Date, listNewDays, now) {
			continue
		}
		if strings.HasPrefix(c.Kicker, "New") {
			c.Kicker = "Popular now"
			if c.Rank > 0 && c.Rank <= 50 {
				c.Kicker = fmt.Sprintf("#%d album this week", c.Rank)
			}
		}
		if strings.HasPrefix(c.Badge, "New") {
			c.Badge = ""
			if c.Kicker == "" {
				c.Kicker = "Popular now"
			}
		}
	}
}

// listRecentBlocks feed New & Popular's first rows and hero: chart albums
// out in the last 90 days.
var listRecentBlocks = []string{"big_new_albums", "new_friday", "new_entries", "climbing", "top_now", "top_world", "albums_week"}

// listRecentDef is the "New and charting" row (rendered like a template).
var listRecentDef = listRowDef{ID: "lists_new_charting", Block: "recent", Title: "New and charting", Reason: "Out in the last 90 days and on the charts {place}", Min: 8}

// listRecentItems are the shelves' albums out in the last 90 days, once
// each, by 0.6·M + 0.4·P (momentum first), with the block each came from.
func listRecentItems(sh *metadata.MusicShelves, now time.Time) ([]metadata.MusicShelfItem, map[string]string) {
	from := map[string]string{}
	var out []metadata.MusicShelfItem
	for _, b := range listRecentBlocks {
		for _, it := range sh.Blocks[b] {
			id := strings.ToLower(it.MBID)
			if it.Kind != "album" || from[id] != "" || !listRecent(it.Date, listRecentDays, now) {
				continue
			}
			from[id] = b
			out = append(out, it)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return 0.6*out[i].M+0.4*out[i].P > 0.6*out[j].M+0.4*out[j].P
	})
	return out, from
}

// listNewOwnFirst are New & Popular's own rows of new releases: they
// follow the recent list rows, above the popularity rows.
func listNewOwnFirst(id string) bool {
	switch id {
	case "np_music_week", "np_music_recent", "np_music_library", "np_music_short", "np_music_coming":
		return true
	}
	return strings.HasPrefix(id, "np_music_genre_")
}

// musicNewListRows leads New & Popular → Music with recent music: the hero
// is the region's charting albums out in the last 90 days (momentum
// first), then rows of recent releases, and only then the popularity rows
// (charts, which hold older hits under labels that say so). One seen set
// over the page, so no album shows twice.
func (h *Handlers) musicNewListRows(ctx context.Context, r *http.Request, in musicNewInput, hero []MusicCard, rows []MusicRow, now time.Time) ([]MusicCard, []MusicRow) {
	region := strings.ToUpper(r.URL.Query().Get("region"))
	if region == "" || region == "GLOBAL" || !isCountry(region) {
		region = "global"
	}
	sh := h.musicShelvesOf(ctx, region, now)
	if sh == nil {
		honestLabels(hero, now)
		return hero, rows
	}
	place := listPlace(sh.Chart)
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
	recent, from := listRecentItems(sh, now)
	// Hero: recent charting albums, one per artist, not-had first.
	var lead []MusicCard
	artists := map[string]bool{}
	for _, wantHad := range []bool{false, true} {
		for _, it := range recent {
			c := listCard(it)
			if len(lead) >= 8 || artists[c.ArtistMBID] || had[c.MBID] != wantHad {
				continue
			}
			artists[c.ArtistMBID] = true
			c.Kicker = listKicker(from[c.MBID], place, it, now)
			lead = append(lead, c)
		}
	}
	if len(lead) >= 3 {
		hero = lead
	}
	honestLabels(hero, now)
	for _, c := range hero {
		mark(c)
	}
	var out []MusicRow
	take := func(d listRowDef, items []metadata.MusicShelfItem) {
		cards, sc := listCandidates(d, items, nil)
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
			return
		}
		for _, c := range row.Items {
			mark(c)
		}
		out = append(out, row)
	}
	own := func(first bool) {
		for _, row := range rows {
			if listNewOwnFirst(row.ID) != first {
				continue
			}
			var items []MusicCard
			for _, c := range row.Items {
				// Best new songs is a new row: songs of the last 90 days only.
				if !taken(c) && (row.ID != "np_music_tracks" || listRecent(c.Date, listRecentDays, now)) {
					items = append(items, c)
				}
			}
			if len(items) < min(6, len(row.Items)) {
				continue
			}
			honestLabels(items, now)
			for _, c := range items {
				mark(c)
			}
			row.Items = items
			out = append(out, row)
		}
	}
	defs := map[string]listRowDef{}
	for _, d := range listRows(sh) {
		defs[d.ID] = d
	}
	recentRows := map[string]bool{"lists_new_friday": true, "lists_big_new": true, "lists_new_entries": true}
	// 1. Recent: new and charting, out this Friday, biggest new albums,
	// taking off; every item out in the last 90 days.
	rd := listRecentDef
	rd.Reason = strings.ReplaceAll(rd.Reason, "{place}", place)
	take(rd, recent)
	for _, id := range []string{"lists_new_friday", "lists_big_new", "lists_new_entries"} {
		if d, ok := defs[id]; ok {
			var items []metadata.MusicShelfItem
			for _, it := range sh.Blocks[d.Block] {
				if it.Kind == "album" && listRecent(it.Date, listRecentDays, now) {
					items = append(items, it)
				}
			}
			take(d, items)
		}
	}
	// 2. The page's own new-release rows.
	own(true)
	// 3. Popularity rows: the charts (older hits welcome, labelled as chart
	// rows), Big this week in {genre}, the page's other rows, the videos.
	for _, id := range listNewOrder {
		if d, ok := defs[id]; ok && !recentRows[id] {
			take(d, h.listBlock(ctx, sh, d))
		}
	}
	for _, d := range listTemplates {
		if d.Block == "tile:week" {
			for i, e := range listTileRows(sh, d, "week") {
				if i < listNewWeekRows {
					take(e, sh.Blocks[e.Block])
				}
			}
		}
	}
	own(false)
	if d, ok := defs["lists_videos"]; ok {
		take(d, h.listBlock(ctx, sh, d))
	}
	return hero, out
}

// musicHeadRows is the plan's fixed head: the
// region's top chart row follows New from your artists and Browse.
func (h *Handlers) musicHeadRows() []string {
	if h.musicTplOn() {
		return append(append([]string{}, musicHead...), "tpl:A01")
	}
	if h.MusicShelves != nil {
		return append(append([]string{}, musicHead...), "lists_top_now")
	}
	return musicHead
}
