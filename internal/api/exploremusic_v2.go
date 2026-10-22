package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs/internal/api/musicscore"
	"github.com/CoderLuii/holyarrs/internal/metadata"
)

// Explore → Music, flag music_rows_v2 (plan 2026-10-22, Phase 1): eight
// rows with plain sort keys, one shared dedup, and three cache layers.
// Layer 1 keeps chart and library candidates, layer 2 the member's row and
// item order for the day, layer 3 (state chips and the asks row) is read
// live on every answer, so an ask moves a chip and never the order.

const (
	musicChartTTL   = 6 * time.Hour
	musicLibraryTTL = 5 * time.Minute
)

// musicV2State is the flag's cache layers.
type musicV2State struct {
	cand musicscore.Candidates
	days musicscore.Days
	// seedLog keeps each member's Since you have seeds per day (v3).
	seedLog musicSeedLog
	// now is the clock (tests); nil is time.Now.
	now func() time.Time
}

// musicV2Order is layer 2: one member's answer for one day, before state.
type musicV2Order struct {
	Hero    *MusicCard
	Alts    []MusicCard
	Rows    []MusicRow
	Regions []string
	Cold    bool
}

// musicLib is the library snapshot the rows are built from (layer 1).
type musicLib struct {
	artists  []musicLibArtist
	byMBID   map[string]int // artist mbid → index
	albums   []musicLibAlbum
	hadParts map[int64]bool
}

type musicLibArtist struct {
	id     int64
	name   string
	mbid   string
	image  string
	hadAny bool
	had    int
}

type musicLibAlbum struct {
	partID, titleID int64
	name, mbid      string
	kind            string
	secondary       bool
	date            string
}

func (a *API) musicLibrary(ctx context.Context, now time.Time) (*musicLib, error) {
	if v, ok := a.musicv2.cand.Get("lib:snapshot", now); ok {
		return v.(*musicLib), nil
	}
	db := a.s.Store.Read()
	lib := &musicLib{byMBID: map[string]int{}, hadParts: map[int64]bool{}}
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT i.part_id FROM items i JOIN slots s ON s.item_id = i.id
		WHERE i.kind = 'track' AND s.file_id IS NOT NULL AND i.part_id IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			lib.hadParts[id] = true
		}
	}
	rows.Close()
	ps, err := db.QueryContext(ctx, `SELECT p.id, p.title_id, p.name, coalesce(json_extract(p.source_ids, '$.musicbrainz'), ''), lower(coalesce(p.release_type, '')),
		p.secondary_types != '[]', coalesce(substr(p.release_date, 1, 10), '')
		FROM parts p JOIN titles t ON t.id = p.title_id
		WHERE t.kind = 'artist' AND t.state != 'removed' AND p.kind = 'album' AND p.dropped_at IS NULL ORDER BY p.id`)
	if err != nil {
		return nil, err
	}
	had := map[int64]int{}
	for ps.Next() {
		var al musicLibAlbum
		if ps.Scan(&al.partID, &al.titleID, &al.name, &al.mbid, &al.kind, &al.secondary, &al.date) == nil {
			al.mbid = strings.ToLower(al.mbid)
			lib.albums = append(lib.albums, al)
			if lib.hadParts[al.partID] {
				had[al.titleID]++
			}
		}
	}
	ps.Close()
	ts, err := db.QueryContext(ctx, `SELECT id, name, lower(coalesce(json_extract(source_ids, '$.musicbrainz'), '')), artwork FROM titles WHERE kind = 'artist' AND state != 'removed' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	for ts.Next() {
		var ar musicLibArtist
		var art string
		if ts.Scan(&ar.id, &ar.name, &ar.mbid, &art) == nil {
			ar.had, ar.hadAny = had[ar.id], had[ar.id] > 0
			if l := artworkLinksJSON(art); len(l) > 0 && strings.HasPrefix(l[0], "/api/v1/artwork/") {
				ar.image = l[0]
			}
			if ar.mbid != "" {
				lib.byMBID[ar.mbid] = len(lib.artists)
			}
			lib.artists = append(lib.artists, ar)
		}
	}
	ts.Close()
	a.musicv2.cand.Put("lib:snapshot", lib, musicLibraryTTL, now)
	return lib, nil
}

// musicActs reads the household's follows and music asks, live.
type musicActs struct {
	follows map[int64]map[int64]bool // title id → user ids
	// asked: artist mbid → user ids on an artist ask (any albums).
	asked map[string]map[int64]bool
	// albumState: release group mbid → asked or approved (household).
	albumState map[string]string
	// artistState: artist mbid → asked or approved, for whole-artist asks.
	artistState map[string]string
	// mine: the member's open album asks, newest first.
	mine []musicAsk
}

type musicAsk struct {
	artist, artistMBID string
	album, albumMBID   string
	state, at          string
}

func (a *API) musicActsOf(ctx context.Context, member int64) (*musicActs, error) {
	db := a.s.Store.Read()
	x := &musicActs{follows: map[int64]map[int64]bool{}, asked: map[string]map[int64]bool{}, albumState: map[string]string{}, artistState: map[string]string{}}
	rows, err := db.QueryContext(ctx, `SELECT user_id, title_id FROM follows`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var u, t int64
		if rows.Scan(&u, &t) == nil {
			if x.follows[t] == nil {
				x.follows[t] = map[int64]bool{}
			}
			x.follows[t][u] = true
		}
	}
	rows.Close()
	rows, err = db.QueryContext(ctx, `SELECT a.id, a.name, lower(coalesce(json_extract(a.source_ids, '$.musicbrainz'), '')), a.albums, a.state, a.asked_at,
		coalesce((SELECT group_concat(user_id) FROM ask_people p WHERE p.ask_id = a.id), ''), a.asked_by
		FROM asks a WHERE a.kind = 'artist' AND a.state IN ('asked', 'approved') ORDER BY a.asked_at DESC, a.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, by int64
		var name, mbid, albums, state, at, people string
		if rows.Scan(&id, &name, &mbid, &albums, &state, &at, &people, &by) != nil {
			continue
		}
		users := map[int64]bool{by: true}
		for _, p := range strings.Split(people, ",") {
			if u, err := strconv.ParseInt(p, 10, 64); err == nil {
				users[u] = true
			}
		}
		if mbid != "" {
			if x.asked[mbid] == nil {
				x.asked[mbid] = map[int64]bool{}
			}
			for u := range users {
				x.asked[mbid][u] = true
			}
		}
		var list []struct {
			MBID string `json:"mbid"`
			Name string `json:"name"`
		}
		_ = json.Unmarshal([]byte(albums), &list)
		if len(list) == 0 && mbid != "" {
			x.artistState[mbid] = state
		}
		for _, al := range list {
			x.albumState[strings.ToLower(al.MBID)] = state
		}
		if users[member] {
			if len(list) == 0 {
				x.mine = append(x.mine, musicAsk{artist: name, artistMBID: mbid, state: state, at: at})
			}
			for _, al := range list {
				x.mine = append(x.mine, musicAsk{artist: name, artistMBID: mbid, album: al.Name, albumMBID: strings.ToLower(al.MBID), state: state, at: at})
			}
		}
	}
	return x, rows.Err()
}

func artworkLinksJSON(raw string) []string {
	var v any
	if json.Unmarshal([]byte(raw), &v) != nil {
		return nil
	}
	var out []string
	var walk func(any)
	walk = func(x any) {
		switch t := x.(type) {
		case string:
			out = append(out, t)
		case []any:
			for _, e := range t {
				walk(e)
			}
		case map[string]any:
			keys := make([]string, 0, len(t))
			for k := range t {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				walk(t[k])
			}
		}
	}
	walk(v)
	return out
}

func (a *API) musicChartOf(ctx context.Context, region, kind string, now time.Time) []metadata.MusicChartItem {
	if a.s.MusicCharts == nil {
		return nil
	}
	key := "chart:" + region + ":" + kind
	if v, ok := a.musicv2.cand.Get(key, now); ok {
		return v.([]metadata.MusicChartItem)
	}
	c, err := a.s.MusicCharts(ctx, region, kind, "")
	if err != nil {
		return nil
	}
	a.musicv2.cand.Put(key, c.Items, musicChartTTL, now)
	return c.Items
}

func (a *API) musicGenreTiles(ctx context.Context, region string, now time.Time) []metadata.MusicGenre {
	if a.s.MusicGenres == nil {
		return nil
	}
	key := "genres:" + region
	if v, ok := a.musicv2.cand.Get(key, now); ok {
		return v.([]metadata.MusicGenre)
	}
	g, err := a.s.MusicGenres(ctx, region)
	if err != nil {
		return nil
	}
	a.musicv2.cand.Put(key, g.Genres, musicChartTTL, now)
	return g.Genres
}

func dayAt(now time.Time, d int) string { return now.UTC().AddDate(0, 0, d).Format("2006-01-02") }

// exploreMusicV2 answers GET /api/v1/explore/music with flag music_rows_v2.
func (a *API) exploreMusicV2(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	now := time.Now().UTC()
	if a.musicv2.now != nil {
		now = a.musicv2.now().UTC()
	}
	member := a.person(r).UserID
	region := strings.ToUpper(r.URL.Query().Get("region"))
	if region == "" || region == "GLOBAL" || !isCountry(region) {
		region = "global"
	}
	lib, err := a.musicLibrary(ctx, now)
	if err != nil {
		internal(w, err)
		return
	}
	acts, err := a.musicActsOf(ctx, member)
	if err != nil {
		internal(w, err)
		return
	}
	key := musicscore.DayKey{Member: member, Date: musicscore.Day(now), View: region}
	var ord *musicV2Order
	if v, ok := a.musicv2.days.Get(key); ok {
		ord = v.(*musicV2Order)
	} else {
		if a.s.MusicRowsV3 {
			ord = a.buildMusicV3(ctx, member, region, lib, acts, now)
		} else {
			ord = a.buildMusicV2(ctx, member, region, lib, acts, now)
		}
		a.musicv2.days.Put(key, ord)
	}
	// Layer 3: state chips and the member's asks, read live.
	state := func(c MusicCard) MusicCard {
		c.State = ""
		if c.MBID != "" && c.Kind == "album" {
			if s := acts.albumState[c.MBID]; s != "" {
				c.State = s
			}
		}
		if c.State == "" && c.ArtistMBID != "" {
			if s := acts.artistState[c.ArtistMBID]; s != "" {
				c.State = s
			}
		}
		if c.Kind == "album" && c.PartID != 0 && lib.hadParts[c.PartID] {
			c.State = "have"
		}
		return c
	}
	out := make([]MusicRow, 0, len(ord.Rows)+1)
	anchors := 0
	for _, row := range ord.Rows {
		if row.ID == "new_from" || row.ID == "genres" {
			anchors++
		}
	}
	asksRow := MusicRow{ID: "your_asks", Type: "your_asks", Title: "Your asks in progress", Reason: "Albums you asked for, newest first", Shape: "square"}
	for _, m := range acts.mine {
		c := MusicCard{Kind: "album", MBID: m.albumMBID, Name: m.album, Artist: m.artist, ArtistMBID: m.artistMBID, State: m.state, Date: dayOfText(m.at), Genres: []string{}}
		c.Key = "album:" + m.albumMBID
		if m.albumMBID == "" {
			c.Kind, c.MBID, c.Name, c.Key = "artist", m.artistMBID, m.artist, "artist:"+m.artistMBID
		} else {
			c.Image = "/api/v1/music-trending/art/album/" + m.albumMBID
		}
		if len(asksRow.Items) < musicscore.RowMax {
			asksRow.Items = append(asksRow.Items, c)
		}
	}
	for i, row := range ord.Rows {
		cp := row
		cp.Items = make([]MusicCard, len(row.Items))
		for j, c := range row.Items {
			cp.Items[j] = state(c)
		}
		out = append(out, cp)
		if i == anchors-1 && len(asksRow.Items) > 0 {
			out = append(out, asksRow)
		}
	}
	if anchors == 0 && len(asksRow.Items) > 0 {
		out = append([]MusicRow{asksRow}, out...)
	}
	if a.s.MusicRowsV3 {
		sinkAsked(out)
	}
	var hero *MusicCard
	if ord.Hero != nil {
		h := state(*ord.Hero)
		hero = &h
	}
	alts := make([]MusicCard, 0, len(ord.Alts))
	for _, c := range ord.Alts {
		alts = append(alts, state(c))
	}
	regions := ord.Regions
	if regions == nil {
		regions = []string{}
	}
	a.exploreCaching(w, r, "private, no-cache")
	writeJSON(w, http.StatusOK, map[string]any{"v2": true, "kid": false, "cold": ord.Cold, "genres": []string{}, "genre": "", "region": region, "regions": regions, "hero": hero, "hero_alts": alts, "rows": out})
}

func dayOfText(s string) string {
	if len(s) >= 10 {
		return s[:10]
	}
	return ""
}

// buildMusicV2 builds layer 2: candidates, row order, then the shared fill.
func (a *API) buildMusicV2(ctx context.Context, member int64, region string, lib *musicLib, acts *musicActs, now time.Time) *musicV2Order {
	ord := &musicV2Order{}
	// A per library artist.
	aff := map[int64]float64{}
	ownFollows, haveAny := 0, false
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
		aff[ar.id] = musicscore.Affinity(x)
		haveAny = haveAny || ar.hadAny
	}
	// Personal rows unlock at 3 follows or 1 have.
	ord.Cold = ownFollows < 3 && !haveAny
	artistOf := map[int64]musicLibArtist{}
	for _, ar := range lib.artists {
		artistOf[ar.id] = ar
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
	item := func(c MusicCard, score float64) musicscore.Item {
		return musicscore.Item{Key: c.Key, Album: c.MBID, FoldKey: fullAlbumKey(c.Name) + "|" + fullAlbumKey(c.Artist), Artist: strings.ToLower(c.ArtistMBID + "|" + c.Artist), Score: score}
	}
	type built struct {
		row   musicscore.Row
		cards map[string]MusicCard
		meta  MusicRow
	}
	var all []built
	addRow := func(meta MusicRow, row musicscore.Row, cards []MusicCard, scores []float64) {
		b := built{row: row, cards: map[string]MusicCard{}, meta: meta}
		b.row.ID = meta.ID
		for i, c := range cards {
			b.row.Items = append(b.row.Items, item(c, scores[i]))
			b.cards[c.Key] = c
		}
		all = append(all, b)
	}
	studio := func(al musicLibAlbum) bool {
		return !al.secondary && (al.kind == "album" || al.kind == "ep" || al.kind == "single")
	}
	from, to := dayAt(now, -30), dayAt(now, 7)

	// Rows 1 and 2: new from artists with A > 0, −30..+7 days, newest first.
	var fresh []MusicCard
	for _, al := range lib.albums {
		if aff[al.titleID] > 0 && studio(al) && al.date >= from && al.date <= to {
			fresh = append(fresh, albumCard(al))
		}
	}
	sort.SliceStable(fresh, func(i, j int) bool {
		if fresh[i].Date != fresh[j].Date {
			return fresh[i].Date > fresh[j].Date
		}
		return fresh[i].Key < fresh[j].Key
	})
	for i := range fresh {
		fresh[i].Upcoming = fresh[i].Date > dayAt(now, 0)
	}
	seen := map[string]bool{}
	albums := a.musicChartOf(ctx, region, "albums", now)
	chartCard := func(it metadata.MusicChartItem) MusicCard {
		c := MusicCard{Kind: "album", MBID: strings.ToLower(it.MBID), Name: it.Title, Artist: it.Artist, ArtistMBID: strings.ToLower(it.ArtistMBID), Date: it.Date, Rank: it.Rank, Explicit: it.Explicit, Genres: append([]string{}, it.Genres...)}
		c.Key = "album:" + c.MBID
		if c.MBID == "" {
			c.Key = "chart:" + strings.ToLower(it.Artist+"|"+it.Title)
		}
		if c.MBID != "" {
			c.Image = "/api/v1/music-trending/art/album/" + c.MBID
		} else {
			c.Image = it.Cover
		}
		if c.ArtistMBID != "" {
			if i, ok := lib.byMBID[c.ArtistMBID]; ok {
				c.TitleID = lib.artists[i].id
			}
		}
		return c
	}
	var heroes []MusicCard
	if !ord.Cold {
		for _, c := range fresh {
			if !c.Upcoming && len(heroes) < 6 {
				h := c
				h.ArtistImage = artistOf[c.TitleID].image
				heroes = append(heroes, h)
			}
		}
	}
	if len(heroes) == 0 {
		// The region's top three new albums, else its top three.
		var recent, top []MusicCard
		for _, it := range albums {
			c := chartCard(it)
			if c.MBID == "" {
				continue
			}
			c.ArtistImage = "/api/v1/music-trending/art/artist/" + c.ArtistMBID
			if it.Date >= from && len(recent) < 3 {
				recent = append(recent, c)
			}
			if len(top) < 3 {
				top = append(top, c)
			}
		}
		heroes = recent
		if len(heroes) == 0 {
			heroes = top
		}
	}
	if len(heroes) > 0 {
		ord.Hero, ord.Alts = &heroes[0], heroes[1:]
		seen[heroes[0].Key] = true
		if heroes[0].MBID != "" {
			seen["rg:"+heroes[0].MBID] = true
		}
	}
	if !ord.Cold {
		scores := make([]float64, len(fresh))
		for i, c := range fresh {
			scores[i] = musicscore.Freshness(c.Date, 14, now)
		}
		addRow(MusicRow{ID: "new_from", Type: "new_from", Title: "New from your artists", Reason: "Out in the last 30 days or this week, from artists you have or follow", Shape: "square"},
			musicscore.Row{Anchor: true, Prior: musicscore.PriorPersonal, ArtistRow: true}, fresh, scores)
	}

	// Row 4: genre tiles (grey art on the hue, or the flat hue).
	{
		var cards []MusicCard
		var scores []float64
		for _, g := range a.musicGenreTiles(ctx, region, now) {
			if len(cards) >= 12 {
				break
			}
			hue := g.Hue
			cards = append(cards, MusicCard{Key: "genre:" + g.Slug, Kind: "genre", MBID: g.Slug, Name: g.Name, Hue: &hue, Image: musicGenreArt(g), Genres: []string{g.Slug}})
			scores = append(scores, 1)
		}
		addRow(MusicRow{ID: "genres", Type: "genres", Title: "Genres", Reason: "The genres charting in your region", Shape: "tile"},
			musicscore.Row{Anchor: true, Prior: musicscore.PriorMixed, ArtistRow: true}, cards, scores)
	}

	// Row 5: songs out in the last 14 days, one per artist, chart order.
	{
		var cards []MusicCard
		var scores []float64
		per := map[string]bool{}
		for _, it := range a.musicChartOf(ctx, region, "songs", now) {
			if it.Date == "" || it.Date < dayAt(now, -14) || per[strings.ToLower(it.Artist)] {
				continue
			}
			per[strings.ToLower(it.Artist)] = true
			c := MusicCard{Kind: "song", Name: it.Title, Track: it.Title, Artist: it.Artist, ArtistMBID: strings.ToLower(it.ArtistMBID), Date: it.Date, Rank: it.Rank, Explicit: it.Explicit, Image: it.Cover, Genres: append([]string{}, it.Genres...)}
			c.Key = "song:" + strings.ToLower(it.Artist+"|"+it.Title)
			cards = append(cards, c)
			scores = append(scores, it.P)
		}
		addRow(MusicRow{ID: "fresh_songs", Type: "fresh_songs", Title: "Fresh songs", Reason: "New songs on the chart; each one asks for its album", Shape: "song"},
			musicscore.Row{Prior: musicscore.PriorChart}, cards, scores)
	}

	// Row 6: studio albums missing from artists you have, by have-count.
	if !ord.Cold {
		var cards []MusicCard
		var counts []float64
		var list []musicLibAlbum
		for _, al := range lib.albums {
			ar := artistOf[al.titleID]
			if !ar.hadAny || al.kind != "album" || al.secondary || al.date == "" || al.date > dayAt(now, 0) || lib.hadParts[al.partID] || acts.albumState[al.mbid] != "" {
				continue
			}
			list = append(list, al)
		}
		sort.SliceStable(list, func(i, j int) bool {
			hi, hj := artistOf[list[i].titleID].had, artistOf[list[j].titleID].had
			if hi != hj {
				return hi > hj
			}
			if list[i].date != list[j].date {
				return list[i].date > list[j].date
			}
			return list[i].partID < list[j].partID
		})
		for _, al := range list {
			cards = append(cards, albumCard(al))
			counts = append(counts, float64(artistOf[al.titleID].had))
		}
		addRow(MusicRow{ID: "missing", Type: "missing", Title: "Missing from your artists", Reason: "Studio albums by artists you have, not in your Library or asked for yet", Shape: "square"},
			musicscore.Row{Prior: musicscore.PriorPersonal}, cards, musicscore.Norm(counts))
	}

	// Row 7: the region's top albums, chart order.
	{
		var cards []MusicCard
		var scores []float64
		for _, it := range albums {
			cards = append(cards, chartCard(it))
			scores = append(scores, it.P)
		}
		title := "Top albums"
		if region != "global" {
			title = "Top albums in " + region
		}
		addRow(MusicRow{ID: "top_albums", Type: "top_albums", Title: title, Reason: chartReason(region, "Chart"), Shape: "square"},
			musicscore.Row{Prior: musicscore.PriorChart}, cards, scores)
	}

	// Row 8: coming within 90 days from artists in the Library, soonest first.
	{
		var list []musicLibAlbum
		for _, al := range lib.albums {
			if studio(al) && al.date > dayAt(now, 0) && al.date <= dayAt(now, 90) {
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
		var scores []float64
		for i, al := range list {
			c := albumCard(al)
			c.Upcoming = true
			cards = append(cards, c)
			scores = append(scores, 1-float64(i)/float64(len(list)))
		}
		addRow(MusicRow{ID: "coming_soon", Type: "coming_soon", Title: "Coming soon", Reason: "Out within 90 days, from artists in your Library", Shape: "square"},
			musicscore.Row{Prior: musicscore.PriorMixed}, cards, scores)
	}

	// Order (anchors first, then R), fill with one seen set, gate.
	byID := map[string]built{}
	var rows []musicscore.Row
	for _, b := range all {
		byID[b.meta.ID] = b
		rows = append(rows, b.row)
	}
	for _, r := range musicscore.Dedup(musicscore.Order(rows, member, musicscore.Day(now)), seen) {
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
