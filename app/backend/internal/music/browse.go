package music

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

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/music/rowtpl"
	musicscore "github.com/HolyArrs/holyarrs/app/backend/internal/music/score"
)

// The row template engine (plan 24 October 2026-music-row-
// templates): Browse pages for genres, moods, seasons, decades and
// activities, filled from the server's catalog bundle.

// musicTplOn reports whether the engine serves music pages.
func (h *Handlers) musicTplOn() bool {
	return !h.MusicRowsTplOff && h.MusicPools != nil
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
func (h *Handlers) musicBundle(ctx context.Context, now time.Time) *rowtpl.Bundle {
	b := h.readMusicBundle(ctx, now)
	if b != nil {
		h.musicPre.catalog.Store(b)
		h.tplPreload(b, "") // a new bundle's covers are stored again
	}
	return b
}

func (h *Handlers) readMusicBundle(ctx context.Context, now time.Time) *rowtpl.Bundle {
	if v, ok := h.exploreCache.cand.Get("tpl:bundle", now); ok {
		return v.(*tplBundle).b
	}
	if _, failed := h.exploreCache.cand.Get("tpl:fail", now); failed {
		return nil
	}
	select {
	case tplFetch <- struct{}{}:
	case <-ctx.Done():
		return nil
	}
	defer func() { <-tplFetch }()
	// Another request may have filled the cache while this one waited.
	if v, ok := h.exploreCache.cand.Get("tpl:bundle", now); ok {
		return v.(*tplBundle).b
	}
	if _, failed := h.exploreCache.cand.Get("tpl:fail", now); failed {
		return nil
	}
	var last *tplBundle
	if v, ok := h.exploreCache.cand.Get("tpl:last", now); ok {
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
	body, tag, err := h.MusicPools(dctx, etag)
	switch {
	case errors.Is(err, metadata.ErrNotModified) && last != nil:
		h.exploreCache.cand.Put("tpl:bundle", last, tplBundleTTL, now)
		return last.b
	case err != nil:
		if last != nil {
			h.exploreCache.cand.Put("tpl:bundle", last, 10*time.Minute, now)
			return last.b
		}
		// The download's own result decides (the caller cannot cancel it):
		// a failure, its own timeout included, is kept for minutes.
		h.exploreCache.cand.Put("tpl:fail", true, tplFailTTL, now)
		return nil
	}
	b, err := rowtpl.ParseBundle(body)
	if err != nil {
		if last != nil {
			h.exploreCache.cand.Put("tpl:bundle", last, 10*time.Minute, now)
			return last.b
		}
		h.exploreCache.cand.Put("tpl:fail", true, tplFailTTL, now)
		return nil
	}
	x := &tplBundle{b: b, etag: tag}
	h.exploreCache.cand.Put("tpl:bundle", x, tplBundleTTL, now)
	h.exploreCache.cand.Put("tpl:last", x, tplKeepTTL, now)
	return b
}

// tplCountsMu lets one count run at a time (it builds every Browse page).
var tplCountsMu sync.Mutex

// tplCounts answers each Browse page's row count for a region, as a member
// with an empty library sees it: cached per region and bundle version for
// 6 hours, so tiles appear by themselves once their data grows.
func (h *Handlers) tplCounts(ctx context.Context, b *rowtpl.Bundle, region string, now time.Time) map[string]int {
	key := "tpl:counts:" + strings.ToLower(region) + ":" + b.Version
	if v, ok := h.exploreCache.cand.Get(key, now); ok {
		return v.(map[string]int)
	}
	tplCountsMu.Lock()
	defer tplCountsMu.Unlock()
	if v, ok := h.exploreCache.cand.Get(key, now); ok {
		return v.(map[string]int)
	}
	counts := rowtpl.PageCounts(b, now, strings.ToLower(region), tplBlocks(h.musicShelvesOf(ctx, region, now)), rowtpl.BrowseKeys())
	h.exploreCache.cand.Put(key, counts, tplBundleTTL, now)
	return counts
}

// tplVisible answers the Browse sections a member sees: tiles whose page
// has enough rows, seasons inside their window.
func (h *Handlers) tplVisible(ctx context.Context, b *rowtpl.Bundle, region string, lib *rowtpl.Lib, now time.Time) []rowtpl.BrowseSection {
	r := strings.ToLower(region)
	return rowtpl.VisibleSections(rowtpl.BrowseSections(b, now, r, lib), h.tplCounts(ctx, b, region, now), now, r)
}

// tplVisibleArt is tplVisible with the cover rule: each tile's covers come
// only from albums whose picture is stored or known to load, and a tile with
// no such album is left out. The Browse tiles every page shows come from it;
// tplVisible (every candidate cover) is for the preload, which stores them.
func (h *Handlers) tplVisibleArt(ctx context.Context, b *rowtpl.Bundle, region string, lib *rowtpl.Lib, now time.Time) []rowtpl.BrowseSection {
	r := strings.ToLower(region)
	return rowtpl.VisibleSections(rowtpl.BrowseSectionsArt(b, now, r, lib, h.tplHasArt(b)), h.tplCounts(ctx, b, region, now), now, r)
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
func (h *Handlers) musicTplLib(ctx context.Context, member int64, lib *musicLib, acts *musicActs, b *rowtpl.Bundle, now time.Time) *rowtpl.Lib {
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
	sims := h.musicSimilarOf(ctx, seedIDs, now)
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
		c.ArtistImage = heroArtistImage(it.ArtistID)
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
	hv := fnv.New32a()
	hv.Write([]byte(key))
	return int(hv.Sum32() % 24)
}

// buildTplPage fills one Browse page: the Spotlight (hero and two more)
// and up to 30 rows. ok is false when the engine or the key is missing.
func (h *Handlers) buildTplPage(ctx context.Context, member int64, kind, slug, region string, lib *musicLib, acts *musicActs, now time.Time) (hero *MusicCard, alts []MusicCard, rows []MusicRow, tile MusicCard, ok bool) {
	b := h.musicBundle(ctx, now)
	if b == nil {
		return nil, nil, nil, tile, false
	}
	h.tplPreload(b, region)
	key, found := tplKey(b, kind, slug)
	if !found {
		return nil, nil, nil, tile, false
	}
	hue := tplHue(key)
	tile = MusicCard{Key: key, Kind: "genre", MBID: slug, Name: b.Names[key].Name, Hue: &hue, Genres: []string{}}
	// Not enough data: no near-empty page (the caller writes the note).
	if h.tplCounts(ctx, b, region, now)[key] < rowtpl.MinPageRows {
		return nil, nil, nil, tile, true
	}
	sh := h.musicShelvesOf(ctx, region, now)
	tl := h.musicTplLib(ctx, member, lib, acts, b, now)
	hasArt := h.tplHasArt(b)
	build := func(m int64) ([]int, []rowtpl.Row) {
		c := &rowtpl.Ctx{B: b, Kind: kindOfKey(key), Key: key, Region: strings.ToLower(region), Now: now, Member: m, Lib: tl, Blocks: tplBlocks(sh), CountryName: countryName, HasArt: hasArt}
		if sh != nil {
			c.Neighbor = sh.Neighbor
		}
		spot := rowtpl.Spotlight(c)
		return spot, rowtpl.Build(c)
	}
	// The tile count is taken without a member; the member's daily mix can
	// fold rows together and leave fewer. Then take the page without the
	// member's mix, and when it is still short, the note (never a
	// near-empty page).
	spot, built := build(member)
	if len(built) < rowtpl.MinPageRows && member != 0 {
		spot, built = build(0)
	}
	if len(built) < rowtpl.MinPageRows {
		return nil, nil, nil, tile, true
	}
	for k, i := range spot {
		card := tplCard(b.Items[i])
		if k == 0 {
			hero = &card
		} else {
			alts = append(alts, card)
		}
	}
	for _, r := range built {
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
func (h *Handlers) tplThin(ctx context.Context, key, region string, now time.Time) []rowtpl.BrowseTile {
	b := h.musicBundle(ctx, now)
	if b == nil {
		return []rowtpl.BrowseTile{}
	}
	out := rowtpl.Similar(b, key, h.tplVisible(ctx, b, region, nil, now), 4)
	if out == nil {
		out = []rowtpl.BrowseTile{}
	}
	return out
}

// buildTplWorld fills New & Popular ("new") or the world part of Explore
// ("explore") from the engine: the Spotlight and up to 30 rows. ok is
// false when the bundle is missing.
func (h *Handlers) buildTplWorld(ctx context.Context, r *http.Request, kind string, now time.Time) (hero []MusicCard, rows []MusicRow, ok bool) {
	b := h.musicBundle(ctx, now)
	if b == nil {
		return nil, nil, false
	}
	member := h.Person(r).UserID
	region := strings.ToUpper(r.URL.Query().Get("region"))
	if region == "" || region == "GLOBAL" || !isCountry(region) {
		region = "global"
	}
	h.tplPreload(b, region)
	var lib *rowtpl.Lib
	if ml, err := h.musicLibrary(ctx, now); err == nil {
		if acts, err := h.musicActsOf(ctx, member); err == nil {
			lib = h.musicTplLib(ctx, member, ml, acts, b, now)
		}
	}
	sh := h.musicShelvesOf(ctx, region, now)
	c := &rowtpl.Ctx{B: b, Kind: kind, Region: strings.ToLower(region), Now: now, Member: member, Lib: lib, Blocks: tplBlocks(sh), CountryName: countryName, HasArt: h.tplHasArt(b)}
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
	h.rememberMusicCards(rows, hero...)
	return hero, rows, len(rows) > 0
}

// tplRememberCovers lists the albums behind Browse tile covers, so the
// picture route serves them (and fetches them ahead).
func (h *Handlers) tplRememberCovers(b *rowtpl.Bundle, secs []rowtpl.BrowseSection) {
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
	h.rememberMusicNames(names)
}

// tplExploreCands answers the engine's Explore candidates (world rows and
// the personal rows it owns; the Explore builder keeps its own).
func (h *Handlers) tplExploreCands(ctx context.Context, member int64, region string, lib *musicLib, acts *musicActs, b *rowtpl.Bundle, sh *metadata.MusicShelves, now time.Time) []rowtpl.Cand {
	c := &rowtpl.Ctx{B: b, Kind: "explore", Region: strings.ToLower(region), Now: now, Member: member, Lib: h.musicTplLib(ctx, member, lib, acts, b, now), Blocks: tplBlocks(sh), CountryName: countryName, HasArt: h.tplHasArt(b)}
	if sh != nil {
		c.Neighbor = sh.Neighbor
	}
	return rowtpl.Candidates(c)
}

// tplPlanRow turns an engine candidate into a v7 plan row with its cards.
func tplPlanRow(b *rowtpl.Bundle, cd rowtpl.Cand) (MusicRow, musicscore.Row, []MusicCard, []musicscore.Scored) {
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
func (h *Handlers) musicBrowseGate(w http.ResponseWriter, r *http.Request, empty map[string]any) bool {
	if p, _ := parts.ForTitleKind("artist"); !h.PartIsOn(p) {
		webserver.Fail(w, http.StatusNotFound, "music is turned off")
		return false
	}
	if h.KidMax(r) != "" {
		webserver.WriteJSON(w, http.StatusOK, empty)
		return false
	}
	if !h.Limit(h.Person(r).UserID, time.Now()) {
		w.Header().Set("Retry-After", "60")
		webserver.Fail(w, http.StatusTooManyRequests, "too many Explore requests; try again in a minute")
		return false
	}
	return true
}

// musicBrowsePage answers GET /api/v1/music/browse/{kind}/{slug}: a mood,
// season, decade, activity or genre page from the row template engine.
func (h *Handlers) musicBrowsePage(w http.ResponseWriter, r *http.Request) {
	kind, slug := r.PathValue("kind"), r.PathValue("slug")
	if !tplKinds[kind] || !tplKeyRe.MatchString(slug) {
		webserver.Fail(w, http.StatusBadRequest, "unknown page")
		return
	}
	if !h.musicBrowseGate(w, r, map[string]any{"kid": true, "v2": true, "genres": []string{}, "genre": slug, "hero": nil, "rows": []MusicRow{}}) {
		return
	}
	if !h.musicTplOn() {
		webserver.Fail(w, http.StatusNotFound, "music pages are off")
		return
	}
	h.serveTplPage(w, r, kind, slug)
}

// serveTplPage writes one engine page (shared with the genre route).
func (h *Handlers) serveTplPage(w http.ResponseWriter, r *http.Request, kind, slug string) bool {
	if h.KidMax(r) != "" {
		webserver.WriteJSON(w, http.StatusOK, map[string]any{"kid": true, "v2": true, "genres": []string{}, "genre": slug, "hero": nil, "rows": []MusicRow{}})
		return true
	}
	ctx := r.Context()
	now := time.Now().UTC()
	member := h.Person(r).UserID
	region := strings.ToUpper(r.URL.Query().Get("region"))
	if region == "" || region == "GLOBAL" || !isCountry(region) {
		region = "global"
	}
	lib, err := h.musicLibrary(ctx, now)
	if err != nil {
		webserver.Internal(w, "api", err)
		return true
	}
	acts, err := h.musicActsOf(ctx, member)
	if err != nil {
		webserver.Internal(w, "api", err)
		return true
	}
	hero, alts, rows, tile, ok := h.buildTplPage(ctx, member, kind, slug, region, lib, acts, now)
	if !ok {
		if kind == "genre" {
			return false
		}
		webserver.Fail(w, http.StatusNotFound, "no such page")
		return true
	}
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
	if rows == nil {
		// Not enough music for this page yet: a short note and similar pages.
		h.ExploreCaching(w, r, "private, no-cache")
		h.WriteMusicJSON(w, r, map[string]any{"v2": true, "kid": false, "cold": false, "genres": []string{}, "genre": slug, "kind": kind, "region": region, "regions": []string{},
			"tile": tile, "hero": nil, "rows": []MusicRow{}, "thin": true, "similar": h.tplThin(ctx, tile.Key, region, now)})
		return true
	}
	// The picture route serves only listed albums and artists: list these
	// (and fetch their pictures ahead).
	extra := append([]MusicCard{}, alts...)
	if hero != nil {
		extra = append(extra, *hero)
	}
	h.rememberMusicCards(rows, extra...)
	h.ExploreCaching(w, r, "private, no-cache")
	h.WriteMusicJSON(w, r, map[string]any{"v2": true, "kid": false, "cold": false, "genres": []string{}, "genre": slug, "kind": kind, "region": region, "regions": []string{},
		"tile": tile, "hero": hero, "hero_alts": alts, "rows": rows})
	return true
}

// musicBrowseTiles answers GET /api/v1/music/browse-tiles?region=: the
// Search > Music Browse sections (genres, moods, seasons, decades,
// activities) with their tiles' pages and covers, and "explore", the mix of
// them Explore and New & Popular show. A tile has only covers of albums whose
// picture is stored or known to load; a tile with none is left out.
func (h *Handlers) musicBrowseTiles(w http.ResponseWriter, r *http.Request) {
	if !h.musicBrowseGate(w, r, map[string]any{"kid": true, "sections": []rowtpl.BrowseSection{}}) {
		return
	}
	if !h.musicTplOn() {
		webserver.Fail(w, http.StatusNotFound, "music pages are off")
		return
	}
	ctx := r.Context()
	now := time.Now().UTC()
	b := h.musicBundle(ctx, now)
	if b == nil {
		webserver.Fail(w, http.StatusServiceUnavailable, "music catalog not ready")
		return
	}
	region := strings.ToLower(r.URL.Query().Get("region"))
	if region == "" || !isCountry(strings.ToUpper(region)) {
		region = "global"
	}
	h.tplPreload(b, region)
	member := h.Person(r).UserID
	var lib *rowtpl.Lib
	if ml, err := h.musicLibrary(ctx, now); err == nil {
		if acts, err := h.musicActsOf(ctx, member); err == nil {
			lib = h.musicTplLib(ctx, member, ml, acts, b, now)
		}
	}
	h.ExploreCaching(w, r, "private, no-cache")
	secs := h.tplVisibleArt(ctx, b, region, lib, now)
	if secs == nil {
		secs = []rowtpl.BrowseSection{}
	}
	h.tplRememberCovers(b, secs)
	explore := rowtpl.ExploreTiles(secs)
	if explore == nil {
		explore = []rowtpl.BrowseTile{}
	}
	h.WriteMusicJSON(w, r, map[string]any{"sections": secs, "explore": explore})
}
