package music

import (
	"context"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/music/rowtpl"
	musicscore "github.com/HolyArrs/holyarrs/app/backend/internal/music/score"
)

// Music genre pages (plan 22 October 2026, Phase 2, §2.8), Music only: built
// from the region chart filtered to the genre, the Library's artists in
// the genre (MusicBrainz genres or a charted album), their missing albums
// and the genre's popular artists. Movies and Shows genre pages are the
// old ones, untouched.

var musicSlugRe = SlugRe

// musicSlug is a genre name's URL name, the metadata server's rule.
func musicSlug(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ReplaceAll(strings.ToLower(name), "&", "and") {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case !dash && b.Len() > 0:
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}

// genreMatch says whether a MusicBrainz genre (slug) belongs to a chart
// genre slug: the same, or one names the other's first words ("hip-hop"
// in "hip-hop-rap").
func genreMatch(mb, slug string) bool {
	return mb != "" && (mb == slug || strings.HasPrefix(slug, mb+"-") || strings.HasPrefix(mb, slug+"-"))
}

// musicGenreArt is a genre tile's picture link in the app, or "".
func musicGenreArt(g metadata.MusicGenre) string {
	const pre = "/v1/music/genres/tile/"
	if !strings.HasPrefix(g.Art, pre) {
		return ""
	}
	u, err := url.Parse(g.Art)
	if err != nil {
		return ""
	}
	return "/api/v1/music/genre-art/" + g.Slug + "?v=" + url.QueryEscape(u.Query().Get("v"))
}

// musicGenreArts is a genre's slideshow in the app: photo 0 is the same
// link as musicGenreArt, the others add n=1..3.
func musicGenreArts(g metadata.MusicGenre) []string {
	const pre = "/v1/music/genres/tile/"
	var out []string
	for i, a := range g.Arts {
		if i > 3 || !strings.HasPrefix(a, pre) {
			continue
		}
		u, err := url.Parse(a)
		if err != nil {
			continue
		}
		link := "/api/v1/music/genre-art/" + g.Slug + "?v=" + url.QueryEscape(u.Query().Get("v"))
		if i > 0 {
			link += "&n=" + strconv.Itoa(i)
		}
		out = append(out, link)
	}
	return out
}

// genreArtHandlers serves the genre tile pictures; the metadata call is
// read per request so a later change to it is seen.
func (h *Handlers) genreArtHandlers() GenreArtHandlers {
	return GenreArtHandlers{
		Tile: func(ctx context.Context, slug, v string) ([]byte, error) {
			if h.MusicGenreTile == nil {
				return nil, metadata.ErrNotFound
			}
			return h.MusicGenreTile(ctx, slug, v)
		},
		KidMax: h.KidMax,
		Cache:  &h.genreArt,
	}
}

// musicGenrePage answers GET /api/v1/music/genre/{slug}?region=.
func (h *Handlers) musicGenrePage(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if !musicSlugRe.MatchString(slug) {
		webserver.Fail(w, http.StatusBadRequest, "unknown genre")
		return
	}
	// Music has no ratings, so a Kid limit shows none of it.
	if h.KidMax(r) != "" {
		webserver.WriteJSON(w, http.StatusOK, map[string]any{"kid": true, "v2": true, "genres": []string{}, "genre": slug, "hero": nil, "rows": []MusicRow{}})
		return
	}
	// The row template engine serves the genres it has a key for; others
	// keep this page.
	if h.musicTplOn() {
		if !h.musicBrowseGate(w, r, map[string]any{"kid": true, "v2": true, "genres": []string{}, "genre": slug, "hero": nil, "rows": []MusicRow{}}) {
			return
		}
		if h.serveTplPage(w, r, "genre", slug) {
			return
		}
	}
	ctx := r.Context()
	now := time.Now().UTC()
	member := h.Person(r).UserID
	region := strings.ToUpper(r.URL.Query().Get("region"))
	if region == "" || region == "GLOBAL" || !isCountry(region) {
		region = "global"
	}
	var tile *metadata.MusicGenre
	for _, g := range h.musicGenreTiles(ctx, region, now) {
		if g.Slug == slug {
			g := g
			tile = &g
			break
		}
	}
	if tile == nil {
		webserver.Fail(w, http.StatusNotFound, "no such genre in this region's charts")
		return
	}
	lib, err := h.musicLibrary(ctx, now)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	acts, err := h.musicActsOf(ctx, member)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	hero, alts, rows := h.buildMusicGenre(ctx, member, region, *tile, lib, acts, now)
	for i := range rows {
		for j := range rows[i].Items {
			rows[i].Items[j] = musicCardState(rows[i].Items[j], lib, acts)
		}
	}
	if hero != nil {
		hv := musicCardState(*hero, lib, acts)
		hero = &hv
	}
	for i := range alts {
		alts[i] = musicCardState(alts[i], lib, acts)
	}
	// Every listed artist and album gets its pictures and art warmed, so the
	// artist page opened from here has its wide photo and logo at once.
	var names []nameKey
	note := func(c MusicCard) {
		if c.Kind == "album" && c.MBID != "" {
			names = append(names, nameKey{"album", c.MBID, c.Name, c.Artist, ""})
		}
		if c.ArtistMBID != "" {
			names = append(names, nameKey{"artist", c.ArtistMBID, c.Artist, "", ""})
		} else if c.Kind == "artist" && c.MBID != "" {
			names = append(names, nameKey{"artist", c.MBID, c.Name, "", ""})
		}
	}
	if hero != nil {
		note(*hero)
	}
	for _, c := range alts {
		note(c)
	}
	for _, row := range rows {
		for _, c := range row.Items {
			note(c)
		}
	}
	h.rememberMusicNames(names)
	hue := tile.Hue
	h.ExploreCaching(w, r, "private, no-cache")
	h.WriteMusicJSON(w, r, map[string]any{"v2": true, "kid": false, "cold": false, "genres": []string{}, "genre": slug, "region": region, "regions": []string{},
		"tile": MusicCard{Key: "genre:" + slug, Kind: "genre", MBID: slug, Name: tile.Name, Hue: &hue, Image: musicGenreArt(*tile), Images: musicGenreArts(*tile), Genres: []string{slug}},
		"hero": hero, "hero_alts": alts, "rows": rows})
}

// musicCardState sets a card's live state chip (layer 3).
func musicCardState(c MusicCard, lib *musicLib, acts *musicActs) MusicCard {
	c.State = ""
	if c.MBID != "" && c.Kind == "album" {
		c.State = acts.albumState[c.MBID]
	}
	if c.State == "" && c.ArtistMBID != "" {
		c.State = acts.artistState[c.ArtistMBID]
	}
	if c.Kind == "album" && c.PartID != 0 && lib.hadParts[c.PartID] {
		c.State = "have"
	}
	return c
}

// musicGenreChart reads one genre's chart (layer 1, 6 hours).
func (h *Handlers) musicGenreChart(ctx context.Context, region, kind, slug string, now time.Time) []metadata.MusicChartItem {
	if h.MusicCharts == nil {
		return nil
	}
	key := "chart:" + region + ":" + kind + ":" + slug
	if v, ok := h.exploreCache.cand.Get(key, now); ok {
		return v.([]metadata.MusicChartItem)
	}
	c, err := h.MusicCharts(ctx, region, kind, slug)
	if err != nil {
		return nil
	}
	h.exploreCache.cand.Put(key, c.Items, spread(musicChartTTL, 0.25), now)
	return c.Items
}

func (h *Handlers) buildMusicGenre(ctx context.Context, member int64, region string, g metadata.MusicGenre, lib *musicLib, acts *musicActs, now time.Time) (*MusicCard, []MusicCard, []MusicRow) {
	albums := h.musicGenreChart(ctx, region, "albums", g.Slug, now)
	songs := h.musicGenreChart(ctx, region, "songs", g.Slug, now)
	artistOf := map[int64]musicLibArtist{}
	for _, ar := range lib.artists {
		artistOf[ar.id] = ar
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
	albumCard := func(al musicLibAlbum) MusicCard {
		ar := artistOf[al.titleID]
		c := MusicCard{Kind: "album", MBID: al.mbid, Name: al.name, Artist: ar.name, ArtistMBID: ar.mbid, TitleID: ar.id, PartID: al.partID, Date: al.date, Genres: []string{g.Slug}}
		c.Key = "album:" + al.mbid
		if al.mbid == "" {
			c.Key = "part:" + strconv.FormatInt(al.partID, 10)
		} else {
			c.Image = "/api/v1/music-trending/art/album/" + al.mbid
		}
		return c
	}
	artistCard := func(name, mbid string, id int64, image string) MusicCard {
		c := MusicCard{Key: "artist:" + mbid, Kind: "artist", MBID: mbid, Name: name, ArtistMBID: mbid, TitleID: id, Genres: []string{g.Slug}}
		c.Image = image
		if c.Image == "" && mbid != "" {
			c.Image = "/api/v1/music-trending/art/artist/" + mbid
		}
		return c
	}

	// Affinity of each Library artist, as on Explore.
	aff := map[int64]float64{}
	for _, ar := range lib.artists {
		x := musicscore.Acts{HaveAny: ar.hadAny, AlbumsHad: ar.had}
		for u := range acts.follows[ar.id] {
			x.Follows = x.Follows || u == member
			x.OtherFollows = x.OtherFollows || u != member
		}
		for u := range acts.asked[ar.mbid] {
			x.Asked = x.Asked || u == member
			x.OtherAsked = x.OtherAsked || u != member
		}
		aff[ar.id] = musicscore.Affinity(x)
	}
	// Library artists in the genre: a MusicBrainz genre that matches, or a
	// charted album or song in it.
	inGenre := map[int64]bool{}
	var ids []string
	for _, ar := range lib.artists {
		if ar.mbid != "" {
			ids = append(ids, ar.mbid)
		}
	}
	known := h.genresOf(ids)
	for _, ar := range lib.artists {
		for _, mg := range known[ar.mbid] {
			if genreMatch(musicSlug(mg), g.Slug) {
				inGenre[ar.id] = true
			}
		}
	}
	for _, it := range append(append([]metadata.MusicChartItem{}, albums...), songs...) {
		if i, ok := lib.byMBID[strings.ToLower(it.ArtistMBID)]; ok && it.ArtistMBID != "" {
			inGenre[lib.artists[i].id] = true
		}
	}

	seen := map[string]bool{}
	var rows []MusicRow
	add := func(row MusicRow, cards []MusicCard) {
		for _, c := range cards {
			if len(row.Items) >= musicscore.RowMax {
				break
			}
			if seen[c.Key] || dup(row.Items, c.Key) {
				continue
			}
			row.Items = append(row.Items, c)
		}
		// A row needs enough items to look full; a thin one is left out and
		// its items stay free for the rows below.
		if len(row.Items) < genreRowMin {
			return
		}
		for _, c := range row.Items {
			seen[c.Key] = true
		}
		rows = append(rows, row)
	}
	type scored struct {
		c MusicCard
		s float64
	}
	byScore := func(l []scored) []MusicCard {
		sort.SliceStable(l, func(i, j int) bool { return l[i].s > l[j].s })
		out := make([]MusicCard, len(l))
		for i, x := range l {
			out[i] = x.c
		}
		return out
	}

	// Spotlight: the top ten by F14 · P, one per artist, as the page's hero
	// (the same count as the template pages); no row repeats the first.
	var spot []scored
	for _, it := range albums {
		if it.MBID == "" {
			continue
		}
		if s := it.F14 * it.P; s > 0 {
			c := chartCard(it)
			c.ArtistImage = heroArtistImage(c.ArtistMBID)
			spot = append(spot, scored{c, s})
		}
	}
	if len(spot) == 0 {
		for _, it := range albums {
			if it.MBID != "" {
				c := chartCard(it)
				c.ArtistImage = heroArtistImage(c.ArtistMBID)
				spot = append(spot, scored{c, it.P})
			}
		}
	}
	var hero *MusicCard
	var alts []MusicCard
	var top []MusicCard
	artists := map[string]bool{}
	for _, c := range byScore(spot) {
		if len(top) < rowtpl.SpotlightMax && !artists[c.ArtistMBID] {
			artists[c.ArtistMBID] = true
			top = append(top, c)
		}
	}
	if len(top) > 0 {
		hero, alts = &top[0], append([]MusicCard{}, top[1:]...)
		seen[top[0].Key] = true
	}

	// New in {genre}: charted, out in the last 60 days, 0.6·P + 0.4·F14.
	{
		var l []scored
		for _, it := range albums {
			if it.F60 > 0 {
				l = append(l, scored{chartCard(it), 0.6*it.P + 0.4*it.F14})
			}
		}
		add(MusicRow{ID: "genre_new", Type: "genre_new", Title: "New in " + g.Name, Reason: "Charting albums out in the last 60 days", Shape: "square"}, byScore(l))
	}
	// Top albums: the genre chart, in order.
	{
		var cards []MusicCard
		for _, it := range albums {
			cards = append(cards, chartCard(it))
		}
		reason := "This week's album chart, " + g.Name + " only"
		if region != "global" {
			reason = "This week's album chart in " + region + ", " + g.Name + " only"
		}
		add(MusicRow{ID: "genre_top", Type: "top_albums", Title: "Top " + g.Name + " albums", Reason: reason, Shape: "square"}, cards)
	}
	// World-list rows (music_lists_v1): the tile's editorial lists, and the
	// region's charts filtered to the genre by the artists' genres.
	shelves := h.musicShelvesOf(ctx, region, now)
	if shelves != nil {
		lt, hasTile := listTileOf(g.Slug)
		name := g.Name
		place := listPlace(shelves.Chart)
		if hasTile {
			var picks, artists []MusicCard
			for _, it := range shelves.Blocks["tile:"+lt.Key] {
				c := listCard(it)
				picks = append(picks, c)
				ar := strings.ToLower(it.ArtistMBID)
				if it.Kind == "artist" {
					ar = strings.ToLower(it.MBID)
				}
				if _, have := lib.byMBID[ar]; !have && metadata.ValidMBID(ar) {
					artists = append(artists, artistCard(it.Artist, ar, 0, ""))
				}
			}
			add(MusicRow{ID: "genre_picks", Type: "genre_picks", Title: name + " picks", Reason: "Picked from this week's " + lt.Name + " lists", Shape: "square"}, picks)
			add(MusicRow{ID: "genre_to_know", Type: "genre_artists", Title: "Artists to know in " + name, Reason: "Artists on this week's " + lt.Name + " lists, not in your Library", Shape: "round"}, artists)
		}
		var ids []string
		for _, b := range []string{"top_now", "albums_week", "climbing", "new_entries", "new_friday", "top_world", "albums_world", "big_new_albums"} {
			for _, it := range shelves.Blocks[b] {
				ids = append(ids, strings.ToLower(it.ArtistMBID), strings.ToLower(it.MBID))
			}
		}
		gs := h.genresOf(ids)
		inG := func(it metadata.MusicShelfItem) bool {
			id := strings.ToLower(it.ArtistMBID)
			if it.Kind == "artist" {
				id = strings.ToLower(it.MBID)
			}
			for _, mg := range gs[id] {
				sl := musicSlug(mg)
				if genreMatch(sl, g.Slug) {
					return true
				}
				for _, ts := range lt.Slugs {
					if genreMatch(sl, ts) {
						return true
					}
				}
			}
			return false
		}
		from := func(blocks ...string) []MusicCard {
			var out []MusicCard
			for _, b := range blocks {
				for _, it := range shelves.Blocks[b] {
					if inG(it) && metadata.ValidMBID(strings.ToLower(it.MBID)) {
						out = append(out, listCard(it))
					}
				}
			}
			return out
		}
		add(MusicRow{ID: "genre_rising", Type: "genre_rising", Title: "Rising in " + name, Reason: "Climbing the charts " + place + " this week, " + name + " only", Shape: "square"}, from("climbing", "new_entries"))
		add(MusicRow{ID: "genre_charts", Type: "genre_charts", Title: name + " on the charts", Reason: "On the charts " + place + " today, " + name + " only", Shape: "square"}, from("top_now", "albums_week", "big_new_albums"))
		add(MusicRow{ID: "genre_friday", Type: "genre_friday", Title: "Out this Friday in " + name, Reason: "New albums and EPs from the last two weeks, " + name + " only", Shape: "square"}, from("new_friday"))
		add(MusicRow{ID: "genre_world_lists", Type: "genre_world_lists", Title: name + " around the world", Reason: "On the charts worldwide today, " + name + " only", Shape: "square"}, from("top_world", "albums_world"))
	}
	// Your artists in {genre}: circles by affinity.
	var mine []musicLibArtist
	for _, ar := range lib.artists {
		if inGenre[ar.id] {
			mine = append(mine, ar)
		}
	}
	sort.SliceStable(mine, func(i, j int) bool {
		if aff[mine[i].id] != aff[mine[j].id] {
			return aff[mine[i].id] > aff[mine[j].id]
		}
		return strings.ToLower(mine[i].name) < strings.ToLower(mine[j].name)
	})
	{
		var cards []MusicCard
		for _, ar := range mine {
			cards = append(cards, artistCard(ar.name, ar.mbid, ar.id, ar.image))
		}
		add(MusicRow{ID: "genre_yours", Type: "genre_yours", Title: "Your artists in " + g.Name, Reason: "Artists in your Library tagged " + g.Name + " or charting in it", Shape: "round"}, cards)
	}
	// Missing from them: 0.5·A + 0.3·P + 0.2·latest, at most 2 per artist.
	{
		chartP := map[string]float64{}
		for _, it := range albums {
			if it.MBID != "" {
				chartP[strings.ToLower(it.MBID)] = it.P
			}
		}
		var list []musicLibAlbum
		for _, al := range lib.albums {
			ar := artistOf[al.titleID]
			if !inGenre[ar.id] || !ar.hadAny || al.kind != "album" || al.secondary || al.date == "" || al.date > dayAt(now, 0) || lib.hadParts[al.partID] || acts.albumState[al.mbid] != "" {
				continue
			}
			list = append(list, al)
		}
		// latest: 1 for the artist's newest missing album, falling with age order.
		sort.SliceStable(list, func(i, j int) bool { return list[i].date > list[j].date })
		perArtist := map[int64]int{}
		var l []scored
		for _, al := range list {
			n := perArtist[al.titleID]
			if n >= 2 {
				continue
			}
			perArtist[al.titleID] = n + 1
			latest := 1.0 / float64(n+1)
			l = append(l, scored{albumCard(al), 0.5*aff[al.titleID] + 0.3*chartP[al.mbid] + 0.2*latest})
		}
		add(MusicRow{ID: "genre_missing", Type: "missing", Title: "Missing from them", Reason: "Studio albums by your " + g.Name + " artists, not in your Library or asked for yet", Shape: "square"}, byScore(l))
	}
	// Popular artists: charting in the genre, not in the Library, by P.
	{
		best := map[string]scored{}
		var order []string
		for _, it := range append(append([]metadata.MusicChartItem{}, albums...), songs...) {
			mb := strings.ToLower(it.ArtistMBID)
			if mb == "" {
				continue
			}
			if _, have := lib.byMBID[mb]; have {
				continue
			}
			if b, ok := best[mb]; !ok || it.P > b.s {
				if !ok {
					order = append(order, mb)
				}
				best[mb] = scored{artistCard(it.Artist, mb, 0, ""), it.P}
			}
		}
		var l []scored
		for _, mb := range order {
			l = append(l, best[mb])
		}
		add(MusicRow{ID: "genre_artists", Type: "genre_artists", Title: "Popular " + g.Name + " artists", Reason: "Charting in " + g.Name + " and not in your Library", Shape: "round"}, byScore(l))
	}
	// Worldwide and songs rows: the global genre chart (other regions) and
	// the artists behind the genre's top songs.
	if region != "global" {
		var cards []MusicCard
		for _, it := range h.musicGenreChart(ctx, "global", "albums", g.Slug, now) {
			if it.MBID != "" {
				cards = append(cards, chartCard(it))
			}
		}
		add(MusicRow{ID: "genre_worldwide", Type: "top_albums", Title: "Top " + g.Name + " albums worldwide", Reason: "This week's album chart worldwide, " + g.Name + " only", Shape: "square"}, cards)
	}
	{
		var cards []MusicCard
		for _, it := range songs {
			if id := strings.ToLower(it.ArtistMBID); metadata.ValidMBID(id) {
				cards = append(cards, artistCard(it.Artist, id, 0, ""))
			}
		}
		add(MusicRow{ID: "genre_songs", Type: "genre_artists", Title: "Behind the top " + g.Name + " songs", Reason: "The artists of this week's most-played " + g.Name + " songs", Shape: "round"}, cards)
	}
	// Coming soon: your genre artists' albums out within 90 days.
	{
		var list []musicLibAlbum
		for _, al := range lib.albums {
			if inGenre[al.titleID] && !al.secondary && (al.kind == "album" || al.kind == "ep" || al.kind == "single") && al.date > dayAt(now, 0) && al.date <= dayAt(now, 90) {
				list = append(list, al)
			}
		}
		sort.SliceStable(list, func(i, j int) bool {
			if list[i].date != list[j].date {
				return list[i].date < list[j].date
			}
			return list[i].partID < list[j].partID
		})
		var cards []MusicCard
		for _, al := range list {
			c := albumCard(al)
			c.Upcoming = true
			cards = append(cards, c)
		}
		add(MusicRow{ID: "genre_soon", Type: "coming_soon", Title: "Coming soon", Reason: "Out within 90 days, from your " + g.Name + " artists", Shape: "square"}, cards)
	}
	if rows == nil {
		rows = []MusicRow{}
	}
	if alts == nil {
		alts = []MusicCard{}
	}
	return hero, alts, rows
}

// genreRowMin is the fewest items a genre page row shows with.
const genreRowMin = 6

func dup(items []MusicCard, key string) bool {
	for _, c := range items {
		if c.Key == key {
			return true
		}
	}
	return false
}
