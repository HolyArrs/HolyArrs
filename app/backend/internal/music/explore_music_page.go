package music

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/discover/rows"
	"github.com/HolyArrs/holyarrs/app/backend/internal/parts"
)

// exploreMusic answers GET /api/v1/explore/music?genre=<tag>&region=<code>.
func (h *Handlers) exploreMusic(w http.ResponseWriter, r *http.Request) {
	if p, _ := parts.ForTitleKind("artist"); !h.PartIsOn(p) {
		webserver.Fail(w, http.StatusNotFound, "music is turned off")
		return
	}
	// Music has no ratings, so a Kid limit shows none of it.
	if h.KidMax(r) != "" {
		webserver.WriteJSON(w, http.StatusOK, map[string]any{"kid": true, "cold": false, "genres": []string{}, "genre": "", "hero": nil, "rows": []MusicRow{}})
		return
	}
	if !h.Limit(h.Person(r).UserID, time.Now()) {
		w.Header().Set("Retry-After", "60")
		webserver.Fail(w, http.StatusTooManyRequests, "too many Explore requests; try again in a minute")
		return
	}
	if h.MusicRowsV2 && h.MusicRowsV3 && h.MusicRowsV7 {
		h.exploreMusicV7(w, r)
		return
	}
	if h.MusicRowsV2 {
		h.exploreMusicV2(w, r)
		return
	}
	q := r.URL.Query()
	genre := strings.ToLower(strings.TrimSpace(q.Get("genre")))
	region := strings.ToUpper(q.Get("region"))
	if region == "" || region == "GLOBAL" || !isCountry(region) {
		region = "global"
	}
	ctx := r.Context()
	db := h.Store.Read()
	now := time.Now().UTC()
	key := musicAnswerKey(h.Person(r).UserID, genre, region, musicHour(q.Get("hour"), now))
	if body, ok := h.cachedMusicAnswer(key, now); ok {
		h.ExploreCaching(w, r, "private, max-age=60")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
		return
	}

	titles, err := database.Select[mediafiles.Title](ctx, db, "titles", "kind = 'artist' AND state != 'removed' ORDER BY id")
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	files := map[int64]int{}
	rows, err := db.QueryContext(ctx, `SELECT i.title_id, COUNT(*) FROM items i JOIN slots s ON s.item_id = i.id
		WHERE i.kind = 'track' AND s.file_id IS NOT NULL GROUP BY i.title_id`)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	for rows.Next() {
		var id int64
		var n int
		if rows.Scan(&id, &n) == nil {
			files[id] = n
		}
	}
	rows.Close()
	artists := make([]*musicArtist, 0, len(titles))
	byTitle := map[int64]*musicArtist{}
	owned := map[string]int64{}
	var ids []string
	for _, t := range titles {
		m := &musicArtist{id: t.ID, name: t.Name, mbid: strings.ToLower(t.SourceIDs["musicbrainz"]), image: artistImage(t), files: files[t.ID], added: t.CreatedAt}
		artists = append(artists, m)
		byTitle[t.ID] = m
		if m.mbid != "" {
			owned[m.mbid] = t.ID
			ids = append(ids, m.mbid)
		}
	}
	// Seeds: the artists with most music on disk, then the newest adds;
	// the top 8 take turns by day so the rows change.
	seeds := append([]*musicArtist{}, artists...)
	sort.SliceStable(seeds, func(i, j int) bool {
		if seeds[i].files != seeds[j].files {
			return seeds[i].files > seeds[j].files
		}
		return seeds[i].added.After(seeds[j].added)
	})
	seeds = seeds[:min(16, len(seeds))]
	if n := len(seeds); n > 1 {
		k := int(now.Unix()/86400) % n
		seeds = append(seeds[k:], seeds[:k]...)
	}
	seeds = seeds[:min(musicSeedCount(len(artists)), len(seeds))]
	var seedIDs []string
	for _, s := range seeds {
		if s.mbid != "" {
			seedIDs = append(seedIDs, s.mbid)
		}
	}
	st := webserver.NewServerTiming()
	similar := h.similarTo(seedIDs, musicSimilarWait)
	st.Mark("similar")

	// Charts, from the cached trending lists.
	var trendArtists, trendAlbums []MusicCard
	regions := []string{}
	if h.MusicTrending != nil {
		for _, kind := range []string{"artist", "album"} {
			l, err := h.cachedMusicTrend(ctx, kind, "global")
			if err != nil {
				continue
			}
			if len(l.Regions) > len(regions) {
				regions = l.Regions
			}
			for _, it := range l.Items {
				c := MusicCard{Kind: kind, MBID: strings.ToLower(it.MBID), Rank: it.Rank, Listens: it.Listens}
				if kind == "artist" {
					c.Name, c.ArtistMBID, c.Artist = it.Name, c.MBID, it.Name
				} else {
					c.Name, c.Artist, c.ArtistMBID = it.Title, it.Artist, strings.ToLower(it.ArtistMBID)
				}
				if c.MBID == "" || c.Name == "" || c.ArtistMBID == "" {
					continue
				}
				c.Image = "/api/v1/music-trending/art/" + kind + "/" + c.MBID
				if kind == "artist" {
					trendArtists = append(trendArtists, c)
				} else {
					trendAlbums = append(trendAlbums, c)
				}
				ids = append(ids, c.ArtistMBID)
			}
		}
	}
	for _, l := range similar {
		for _, s := range l {
			ids = append(ids, s.MBID)
		}
	}
	st.Mark("charts")
	genres, genresDone := h.genresWaiting(ids, musicGenreWait)
	st.Mark("genres")
	for _, m := range artists {
		m.genres = genres[m.mbid]
	}
	tag := func(c MusicCard) MusicCard {
		c.Genres = genres[c.ArtistMBID]
		if c.Genres == nil {
			c.Genres = []string{}
		}
		if id, ok := owned[c.ArtistMBID]; ok && c.Kind == "artist" {
			c.TitleID = id
		}
		return c
	}
	var names []nameKey
	albumCard := func(m *musicArtist, p mediafiles.Part) MusicCard {
		rg := strings.ToLower(p.SourceIDs["musicbrainz"])
		c := MusicCard{Key: "album:" + strconv.FormatInt(p.ID, 10), Kind: "album", MBID: rg, Name: p.Name, Artist: m.name, ArtistMBID: m.mbid, TitleID: m.id, PartID: p.ID, Date: dayOf(p.ReleaseDate)}
		if rg != "" {
			c.Image = "/api/v1/music-trending/art/album/" + rg
			names = append(names, nameKey{"album", rg, p.Name, m.name, ""})
		}
		return tag(c)
	}

	var out []MusicRow
	add := func(r MusicRow) {
		for i := range r.Items {
			if r.Items[i].Key == "" {
				r.Items[i].Key = r.Items[i].Kind + ":" + r.Items[i].MBID
			}
		}
		out = append(out, r)
	}

	// New from artists you have: studio releases (no live, remix or
	// compilation) out in the last 90 days, then the ones coming. The hero is
	// the newest album, else the newest EP; the next ones are its stand-ins
	// (the page shows the first whose cover loads).
	var hero *MusicCard
	var heroAlts, heroAlbums, heroEPs []MusicCard
	{
		ps, err := database.Select[mediafiles.Part](ctx, db, "parts", `kind = 'album' AND dropped_at IS NULL AND release_date >= ? AND release_date < ?
			AND lower(coalesce(release_type, '')) IN ('album', 'ep', 'single') AND secondary_types = '[]' AND coalesce(release_date_precision, 'day') != 'year'
			AND title_id IN (SELECT id FROM titles WHERE kind = 'artist' AND state != 'removed') ORDER BY release_date DESC, id`,
			now.AddDate(0, 0, -90).Format("2006-01-02"), now.AddDate(0, 0, 180).Format("2006-01-02"))
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		var out, coming []MusicCard
		for _, p := range ps {
			m := byTitle[p.TitleID]
			if m == nil {
				continue
			}
			c := albumCard(m, p)
			if p.ReleaseDate.After(now) {
				c.Upcoming = true
				coming = append([]MusicCard{c}, coming...) // soonest first
				continue
			}
			out = append(out, c)
			hv := c
			hv.ArtistImage = m.image
			switch strings.ToLower(derefStr(p.ReleaseType)) {
			case "album":
				heroAlbums = append(heroAlbums, hv)
			case "ep":
				heroEPs = append(heroEPs, hv)
			}
		}
		if all := append(heroAlbums, heroEPs...); len(all) > 0 {
			hero, heroAlts = &all[0], all[1:min(len(all), 6)]
		}
		add(MusicRow{ID: "new_from", Type: "new_from", Title: "New from artists you have", Reason: "Out in the last 90 days, and coming soon", Shape: "square", Items: append(out, coming...)})
	}

	// Charts.
	if hero == nil && len(trendAlbums) > 0 {
		for i, c := range trendAlbums[:min(len(trendAlbums), 6)] {
			c = tag(c)
			c.ArtistImage = heroArtistImage(c.ArtistMBID)
			names = append(names, nameKey{"artist", c.ArtistMBID, c.Artist, "", ""})
			if i == 0 {
				hero = &c
			} else {
				heroAlts = append(heroAlts, c)
			}
		}
	}
	for i := range trendAlbums {
		trendAlbums[i] = tag(trendAlbums[i])
	}
	for i := range trendArtists {
		trendArtists[i] = tag(trendArtists[i])
	}
	add(MusicRow{ID: "top_albums", Type: "top_albums", Title: "Top albums this week", Reason: chartReason("global", "Chart"), Shape: "square", Items: trendAlbums})
	add(MusicRow{ID: "trending_artists", Type: "trending_artists", Title: "Trending artists", Reason: chartReason("global", "Most played"), Shape: "round", Items: trendArtists})

	// Because you have / Fans of: similar artists not in the Library.
	pool := map[string]MusicCard{}
	for i, s := range seeds {
		list := similar[s.mbid]
		var items []MusicCard
		for _, x := range list {
			if _, mine := owned[x.MBID]; mine {
				continue
			}
			c := tag(MusicCard{Kind: "artist", MBID: x.MBID, Name: x.Name, Artist: x.Name, ArtistMBID: x.MBID, Image: "/api/v1/music-trending/art/artist/" + x.MBID})
			if len(c.Genres) == 0 {
				c.Genres = s.genres // like its seed until its own are known
			}
			names = append(names, nameKey{"artist", x.MBID, x.Name, "", ""})
			items = append(items, c)
			if _, ok := pool[x.MBID]; !ok {
				pool[x.MBID] = c
			}
		}
		t, title, reason := "because_have", "Because you have "+s.name, "Artists people who play "+s.name+" also play"
		if i%2 == 1 {
			t, title, reason = "fans_of", "Fans of "+s.name+" also like", "From listening sessions that include "+s.name
		}
		add(MusicRow{ID: t + ":" + s.mbid, Type: t, Title: title, Reason: reason, Shape: "round", Items: items})
	}

	// Missing albums: studio albums of artists you have that have no music
	// yet, artists with music first, a few each, newest first.
	{
		ps, err := database.Select[mediafiles.Part](ctx, db, "parts", `kind = 'album' AND dropped_at IS NULL AND release_date IS NOT NULL AND release_date <= ?
			AND lower(coalesce(release_type, '')) = 'album' AND secondary_types = '[]'
			AND title_id IN (SELECT id FROM titles WHERE kind = 'artist' AND state != 'removed')
			AND NOT EXISTS (SELECT 1 FROM items i JOIN slots s ON s.item_id = i.id WHERE i.part_id = parts.id AND s.file_id IS NOT NULL)
			ORDER BY release_date DESC, id`, now.Format("2006-01-02T15:04:05Z"))
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		by := map[int64][]mediafiles.Part{}
		for _, p := range ps {
			if len(by[p.TitleID]) < 3 {
				by[p.TitleID] = append(by[p.TitleID], p)
			}
		}
		order := append([]*musicArtist{}, artists...)
		sort.SliceStable(order, func(i, j int) bool { return order[i].files > order[j].files })
		var items []MusicCard
		for round := 0; round < 3 && len(items) < musicRowMax; round++ {
			for _, m := range order {
				if l := by[m.id]; round < len(l) && len(items) < musicRowMax {
					items = append(items, albumCard(m, l[round]))
				}
			}
		}
		add(MusicRow{ID: "missing", Type: "missing", Title: "Missing albums from artists you have", Reason: "Studio albums not in your Library yet", Shape: "square", Items: items})
	}

	// Genre rows: the Library's top genres; cold start: the charts'.
	cold := len(artists) == 0
	count := map[string]int{}
	if cold {
		for _, c := range append(append([]MusicCard{}, trendArtists...), trendAlbums...) {
			for _, g := range c.Genres {
				count[g]++
			}
		}
	} else {
		for _, m := range artists {
			for _, g := range m.genres {
				count[g]++
			}
		}
	}
	top := topGenres(count, 8)
	for _, c := range trendArtists {
		if _, ok := pool[c.MBID]; !ok && c.TitleID == 0 {
			pool[c.MBID] = c
		}
	}
	nGenreRows := 3
	if len(artists) >= 10 {
		nGenreRows = 6
	}
	if cold {
		nGenreRows = 6
	}
	for _, g := range top[:min(nGenreRows, len(top))] {
		var items []MusicCard
		for _, c := range sortedPool(pool) {
			if hasGenre(c.Genres, g) {
				items = append(items, c)
			}
		}
		if cold {
			add(MusicRow{ID: "popular_genre:" + g, Type: "popular_genre", Title: "Popular " + g, Reason: "Charting " + g + " artists this week", Shape: "round", Items: items})
		} else {
			add(MusicRow{ID: "genre:" + g, Type: "genre", Title: titleCase(g) + " picks for you", Reason: "Because " + g + " is big in your Library", Shape: "round", Items: items})
		}
	}

	out = h.moreMusicRows(ctx, &musicCtx{now: now, hour: musicHour(q.Get("hour"), now), region: region, artists: artists, byTitle: byTitle, owned: owned,
		genres: genres, trendAlbums: trendAlbums, trendArtists: trendArtists, pool: pool, top: top, tag: tag, albumCard: albumCard, names: &names, cold: cold}, out)
	st.Mark("more")

	// Cold start: only rows that need no Library.
	final := []MusicRow{}
	pageSeen := map[string]bool{}
	if hero != nil && genre == "" {
		pageSeen[hero.Key] = true
	}
	for _, row := range out {
		if cold && !musicColdTypes[row.Type] {
			continue
		}
		if genre != "" {
			kept := []MusicCard{}
			for _, c := range row.Items {
				if hasGenre(c.Genres, genre) {
					kept = append(kept, c)
				}
			}
			row.Items = kept
		}
		if row.Type == "new_from" && hero != nil {
			kept := []MusicCard{}
			for _, c := range row.Items {
				if c.Key != hero.Key {
					kept = append(kept, c)
				}
			}
			row.Items = kept
		}
		row.Items = uniqCards(row.Items)
		// A title once in the first rows (hero included), unless that leaves
		// the row under its minimum.
		if len(final) < engine.HeadDedupe {
			fresh := []MusicCard{}
			for _, c := range row.Items {
				if !pageSeen[c.Key] {
					fresh = append(fresh, c)
				}
			}
			if len(fresh) >= musicMinFor(row.Type, len(artists)) {
				row.Items = fresh
			}
		}
		if len(row.Items) > musicRowMax {
			row.Items = row.Items[:musicRowMax]
		}
		if len(row.Items) < musicMinFor(row.Type, len(artists)) {
			continue
		}
		for i := range row.Items {
			row.Items[i].Fresh = h.MusicFresh(row.Type, row.Items[i])
			pageSeen[row.Items[i].Key] = true
		}
		final = append(final, row)
	}
	if genre != "" {
		cands := []MusicCard{}
		if hero != nil {
			cands = append(cands, *hero)
		}
		hero = nil
		kept := []MusicCard{}
		for _, c := range append(cands, heroAlts...) {
			if hasGenre(c.Genres, genre) {
				kept = append(kept, c)
			}
		}
		if len(kept) > 0 {
			hero, heroAlts = &kept[0], kept[1:]
		} else {
			heroAlts = nil
		}
	}
	if heroAlts == nil {
		heroAlts = []MusicCard{}
	}
	h.rememberMusicNames(names)
	h.ExploreCaching(w, r, "private, max-age=60")
	complete := genresDone && len(similar) == len(seedIDs)
	st.Write(w)
	h.writeMusicAnswer(w, key, map[string]any{"kid": false, "cold": cold, "genres": top, "genre": genre, "region": region, "regions": regions, "hero": hero, "hero_alts": heroAlts, "rows": final}, complete, now)
}

type nameKey struct{ kind, mbid, name, artist, cover string }

// musicNamesMax caps the listed albums and artists (the list starts over
// past it); room for a household's open pages, catalog items aside (the
// picture route finds those in the catalog).
const musicNamesMax = 60000

// rememberMusicNames lets the picture route serve these albums and artists.
func (h *Handlers) rememberMusicNames(list []nameKey) {
	h.rememberNames(list)
	// Every row and page that lists them also gets their pictures stored.
	h.warmArtItems(list)
}
