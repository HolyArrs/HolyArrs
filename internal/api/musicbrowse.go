package api

import (
	"context"
	"errors"
	"hash/fnv"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/music/rowtpl"
	musicscore "github.com/CoderLuii/holyarrs/internal/music/score"
	"github.com/CoderLuii/holyarrs/internal/parts"
)

// The row template engine (music_lists_v1, plan 2026-10-24-music-row-
// templates): Browse pages for genres, moods, seasons, decades and
// activities, filled from the server's catalog bundle.

// musicTplOn reports whether the engine serves music pages.
func (a *API) musicTplOn() bool {
	return a.s.MusicListsV1 && !a.s.MusicRowsTplOff && a.s.MusicPools != nil
}

const (
	tplBundleTTL = 6 * time.Hour
	tplKeepTTL   = 7 * 24 * time.Hour
	// tplFailTTL keeps a failed read (no copy to serve) for a few minutes,
	// so page views never turn into a download each.
	tplFailTTL = 5 * time.Minute
)

// tplFetch lets one bundle download run at a time; callers that wait read
// the cache it filled (a single flight without a new dependency). A waiting
// request whose own context ends gives up and answers without the bundle.
var tplFetch = make(chan struct{}, 1)

type tplBundle struct {
	b    *rowtpl.Bundle
	etag string
}

// musicBundle answers the catalog bundle: cached 6 hours, then re-read
// with its ETag; a failed read keeps the last good copy for a week.
func (a *API) musicBundle(ctx context.Context, now time.Time) *rowtpl.Bundle {
	if v, ok := a.musicv2.cand.Get("tpl:bundle", now); ok {
		return v.(*tplBundle).b
	}
	if _, failed := a.musicv2.cand.Get("tpl:fail", now); failed {
		return nil
	}
	select {
	case tplFetch <- struct{}{}:
	case <-ctx.Done():
		return nil
	}
	defer func() { <-tplFetch }()
	// Another request may have filled the cache while this one waited.
	if v, ok := a.musicv2.cand.Get("tpl:bundle", now); ok {
		return v.(*tplBundle).b
	}
	if _, failed := a.musicv2.cand.Get("tpl:fail", now); failed {
		return nil
	}
	var last *tplBundle
	if v, ok := a.musicv2.cand.Get("tpl:last", now); ok {
		last = v.(*tplBundle)
	}
	etag := ""
	if last != nil {
		etag = last.etag
	}
	// The download is the whole household's: it never ends with the asking
	// request, and has its own timeout.
	dctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	body, tag, err := a.s.MusicPools(dctx, etag)
	switch {
	case errors.Is(err, metadata.ErrNotModified) && last != nil:
		a.musicv2.cand.Put("tpl:bundle", last, tplBundleTTL, now)
		return last.b
	case err != nil:
		if last != nil {
			a.musicv2.cand.Put("tpl:bundle", last, 10*time.Minute, now)
			return last.b
		}
		// The download's own result decides (the caller cannot cancel it):
		// a failure, its own timeout included, is kept for minutes.
		a.musicv2.cand.Put("tpl:fail", true, tplFailTTL, now)
		return nil
	}
	b, err := rowtpl.ParseBundle(body)
	if err != nil {
		if last != nil {
			a.musicv2.cand.Put("tpl:bundle", last, 10*time.Minute, now)
			return last.b
		}
		a.musicv2.cand.Put("tpl:fail", true, tplFailTTL, now)
		return nil
	}
	x := &tplBundle{b: b, etag: tag}
	a.musicv2.cand.Put("tpl:bundle", x, tplBundleTTL, now)
	a.musicv2.cand.Put("tpl:last", x, tplKeepTTL, now)
	return b
}

// tplCountsMu lets one count run at a time (it builds every Browse page).
var tplCountsMu sync.Mutex

// tplCounts answers each Browse page's row count for a region, as a member
// with an empty library sees it: cached per region and bundle version for
// 6 hours, so tiles appear by themselves once their data grows.
func (a *API) tplCounts(ctx context.Context, b *rowtpl.Bundle, region string, now time.Time) map[string]int {
	key := "tpl:counts:" + strings.ToLower(region) + ":" + b.Version
	if v, ok := a.musicv2.cand.Get(key, now); ok {
		return v.(map[string]int)
	}
	tplCountsMu.Lock()
	defer tplCountsMu.Unlock()
	if v, ok := a.musicv2.cand.Get(key, now); ok {
		return v.(map[string]int)
	}
	counts := rowtpl.PageCounts(b, now, strings.ToLower(region), tplBlocks(a.musicShelvesOf(ctx, region, now)), rowtpl.BrowseKeys())
	a.musicv2.cand.Put(key, counts, tplBundleTTL, now)
	return counts
}

// tplVisible answers the Browse sections a member sees: tiles whose page
// has enough rows, seasons inside their window.
func (a *API) tplVisible(ctx context.Context, b *rowtpl.Bundle, region string, lib *rowtpl.Lib, now time.Time) []rowtpl.BrowseSection {
	r := strings.ToLower(region)
	return rowtpl.VisibleSections(rowtpl.BrowseSections(b, now, r, lib), a.tplCounts(ctx, b, region, now), now, r)
}

// tplKinds are the Browse page kinds the engine serves.
var tplKinds = map[string]bool{"genre": true, "mood": true, "season": true, "decade": true, "activity": true}

var tplKeyRe = regexp.MustCompile(`^[a-z0-9-]{1,40}$`)

// tplGenreAlias maps chart genre slugs (old genre page links) to our keys.
var tplGenreAlias = map[string]string{
	"hip-hop-rap": "hip-hop", "r-b-soul": "rnb", "r-and-b": "rnb", "rnb": "rnb", "dance": "dance", "electronic": "dance", "k-pop": "kpop",
	"children-s-music": "kids", "soundtrack": "soundtracks", "singer-songwriter": "folk", "christian-gospel": "christian", "christian": "christian",
	"gospel": "christian", "afrobeats": "afro", "african": "afro", "reggae": "reggae", "soul-funk": "soul-funk", "hard-rock": "rock", "indie-pop": "indie",
	"latin": "latin", "pop-latino": "latin", "urbano-latino": "latin", "musica-mexicana": "latin", "alternative": "alternative",
}

// tplKey answers the bundle key for a page kind and slug, if the bundle has it.
func tplKey(b *rowtpl.Bundle, kind, slug string) (string, bool) {
	if kind == "activity" && slug == "party" {
		kind = "mood"
	}
	if kind == "genre" {
		if k, ok := tplGenreAlias[slug]; ok {
			slug = k
		}
	}
	if kind == "decade" && len(slug) == 3 && strings.HasSuffix(slug, "s") {
		// "90s" → "1990s"
		if slug[0] >= '6' {
			slug = "19" + slug
		} else {
			slug = "20" + slug
		}
	}
	key := kind + ":" + slug
	_, ok := b.Names[key]
	return key, ok && len(b.Keys[key]) > 0
}

// musicTplLib turns the member's library and acts into what the engine
// reads: had and asked albums, artist affinity, similarity to the library,
// library genres and decade, and up to 3 seeds.
func (a *API) musicTplLib(ctx context.Context, member int64, lib *musicLib, acts *musicActs, b *rowtpl.Bundle, now time.Time) *rowtpl.Lib {
	l := &rowtpl.Lib{Albums: map[string]bool{}, Asked: map[string]bool{}, Artists: map[string]float64{}, Sim: map[string]float64{}}
	for _, ar := range lib.artists {
		if ar.mbid == "" {
			continue
		}
		x := musicscore.Acts{HaveAny: ar.hadAny, AlbumsHad: ar.had}
		for u := range acts.follows[ar.id] {
			x.Follows = x.Follows || u == member
			x.OtherFollows = x.OtherFollows || u != member
		}
		for u := range acts.asked[ar.mbid] {
			x.Asked = x.Asked || u == member
			x.OtherAsked = x.OtherAsked || u != member
		}
		if f := musicscore.Affinity(x); f > 0 {
			l.Artists[ar.mbid] = f
		}
	}
	for _, al := range lib.albums {
		if al.mbid != "" && lib.hadParts[al.partID] {
			l.Albums[al.mbid] = true
		}
	}
	for id, st := range acts.albumState {
		if st != "" {
			l.Asked[id] = true
		}
	}
	if len(l.Artists) == 0 {
		return l
	}
	type aa struct {
		id string
		a  float64
	}
	var top []aa
	for id, f := range l.Artists {
		top = append(top, aa{id, f})
	}
	sort.Slice(top, func(i, j int) bool {
		if top[i].a != top[j].a {
			return top[i].a > top[j].a
		}
		return top[i].id < top[j].id
	})
	var seedIDs []string
	for _, x := range top[:min(12, len(top))] {
		seedIDs = append(seedIDs, x.id)
	}
	sims := a.musicSimilarOf(ctx, seedIDs, now)
	for _, id := range seedIDs {
		for _, s := range sims[id] {
			if s.S > l.Sim[s.MBID] {
				l.Sim[s.MBID] = s.S
			}
		}
	}
	// Seeds: the day's 3 strongest artists that have similar artists.
	names := map[string]string{}
	for _, ar := range lib.artists {
		names[ar.mbid] = ar.name
	}
	for _, id := range seedIDs {
		if len(sims[id]) == 0 || len(l.Seeds) == 3 {
			continue
		}
		sim := map[string]float64{}
		for _, s := range sims[id] {
			sim[s.MBID] = s.S
		}
		l.Seeds = append(l.Seeds, rowtpl.Seed{ID: id, Name: names[id], Similar: sim})
	}
	l.Genres, l.Decade = rowtpl.LibraryKeys(b, l)
	return l
}

// tplCard is a bundle item as a Music card.
func tplCard(it rowtpl.Item) MusicCard {
	if it.Kind == "r" {
		return MusicCard{Key: "artist:" + it.ID, Kind: "artist", MBID: it.ID, Name: it.Title, Artist: it.Title, ArtistMBID: it.ID, Image: "/api/v1/music-trending/art/artist/" + it.ID, Genres: []string{}}
	}
	c := MusicCard{Key: "album:" + it.ID, Kind: "album", MBID: it.ID, Name: it.Title, Artist: it.Artist, ArtistMBID: it.ArtistID, Date: it.Date, Image: "/api/v1/music-trending/art/album/" + it.ID, Genres: []string{}}
	if it.ArtistID != "" {
		c.ArtistImage = "/api/v1/music-trending/art/artist/" + it.ArtistID
	}
	return c
}

// tplBlocks turns a region's shelves into the engine's list blocks.
func tplBlocks(sh *metadata.MusicShelves) map[string][]string {
	if sh == nil {
		return nil
	}
	out := map[string][]string{}
	for name, items := range sh.Blocks {
		for _, it := range items {
			id := strings.ToLower(it.MBID)
			if it.Kind == "artist" {
				out[name] = append(out[name], "r:"+id)
			} else {
				out[name] = append(out[name], "a:"+id)
			}
		}
	}
	if sh.Season != "" {
		out["season_key"] = []string{"season:" + sh.Season}
	}
	return out
}

// tplHue is a key's tile colour index (24 hues).
func tplHue(key string) int {
	h := fnv.New32a()
	h.Write([]byte(key))
	return int(h.Sum32() % 24)
}

// buildTplPage fills one Browse page: the Spotlight (hero and two more)
// and up to 30 rows. ok is false when the engine or the key is missing.
func (a *API) buildTplPage(ctx context.Context, member int64, kind, slug, region string, lib *musicLib, acts *musicActs, now time.Time) (hero *MusicCard, alts []MusicCard, rows []MusicRow, tile MusicCard, ok bool) {
	b := a.musicBundle(ctx, now)
	if b == nil {
		return nil, nil, nil, tile, false
	}
	key, found := tplKey(b, kind, slug)
	if !found {
		return nil, nil, nil, tile, false
	}
	hue := tplHue(key)
	tile = MusicCard{Key: key, Kind: "genre", MBID: slug, Name: b.Names[key].Name, Hue: &hue, Genres: []string{}}
	// Not enough data: no near-empty page (the caller writes the note).
	if a.tplCounts(ctx, b, region, now)[key] < rowtpl.MinPageRows {
		return nil, nil, nil, tile, true
	}
	sh := a.musicShelvesOf(ctx, region, now)
	c := &rowtpl.Ctx{B: b, Kind: kindOfKey(key), Key: key, Region: strings.ToLower(region), Now: now, Member: member, Lib: a.musicTplLib(ctx, member, lib, acts, b, now), Blocks: tplBlocks(sh), CountryName: countryName}
	if sh != nil {
		c.Neighbor = sh.Neighbor
	}
	spot := rowtpl.Spotlight(c)
	for k, i := range spot {
		card := tplCard(b.Items[i])
		if k == 0 {
			hero = &card
		} else {
			alts = append(alts, card)
		}
	}
	for _, r := range rowtpl.Build(c) {
		row := MusicRow{ID: "tpl:" + r.ID, Type: "tpl_" + strings.ToLower(r.Template), Title: r.Title, Reason: r.Reason, Shape: r.Shape}
		for _, i := range r.Items {
			row.Items = append(row.Items, tplCard(b.Items[i]))
		}
		rows = append(rows, row)
	}
	return hero, alts, rows, tile, true
}

// tplThin is the note for a page without enough data: links to up to 4
// similar pages that are shown.
func (a *API) tplThin(ctx context.Context, key, region string, now time.Time) []rowtpl.BrowseTile {
	b := a.musicBundle(ctx, now)
	if b == nil {
		return []rowtpl.BrowseTile{}
	}
	out := rowtpl.Similar(b, key, a.tplVisible(ctx, b, region, nil, now), 4)
	if out == nil {
		out = []rowtpl.BrowseTile{}
	}
	return out
}

// buildTplWorld fills New & Popular ("new") or the world part of Explore
// ("explore") from the engine: the Spotlight and up to 30 rows. ok is
// false when the bundle is missing.
func (a *API) buildTplWorld(ctx context.Context, r *http.Request, kind string, now time.Time) (hero []MusicCard, rows []MusicRow, ok bool) {
	b := a.musicBundle(ctx, now)
	if b == nil {
		return nil, nil, false
	}
	member := a.person(r).UserID
	region := strings.ToUpper(r.URL.Query().Get("region"))
	if region == "" || region == "GLOBAL" || !isCountry(region) {
		region = "global"
	}
	var lib *rowtpl.Lib
	if ml, err := a.musicLibrary(ctx, now); err == nil {
		if acts, err := a.musicActsOf(ctx, member); err == nil {
			lib = a.musicTplLib(ctx, member, ml, acts, b, now)
		}
	}
	sh := a.musicShelvesOf(ctx, region, now)
	c := &rowtpl.Ctx{B: b, Kind: kind, Region: strings.ToLower(region), Now: now, Member: member, Lib: lib, Blocks: tplBlocks(sh), CountryName: countryName}
	if sh != nil {
		c.Neighbor = sh.Neighbor
	}
	for _, i := range rowtpl.Spotlight(c) {
		hero = append(hero, tplCard(b.Items[i]))
	}
	for _, rw := range rowtpl.Build(c) {
		row := MusicRow{ID: "tpl:" + rw.ID, Type: "tpl_" + strings.ToLower(rw.Template), Title: rw.Title, Reason: rw.Reason, Shape: rw.Shape}
		for _, i := range rw.Items {
			row.Items = append(row.Items, tplCard(b.Items[i]))
		}
		rows = append(rows, row)
	}
	a.rememberMusicCards(rows, hero...)
	return hero, rows, len(rows) > 0
}

// tplRememberCovers lists the albums behind Browse tile covers, so the
// picture route serves them (and fetches them ahead).
func (a *API) tplRememberCovers(b *rowtpl.Bundle, secs []rowtpl.BrowseSection) {
	byID := map[string]rowtpl.Item{}
	for _, it := range b.Items {
		if it.Kind == "a" {
			byID[it.ID] = it
		}
	}
	var names []nameKey
	for _, s := range secs {
		for _, t := range s.Tiles {
			for _, c := range t.Covers {
				id := strings.TrimPrefix(c, musicArtPrefix+"album/")
				if it, ok := byID[id]; ok {
					names = append(names, nameKey{kind: "album", mbid: id, name: it.Title, artist: it.Artist})
				}
			}
		}
	}
	a.rememberMusicNames(names)
}

// tplExploreCands answers the engine's Explore candidates (world rows and
// the personal rows it owns; the Explore builder keeps its own).
func (a *API) tplExploreCands(ctx context.Context, member int64, region string, lib *musicLib, acts *musicActs, b *rowtpl.Bundle, sh *metadata.MusicShelves, now time.Time) []rowtpl.Cand {
	c := &rowtpl.Ctx{B: b, Kind: "explore", Region: strings.ToLower(region), Now: now, Member: member, Lib: a.musicTplLib(ctx, member, lib, acts, b, now), Blocks: tplBlocks(sh), CountryName: countryName}
	if sh != nil {
		c.Neighbor = sh.Neighbor
	}
	return rowtpl.Candidates(c)
}

// tplV7Row turns an engine candidate into a v7 plan row with its cards.
func tplV7Row(b *rowtpl.Bundle, cd rowtpl.Cand) (MusicRow, musicscore.Row, []MusicCard, []musicscore.Scored) {
	meta := MusicRow{ID: "tpl:" + cd.ID, Type: "tpl_" + cd.Family, Title: cd.Title, Reason: cd.Reason, Shape: cd.Shape}
	row := musicscore.Row{Prior: musicscore.PriorMixed, Min: rowtpl.RowMin, ChartSet: true, Chart: cd.Chart, Shape: cd.Shape, ArtistRow: cd.Shape == "round"}
	if cd.Shape != "round" {
		row.MinArtists = rowtpl.MinArtists
	}
	var cards []MusicCard
	var sc []musicscore.Scored
	for k, i := range cd.Items {
		card := tplCard(b.Items[i])
		cards = append(cards, card)
		item := musicscore.Item{Key: card.Key, Artist: card.ArtistMBID, Score: cd.Scores[k]}
		if card.Kind == "album" {
			item.Album = card.MBID
			item.FoldKey = fullAlbumKey(card.Name) + "|" + fullAlbumKey(card.Artist)
		}
		sc = append(sc, musicscore.Scored{Item: item})
	}
	return meta, row, cards, sc
}

func kindOfKey(key string) string { k, _, _ := strings.Cut(key, ":"); return k }

// countryName answers a country's name for titles ("the US").
func countryName(code string) string {
	if n, ok := listCountries[strings.ToLower(code)]; ok {
		return n
	}
	return ""
}

// musicBrowseGate runs the checks every Browse music page runs first: music turned
// on, no Kid limit (music has no ratings, so a Kid sees none of it; empty
// is written), and the Explore pacing. It answers false when it wrote the
// answer.
func (a *API) musicBrowseGate(w http.ResponseWriter, r *http.Request, empty map[string]any) bool {
	if p, _ := parts.ForTitleKind("artist"); !a.partIsOn(p) {
		fail(w, http.StatusNotFound, "music is turned off")
		return false
	}
	if a.kidMax(r) != "" {
		writeJSON(w, http.StatusOK, empty)
		return false
	}
	if !a.discover.allow(a.person(r).UserID, time.Now()) {
		w.Header().Set("Retry-After", "60")
		fail(w, http.StatusTooManyRequests, "too many Explore requests; try again in a minute")
		return false
	}
	return true
}

// musicBrowsePage answers GET /api/v1/music/browse/{kind}/{slug}: a mood,
// season, decade, activity or genre page from the row template engine.
func (a *API) musicBrowsePage(w http.ResponseWriter, r *http.Request) {
	kind, slug := r.PathValue("kind"), r.PathValue("slug")
	if !tplKinds[kind] || !tplKeyRe.MatchString(slug) {
		fail(w, http.StatusBadRequest, "unknown page")
		return
	}
	if !a.musicBrowseGate(w, r, map[string]any{"kid": true, "v2": true, "genres": []string{}, "genre": slug, "hero": nil, "rows": []MusicRow{}}) {
		return
	}
	if !a.musicTplOn() {
		fail(w, http.StatusNotFound, "music pages are off")
		return
	}
	a.serveTplPage(w, r, kind, slug)
}

// serveTplPage writes one engine page (shared with the genre route).
func (a *API) serveTplPage(w http.ResponseWriter, r *http.Request, kind, slug string) bool {
	if a.kidMax(r) != "" {
		writeJSON(w, http.StatusOK, map[string]any{"kid": true, "v2": true, "genres": []string{}, "genre": slug, "hero": nil, "rows": []MusicRow{}})
		return true
	}
	ctx := r.Context()
	now := time.Now().UTC()
	member := a.person(r).UserID
	region := strings.ToUpper(r.URL.Query().Get("region"))
	if region == "" || region == "GLOBAL" || !isCountry(region) {
		region = "global"
	}
	lib, err := a.musicLibrary(ctx, now)
	if err != nil {
		internal(w, err)
		return true
	}
	acts, err := a.musicActsOf(ctx, member)
	if err != nil {
		internal(w, err)
		return true
	}
	hero, alts, rows, tile, ok := a.buildTplPage(ctx, member, kind, slug, region, lib, acts, now)
	if !ok {
		if kind == "genre" {
			return false
		}
		fail(w, http.StatusNotFound, "no such page")
		return true
	}
	for i := range rows {
		for j := range rows[i].Items {
			rows[i].Items[j] = musicCardState(rows[i].Items[j], lib, acts)
		}
	}
	if hero != nil {
		h := musicCardState(*hero, lib, acts)
		hero = &h
	}
	for i := range alts {
		alts[i] = musicCardState(alts[i], lib, acts)
	}
	if rows == nil {
		// Not enough music for this page yet: a short note and similar pages.
		a.exploreCaching(w, r, "private, no-cache")
		writeJSON(w, http.StatusOK, map[string]any{"v2": true, "kid": false, "cold": false, "genres": []string{}, "genre": slug, "kind": kind, "region": region, "regions": []string{},
			"tile": tile, "hero": nil, "rows": []MusicRow{}, "thin": true, "similar": a.tplThin(ctx, tile.Key, region, now)})
		return true
	}
	// The picture route serves only listed albums and artists: list these
	// (and fetch their pictures ahead).
	extra := append([]MusicCard{}, alts...)
	if hero != nil {
		extra = append(extra, *hero)
	}
	a.rememberMusicCards(rows, extra...)
	a.exploreCaching(w, r, "private, no-cache")
	writeJSON(w, http.StatusOK, map[string]any{"v2": true, "kid": false, "cold": false, "genres": []string{}, "genre": slug, "kind": kind, "region": region, "regions": []string{},
		"tile": tile, "hero": hero, "hero_alts": alts, "rows": rows})
	return true
}

// musicBrowseTiles answers GET /api/v1/music/browse-tiles?region=: the
// Search > Music Browse sections (genres, moods, seasons, decades,
// activities) with their tiles' pages and covers.
func (a *API) musicBrowseTiles(w http.ResponseWriter, r *http.Request) {
	if !a.musicBrowseGate(w, r, map[string]any{"kid": true, "sections": []rowtpl.BrowseSection{}}) {
		return
	}
	if !a.musicTplOn() {
		fail(w, http.StatusNotFound, "music pages are off")
		return
	}
	ctx := r.Context()
	now := time.Now().UTC()
	b := a.musicBundle(ctx, now)
	if b == nil {
		fail(w, http.StatusServiceUnavailable, "music catalog not ready")
		return
	}
	region := strings.ToLower(r.URL.Query().Get("region"))
	if region == "" || !isCountry(strings.ToUpper(region)) {
		region = "global"
	}
	member := a.person(r).UserID
	var lib *rowtpl.Lib
	if ml, err := a.musicLibrary(ctx, now); err == nil {
		if acts, err := a.musicActsOf(ctx, member); err == nil {
			lib = a.musicTplLib(ctx, member, ml, acts, b, now)
		}
	}
	a.exploreCaching(w, r, "private, no-cache")
	secs := a.tplVisible(ctx, b, region, lib, now)
	a.tplRememberCovers(b, secs)
	writeJSON(w, http.StatusOK, map[string]any{"sections": secs})
}
