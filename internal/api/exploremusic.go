package api

import (
	"net/url"
	"context"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/explore"
	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/parts"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// Explore â†’ Music: every row the page shows, built here so they share one
// genre filter and one set of rules. Albums are square, artists round; each
// row says why it is there and hides below its minimum.

// musicRowMin is the fewest cards each music row shows.
var musicRowMin = map[string]int{
	"new_from": 3, "top_albums": 5, "trending_artists": 5,
	"because_have": 4, "fans_of": 4, "missing": 4, "genre": 4, "popular_genre": 4,
}

const (
	musicRowMax = 20
	// musicGenreTTL keeps an artist's genres; musicSimilarTTL a similar list
	// (misses an hour).
	musicGenreTTL   = 7 * 24 * time.Hour
	musicSimilarTTL = 24 * time.Hour
	// musicGenreLookups caps background MusicBrainz genre lookups an hour.
	musicGenreLookups = 300
	musicCacheMax     = 5000
)

// MusicCard is one album or artist card.
type MusicCard struct {
	Key        string `json:"key"`
	Kind       string `json:"kind"` // album, artist
	MBID       string `json:"mbid"`
	Name       string `json:"name"`
	Artist     string `json:"artist,omitempty"`
	ArtistMBID string `json:"artist_mbid,omitempty"`
	TitleID    int64  `json:"title_id,omitempty"`
	PartID     int64  `json:"part_id,omitempty"`
	Image      string `json:"image,omitempty"`
	// ArtistImage is the artist's picture, on hero candidates: the hero
	// shows it when no candidate album has a cover.
	ArtistImage string   `json:"artist_image,omitempty"`
	Date        string   `json:"date,omitempty"` // YYYY-MM-DD: arrived, out or asked
	State       string   `json:"state,omitempty"`
	Rank        int      `json:"rank,omitempty"`
	Listens     int64    `json:"listen_count,omitempty"`
	Upcoming    bool     `json:"upcoming,omitempty"`
	Genres      []string `json:"genres"`
	// Note is the card's own reason line, when the row gives one.
	Note string `json:"note,omitempty"`
	// Fresh is the card's freshness strip (Recently Added, New Release).
	Fresh *explore.Fresh `json:"fresh,omitempty"`
	// Badge names a new release (New album, New EP, Album coming).
	Badge string `json:"badge,omitempty"`
	// Kicker is a Spotlight card's label above its title (New album, Climbing).
	Kicker string `json:"kicker,omitempty"`
	// Following: the member waits for this album (New & Popular, Coming soon).
	Following bool `json:"following,omitempty"`
	// Explicit: the release has explicit lyrics (the "E" mark; Deezer says so).
	Explicit bool `json:"explicit,omitempty"`
	// Track names a song on a Fresh tracks line; the card asks for its album.
	Track string `json:"track,omitempty"`
	// Hue is a genre tile's colour index (music_rows_v2).
	Hue *int `json:"hue,omitempty"`
	// Images is a genre tile's slideshow of artist photos (Image is the first).
	Images []string `json:"images,omitempty"`
	// Cover is a Browse tile's tilted album cover (music_lists_v1): the
	// genre's top charted album not used by another tile.
	Cover string `json:"cover,omitempty"`
	// Href is a Browse tile's page when it is not a genre page (row
	// templates: moods, seasons, decades).
	Href string `json:"href,omitempty"`
}

// MusicRow is one row of Explore â†’ Music.
type MusicRow struct {
	ID     string      `json:"id"`
	Type   string      `json:"type"`
	Title  string      `json:"title"`
	Reason string      `json:"reason"`
	Shape  string      `json:"shape"` // square, round
	Items  []MusicCard `json:"items"`
}

type musicGenreEntry struct {
	genres []string
	links  []metadata.ArtistLink
	at     time.Time
}

type musicSimilarEntry struct {
	list []metadata.SimilarArtist
	ok   bool
	at   time.Time
	done chan struct{}
}

// musicExplore keeps artists' genres and similar lists between requests.
type musicExplore struct {
	mu      sync.Mutex
	genres  map[string]musicGenreEntry
	queue   []string
	queued  map[string]bool
	running bool
	lookups []time.Time
	similar map[string]*musicSimilarEntry
}

type genreSource interface {
	ArtistGenres(ctx context.Context, mbid string) ([]string, error)
}

// genresOf answers the known genres of each id and queues the rest for a
// background MusicBrainz lookup.
func (a *API) genresOf(ids []string) map[string][]string {
	c := &a.musicx
	out := map[string][]string{}
	src, _ := a.genreProvider()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.genres == nil {
		c.genres, c.queued = map[string]musicGenreEntry{}, map[string]bool{}
	}
	for _, id := range ids {
		if id == "" {
			continue
		}
		e, ok := c.genres[id]
		if ok {
			out[id] = e.genres
		}
		if (!ok || time.Since(e.at) > musicGenreTTL) && src != nil && !c.queued[id] && len(c.queue) < 400 {
			c.queue = append(c.queue, id)
			c.queued[id] = true
		}
	}
	if src != nil && len(c.queue) > 0 && !c.running {
		c.running = true
		go a.fillGenres(src)
	}
	return out
}

func (a *API) genreProvider() (genreSource, bool) {
	if a.s.Library == nil || a.s.Library.Artists == nil {
		return nil, false
	}
	g, ok := a.s.Library.Artists.(genreSource)
	return g, ok
}

// fillGenres looks the queued artists up one at a time (MusicBrainz's own
// limiter spaces them), at most musicGenreLookups an hour.
func (a *API) fillGenres(src genreSource) {
	c := &a.musicx
	for {
		c.mu.Lock()
		now := time.Now()
		kept := c.lookups[:0]
		for _, t := range c.lookups {
			if now.Sub(t) < time.Hour {
				kept = append(kept, t)
			}
		}
		c.lookups = kept
		if len(c.queue) == 0 || len(c.lookups) >= musicGenreLookups {
			c.running = false
			c.mu.Unlock()
			return
		}
		id := c.queue[0]
		c.queue = c.queue[1:]
		c.lookups = append(c.lookups, now)
		c.mu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		var g []string
		var links []metadata.ArtistLink
		var err error
		if gl, ok := src.(genreLinkSource); ok {
			g, links, err = gl.ArtistGenresLinks(ctx, id)
		} else {
			g, err = src.ArtistGenres(ctx, id)
		}
		cancel()
		c.mu.Lock()
		delete(c.queued, id)
		if err == nil {
			if len(c.genres) >= musicCacheMax {
				c.genres = map[string]musicGenreEntry{}
			}
			c.genres[id] = musicGenreEntry{genres: g, links: links, at: time.Now()}
		}
		c.mu.Unlock()
	}
}

// similarTo reads the similar artists of each seed, waiting at most wait
// for answers not cached yet (they keep filling in the background).
func (a *API) similarTo(seeds []string, wait time.Duration) map[string][]metadata.SimilarArtist {
	out := map[string][]metadata.SimilarArtist{}
	if a.s.MusicSimilar == nil {
		return out
	}
	c := &a.musicx
	// pending holds the done channels, taken under the lock: fetchSimilar
	// clears e.done under the lock, so reading it later would race.
	var pending []chan struct{}
	c.mu.Lock()
	if c.similar == nil {
		c.similar = map[string]*musicSimilarEntry{}
	}
	entries := map[string]*musicSimilarEntry{}
	for _, id := range seeds {
		e := c.similar[id]
		fresh := e != nil && (e.done == nil) && ((e.ok && time.Since(e.at) < musicSimilarTTL) || (!e.ok && time.Since(e.at) < time.Hour))
		if e == nil || (!fresh && e.done == nil) {
			if len(c.similar) >= musicCacheMax {
				c.similar = map[string]*musicSimilarEntry{}
			}
			e = &musicSimilarEntry{done: make(chan struct{})}
			c.similar[id] = e
			go a.fetchSimilar(id, e)
		}
		if e.done != nil {
			pending = append(pending, e.done)
		}
		entries[id] = e
	}
	c.mu.Unlock()
	// One deadline for all: a timer's channel fires once, so after it the
	// loop would wait on every other pending seed in full.
	timer := time.NewTimer(wait)
	defer timer.Stop()
wait:
	for _, done := range pending {
		select {
		case <-done:
		case <-timer.C:
			break wait
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, e := range entries {
		if e.done == nil && e.ok {
			out[id] = e.list
		}
	}
	return out
}

func (a *API) fetchSimilar(id string, e *musicSimilarEntry) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	list, err := a.s.MusicSimilar(ctx, id)
	c := &a.musicx
	c.mu.Lock()
	e.list, e.ok, e.at = list, err == nil, time.Now()
	done := e.done
	e.done = nil
	c.mu.Unlock()
	close(done)
}

// musicSimilarWait is how long a Music answer waits for similar artists
// not read yet; they keep filling for the next visit.
const musicSimilarWait = 1500 * time.Millisecond

// musicArtist is one artist in the Library.
type musicArtist struct {
	id     int64
	name   string
	mbid   string
	image  string
	files  int
	added  time.Time
	genres []string
}

// artistImage is the picture link of a library artist: the picture route
// (the metadata server's sharp photo first) when the artist has a
// MusicBrainz id, with the saved library artwork as the route's fallback;
// without an id, the saved artwork alone.
func artistImage(t domain.Title) string {
	saved := ""
	if l := artworkLinks(t.Artwork); len(l) > 0 && strings.HasPrefix(l[0], "/api/v1/artwork/") {
		saved = l[0]
	}
	mbid := strings.ToLower(t.SourceIDs["musicbrainz"])
	if !metadata.ValidMBID(mbid) {
		return saved
	}
	if saved == "" {
		return musicArtPrefix + "artist/" + mbid
	}
	return musicArtPrefix + "artist/" + mbid + "?fallback=" + url.QueryEscape(saved)
}

func dayOf(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format("2006-01-02")
}

// exploreMusic answers GET /api/v1/explore/music?genre=<tag>&region=<code>.
func (a *API) exploreMusic(w http.ResponseWriter, r *http.Request) {
	if p, _ := parts.ForTitleKind("artist"); !a.partIsOn(p) {
		fail(w, http.StatusNotFound, "music is turned off")
		return
	}
	// Music has no ratings, so a Kid limit shows none of it.
	if a.kidMax(r) != "" {
		writeJSON(w, http.StatusOK, map[string]any{"kid": true, "cold": false, "genres": []string{}, "genre": "", "hero": nil, "rows": []MusicRow{}})
		return
	}
	if !a.discover.allow(a.person(r).UserID, time.Now()) {
		w.Header().Set("Retry-After", "60")
		fail(w, http.StatusTooManyRequests, "too many Explore requests; try again in a minute")
		return
	}
	if a.s.MusicRowsV2 && a.s.MusicRowsV3 && a.s.MusicRowsV7 {
		a.exploreMusicV7(w, r)
		return
	}
	if a.s.MusicRowsV2 {
		a.exploreMusicV2(w, r)
		return
	}
	q := r.URL.Query()
	genre := strings.ToLower(strings.TrimSpace(q.Get("genre")))
	region := strings.ToUpper(q.Get("region"))
	if region == "" || region == "GLOBAL" || !isCountry(region) {
		region = "global"
	}
	ctx := r.Context()
	db := a.s.Store.Read()
	now := time.Now().UTC()
	key := musicAnswerKey(a.person(r).UserID, genre, region, musicHour(q.Get("hour"), now))
	if body, ok := a.cachedMusicAnswer(key, now); ok {
		a.exploreCaching(w, r, "private, max-age=60")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
		return
	}

	titles, err := store.Select[domain.Title](ctx, db, "titles", "kind = 'artist' AND state != 'removed' ORDER BY id")
	if err != nil {
		internal(w, err)
		return
	}
	files := map[int64]int{}
	rows, err := db.QueryContext(ctx, `SELECT i.title_id, COUNT(*) FROM items i JOIN slots s ON s.item_id = i.id
		WHERE i.kind = 'track' AND s.file_id IS NOT NULL GROUP BY i.title_id`)
	if err != nil {
		internal(w, err)
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
	st := newServerTiming()
	similar := a.similarTo(seedIDs, musicSimilarWait)
	st.mark("similar")

	// Charts, from the cached trending lists.
	var trendArtists, trendAlbums []MusicCard
	regions := []string{}
	if a.s.MusicTrending != nil {
		for _, kind := range []string{"artist", "album"} {
			l, err := a.cachedMusicTrend(ctx, kind, "global")
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
	st.mark("charts")
	genres, genresDone := a.genresWaiting(ids, musicGenreWait)
	st.mark("genres")
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
	albumCard := func(m *musicArtist, p domain.Part) MusicCard {
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
		ps, err := store.Select[domain.Part](ctx, db, "parts", `kind = 'album' AND dropped_at IS NULL AND release_date >= ? AND release_date < ?
			AND lower(coalesce(release_type, '')) IN ('album', 'ep', 'single') AND secondary_types = '[]' AND coalesce(release_date_precision, 'day') != 'year'
			AND title_id IN (SELECT id FROM titles WHERE kind = 'artist' AND state != 'removed') ORDER BY release_date DESC, id`,
			now.AddDate(0, 0, -90).Format("2006-01-02"), now.AddDate(0, 0, 180).Format("2006-01-02"))
		if err != nil {
			internal(w, err)
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
			h := c
			h.ArtistImage = m.image
			switch strings.ToLower(derefStr(p.ReleaseType)) {
			case "album":
				heroAlbums = append(heroAlbums, h)
			case "ep":
				heroEPs = append(heroEPs, h)
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
			c.ArtistImage = "/api/v1/music-trending/art/artist/" + c.ArtistMBID
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
		ps, err := store.Select[domain.Part](ctx, db, "parts", `kind = 'album' AND dropped_at IS NULL AND release_date IS NOT NULL AND release_date <= ?
			AND lower(coalesce(release_type, '')) = 'album' AND secondary_types = '[]'
			AND title_id IN (SELECT id FROM titles WHERE kind = 'artist' AND state != 'removed')
			AND NOT EXISTS (SELECT 1 FROM items i JOIN slots s ON s.item_id = i.id WHERE i.part_id = parts.id AND s.file_id IS NOT NULL)
			ORDER BY release_date DESC, id`, now.Format("2006-01-02T15:04:05Z"))
		if err != nil {
			internal(w, err)
			return
		}
		by := map[int64][]domain.Part{}
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

	out = a.moreMusicRows(ctx, &musicCtx{now: now, hour: musicHour(q.Get("hour"), now), region: region, artists: artists, byTitle: byTitle, owned: owned,
		genres: genres, trendAlbums: trendAlbums, trendArtists: trendArtists, pool: pool, top: top, tag: tag, albumCard: albumCard, names: &names, cold: cold}, out)
	st.mark("more")

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
		if len(final) < explore.HeadDedupe {
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
			row.Items[i].Fresh = a.musicFresh(row.Type, row.Items[i])
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
	a.rememberMusicNames(names)
	a.exploreCaching(w, r, "private, max-age=60")
	complete := genresDone && len(similar) == len(seedIDs)
	st.write(w)
	a.writeMusicAnswer(w, key, map[string]any{"kid": false, "cold": cold, "genres": top, "genre": genre, "region": region, "regions": regions, "hero": hero, "hero_alts": heroAlts, "rows": final}, complete, now)
}

type nameKey struct{ kind, mbid, name, artist, cover string }

// rememberMusicNames lets the picture route serve these albums and artists.
func (a *API) rememberMusicNames(list []nameKey) {
	a.rememberNames(list)
	// Every row and page that lists them also gets their pictures stored.
	a.warmArtItems(list)
}

func (a *API) rememberNames(list []nameKey) {
	c := &a.music
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.names == nil {
		c.lists, c.names = map[string]musicTrendEntry{}, map[string]metadata.MusicTrendItem{}
	}
	if len(c.names) > 20000 {
		c.names = map[string]metadata.MusicTrendItem{}
	}
	for _, n := range list {
		if n.mbid == "" {
			continue
		}
		k := n.kind + "|" + n.mbid
		old, ok := c.names[k]
		if !ok {
			c.names[k] = metadata.MusicTrendItem{MBID: n.mbid, Name: n.name, Title: n.name, Artist: n.artist, Cover: n.cover}
			continue
		}
		// A later list may know what the first did not (the artist of a
		// soundtrack, a song's cover): the name searches need the artist.
		if old.Artist == "" && n.artist != "" {
			old.Artist = n.artist
		}
		if old.Cover == "" && n.cover != "" {
			old.Cover = n.cover
		}
		if old.Name == "" && old.Title == "" {
			old.Name, old.Title = n.name, n.name
		}
		c.names[k] = old
	}
}

// musicArtPrefix is the picture route every music card points at.
const musicArtPrefix = "/api/v1/music-trending/art/"

// rememberMusicCards lets the picture route serve every album and artist
// these rows show, then fetches their pictures in the background, so the
// cards and circles get art from the same cover chain as the charts.
func (a *API) rememberMusicCards(rows []MusicRow, extra ...MusicCard) {
	var names []nameKey
	add := func(img, name, artist string) {
		rest, ok := strings.CutPrefix(img, musicArtPrefix)
		if !ok {
			return
		}
		kind, mbid, _ := strings.Cut(rest, "/")
		if kind == "artist" {
			artist = ""
		}
		names = append(names, nameKey{kind: kind, mbid: mbid, name: name, artist: artist})
	}
	card := func(c MusicCard) {
		name := c.Name
		if c.Kind == "song" {
			return // a song's picture is its own cover link
		}
		add(c.Image, name, c.Artist)
		add(c.ArtistImage, c.Artist, "")
		if c.ArtistMBID != "" && c.Artist != "" {
			add(musicArtPrefix+"artist/"+c.ArtistMBID, c.Artist, "")
		}
	}
	for _, r := range rows {
		for _, c := range r.Items {
			card(c)
		}
	}
	for _, c := range extra {
		card(c)
	}
	a.rememberMusicNames(names)
	a.warmMusicNames(names)
}

// musicWarmTTL is how long a queued card picture is not queued again.
const musicWarmTTL = 10 * time.Minute

// warmMusicNames fetches the pictures of these names into the artwork cache
// in the background (albums at 250 and 500, artists at 250), two at a time.
func (a *API) warmMusicNames(list []nameKey) {
	if !a.partIsOn(parts.Music) || a.s.Library == nil || a.s.Library.Artwork == nil {
		return
	}
	c := &a.music
	now := time.Now()
	var albums, artists []metadata.MusicTrendItem
	c.mu.Lock()
	if c.warmed == nil || len(c.warmed) > 20000 {
		c.warmed = map[string]time.Time{}
	}
	for _, n := range list {
		k := n.kind + "|" + n.mbid
		if at, ok := c.warmed[k]; (ok && now.Sub(at) < musicWarmTTL) || n.mbid == "" {
			continue
		}
		c.warmed[k] = now
		switch n.kind {
		case "album":
			albums = append(albums, metadata.MusicTrendItem{MBID: n.mbid})
		case "artist":
			artists = append(artists, metadata.MusicTrendItem{MBID: n.mbid})
		}
	}
	c.mu.Unlock()
	if len(albums)+len(artists) == 0 {
		return
	}
	c.prefetching.Add(1)
	go func() {
		defer c.prefetching.Done()
		a.prefetchMusicArt("album", albums)
		a.prefetchMusicArt("artist", artists)
	}()
}

func chartReason(region, what string) string {
	if region == "global" {
		return what + " this week, worldwide"
	}
	return what + " this week in " + region
}

func topGenres(count map[string]int, n int) []string {
	list := []string{}
	for g, c := range count {
		if c > 0 {
			list = append(list, g)
		}
	}
	sort.Slice(list, func(i, j int) bool {
		if count[list[i]] != count[list[j]] {
			return count[list[i]] > count[list[j]]
		}
		return list[i] < list[j]
	})
	return list[:min(n, len(list))]
}

func sortedPool(pool map[string]MusicCard) []MusicCard {
	out := make([]MusicCard, 0, len(pool))
	for _, c := range pool {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		ri, rj := out[i].Rank, out[j].Rank
		if (ri == 0) != (rj == 0) {
			return ri == 0 // similar artists (personal) first
		}
		if ri != rj {
			return ri < rj
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func hasGenre(list []string, g string) bool {
	for _, x := range list {
		if x == g {
			return true
		}
	}
	return false
}

func uniqCards(list []MusicCard) []MusicCard {
	seen := map[string]bool{}
	out := make([]MusicCard, 0, len(list))
	for _, c := range list {
		if !seen[c.Key] {
			seen[c.Key] = true
			out = append(out, c)
		}
	}
	return out
}

func titleCase(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
