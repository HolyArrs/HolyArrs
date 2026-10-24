package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/parts"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// New & Popular → Music: new albums, EPs and singles to ask for (or add).
// MusicBrainz release dates say what is new, the trending lists what is
// big; the lists are built in the background and kept 12 hours, so a
// visit never waits on MusicBrainz.

const (
	// Per-kind windows, in days: this week, recently out, singles, coming.
	musicNewWeek    = 7
	musicNewRecent  = 45
	musicNewLibrary = 90
	musicNewSingles = 21
	musicNewAhead   = 60
	// musicNewRowMin is the fewest cards a row shows; musicNewRowMax the most.
	musicNewRowMin  = 6
	musicNewRowMax  = 20
	musicNewHeroMax = 15
	musicNewTTL     = 12 * time.Hour
	// musicNewArtists caps the artists one MusicBrainz search names.
	musicNewArtists = 40
)

// musicNewData is one build of the lists every member shares.
type musicNewData struct {
	at       time.Time
	releases []metadata.NewRelease // albums, EPs and singles, out or coming
	fresh    metadata.MusicTrend   // notable new albums, with listens
	albums   metadata.MusicTrend   // this week's album chart
	artists  metadata.MusicTrend   // this week's artist chart
	// server is the metadata server's new music windows (ListenBrainz fresh
	// releases and Deezer's editorial and charts), by window.
	server map[string][]metadata.MusicNewItem
}

type musicNewStore struct {
	mu       sync.Mutex
	data     *musicNewData
	building bool
	landed   time.Time
}

var musicNew musicNewStore

type releaseSource interface {
	ReleasesBetween(ctx context.Context, from, to string, types, artists []string, limit int) ([]metadata.NewRelease, error)
}

// warmMusicNew starts a build when there is none or it is older than the
// TTL; it never waits.
func (a *API) warmMusicNew() *musicNewData {
	if p, _ := parts.ForTitleKind("artist"); !a.partIsOn(p) {
		return nil
	}
	a.loadMusicNew()
	musicNew.mu.Lock()
	d := musicNew.data
	stale := (d == nil || time.Since(d.at) > musicNewTTL) && !musicNew.building
	musicNew.mu.Unlock()
	if stale {
		go a.refreshMusicNew(context.Background())
	}
	return d
}

// buildMusicNew reads the trending lists, then asks MusicBrainz (paced by
// its shared limiter) for releases by notable artists in each window.
func (a *API) buildMusicNew(ctx context.Context, now time.Time) *musicNewData {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	d := &musicNewData{at: now}
	if a.s.MusicTrending != nil {
		d.fresh, _ = a.cachedMusicTrend(ctx, "fresh", "global")
		d.albums, _ = a.cachedMusicTrend(ctx, "album", "global")
		d.artists, _ = a.cachedMusicTrend(ctx, "artist", "global")
	}
	if a.s.MusicNew != nil {
		d.server = map[string][]metadata.MusicNewItem{}
		for _, w := range []string{"tracks", "week", "recent", "upcoming"} {
			if l, err := a.s.MusicNew(ctx, w); err == nil {
				d.server[w] = l.Items
			}
		}
	}
	var src releaseSource
	if a.s.Library != nil && a.s.Library.Artists != nil {
		src, _ = a.s.Library.Artists.(releaseSource)
	}
	if src == nil {
		return d
	}
	// Notable artists: the Library's, then the charts'.
	var ids []string
	seen := map[string]bool{}
	addID := func(id string) {
		id = strings.ToLower(id)
		if metadata.ValidMBID(id) && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if rows, err := a.s.Store.Read().QueryContext(ctx, `SELECT json_extract(source_ids, '$.musicbrainz') FROM titles
		WHERE kind = 'artist' AND state != 'removed' AND json_extract(source_ids, '$.musicbrainz') IS NOT NULL ORDER BY id DESC LIMIT 80`); err == nil {
		for rows.Next() {
			var id string
			if rows.Scan(&id) == nil {
				addID(id)
			}
		}
		rows.Close()
	}
	for _, l := range []metadata.MusicTrend{d.artists, d.fresh, d.albums} {
		for _, it := range l.Items {
			if l.Kind == "artist" {
				addID(it.MBID)
			} else {
				addID(it.ArtistMBID)
			}
		}
	}
	if len(ids) > 3*musicNewArtists {
		ids = ids[:3*musicNewArtists]
	}
	day := func(n int) string { return now.AddDate(0, 0, n).Format("2006-01-02") }
	windows := []struct {
		from, to string
		types    []string
	}{
		{day(-musicNewLibrary), day(0), []string{"album", "ep"}},
		{day(-musicNewLibrary), day(0), []string{"single"}},
		{day(1), day(musicNewAhead), []string{"album", "ep"}},
	}
	have := map[string]bool{}
	for _, w := range windows {
		for i := 0; i < len(ids); i += musicNewArtists {
			list, err := src.ReleasesBetween(ctx, w.from, w.to, w.types, ids[i:min(i+musicNewArtists, len(ids))], 100)
			if err != nil {
				if errors.Is(err, metadata.ErrServiceBusy) || ctx.Err() != nil {
					return d
				}
				continue
			}
			for _, r := range list {
				if !have[r.MBID] {
					have[r.MBID] = true
					d.releases = append(d.releases, r)
				}
			}
		}
	}
	return d
}

// musicNewInput is what one member's page is built from.
type musicNewInput struct {
	data    *musicNewData
	owned   map[string]int64 // artist MBID → title id
	images  map[string]string
	library []MusicCard // Library releases out in the last 90 days
	follows map[string]bool
}

func daysFrom(date string, now time.Time) (int, bool) {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return 0, false
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return int(today.Sub(t).Hours() / 24), true
}

// musicNewScore is newness × popularity: newness halves after a week,
// popularity grows with listens and with a charting or Library artist.
func musicNewScore(c MusicCard, artistBoost float64, now time.Time) float64 {
	days, ok := daysFrom(c.Date, now)
	if !ok || days < 0 {
		return 0
	}
	newness := 1 / (1 + float64(days)/musicNewWeek)
	pop := 1 + math.Log10(1+float64(c.Listens)) + artistBoost
	return newness * pop
}

// musicNewWindow says which row an out release belongs to by its age.
func musicNewWindow(date string, now time.Time) string {
	days, ok := daysFrom(date, now)
	switch {
	case !ok:
		return ""
	case days < -musicNewAhead:
		return ""
	case days < 0:
		return "coming"
	case days <= musicNewWeek:
		return "week"
	case days <= musicNewRecent:
		return "recent"
	case days <= musicNewLibrary:
		return "library"
	}
	return ""
}

func releaseBadge(kind string, upcoming bool) string {
	switch {
	case upcoming && kind == "ep":
		return "EP coming"
	case upcoming:
		return "Album coming"
	case kind == "ep":
		return "New EP"
	case kind == "single":
		return "New single"
	}
	return "New album"
}

// composeMusicNew builds the hero and rows: scored, deduped across rows
// (hero, Library, this week, recent, singles, climbing, coming), and a
// row under musicNewRowMin left out.
func composeMusicNew(in musicNewInput, now time.Time) ([]MusicCard, []MusicRow) {
	boost := map[string]float64{}
	if in.data != nil {
		for _, it := range in.data.artists.Items {
			if it.Rank > 0 {
				boost[strings.ToLower(it.MBID)] = math.Max(boost[strings.ToLower(it.MBID)], 2*(1-float64(it.Rank)/200))
			}
		}
	}
	for id := range in.owned {
		boost[id] += 1
	}
	listens := map[string]int64{}
	cards := map[string]MusicCard{}
	card := func(mbid, name, artist, artistMBID, date, kind string, tags []string) MusicCard {
		c := MusicCard{Key: "album:" + mbid, Kind: "album", MBID: mbid, Name: name, Artist: artist, ArtistMBID: artistMBID, Date: date,
			Image: "/api/v1/music-trending/art/album/" + mbid, Genres: append([]string{}, tags...)}
		if c.Genres == nil {
			c.Genres = []string{}
		}
		if id, ok := in.owned[artistMBID]; ok {
			c.TitleID = id
		}
		if img := in.images[artistMBID]; img != "" {
			c.ArtistImage = img
		} else {
			c.ArtistImage = "/api/v1/music-trending/art/artist/" + artistMBID
		}
		days, _ := daysFrom(date, now)
		c.Upcoming = days < 0
		c.Badge = releaseBadge(kind, c.Upcoming)
		c.Following = in.follows[mbid]
		return c
	}
	kinds := map[string]string{}
	// pop lifts a release Deezer's editorial or charts rank (position 1 most).
	pop := map[string]float64{}
	explicit := map[string]bool{}
	if in.data != nil {
		for _, l := range []metadata.MusicTrend{in.data.fresh, in.data.albums} {
			for _, it := range l.Items {
				listens[strings.ToLower(it.MBID)] += it.Listens
			}
		}
		for _, r := range in.data.releases {
			if len(r.SecondaryTypes) > 0 {
				continue // live, compilation, remix: not new music
			}
			cards[r.MBID] = card(r.MBID, r.Name, r.Artist, r.ArtistMBID, r.ReleaseDate, r.PrimaryType, r.Tags)
			kinds[r.MBID] = r.PrimaryType
		}
		for _, it := range in.data.fresh.Items {
			id := strings.ToLower(it.MBID)
			if _, ok := cards[id]; ok || len(it.ReleaseDate) != 10 || !metadata.ValidMBID(id) || it.Title == "" {
				continue
			}
			cards[id] = card(id, it.Title, it.Artist, strings.ToLower(it.ArtistMBID), it.ReleaseDate, "album", it.Tags)
			kinds[id] = "album"
		}
		for _, w := range []string{"week", "recent", "upcoming"} {
			for _, it := range in.data.server[w] {
				id := strings.ToLower(it.MBID)
				if !metadata.ValidMBID(id) || it.Title == "" || musicNewWindow(it.ReleaseDate, now) == "" {
					continue
				}
				if it.Popularity > 0 {
					pop[id] = math.Max(pop[id], math.Max(0.2, 2*(1-float64(it.Popularity)/200)))
				}
				explicit[id] = explicit[id] || it.Explicit
				listens[id] += it.Listens
				if _, ok := cards[id]; ok {
					continue
				}
				kind := strings.ToLower(it.Type)
				if kind != "ep" && kind != "single" {
					kind = "album"
				}
				cards[id] = card(id, it.Title, it.Artist, strings.ToLower(it.ArtistMBID), it.ReleaseDate, kind, nil)
				kinds[id] = kind
			}
		}
	}
	for _, c := range in.library {
		if _, ok := cards[c.MBID]; !ok || c.MBID == "" {
			k := c.MBID
			if k == "" {
				k = c.Key
			}
			kind := strings.ToLower(c.Badge)
			c.Badge = releaseBadge(kind, false)
			c.Following = in.follows[c.MBID]
			cards[k] = c
			kinds[k] = kind
		} else {
			lc := cards[c.MBID]
			lc.TitleID, lc.PartID, lc.Key = c.TitleID, c.PartID, c.Key
			cards[c.MBID] = lc
		}
	}
	score := map[string]float64{}
	for k, c := range cards {
		c.Listens = listens[c.MBID]
		c.Explicit = c.Explicit || explicit[c.MBID]
		cards[k] = c
		score[k] = musicNewScore(c, boost[c.ArtistMBID]+pop[c.MBID], now)
	}
	sorted := func(keep func(k string, c MusicCard) bool, by func(a, b string) bool) []string {
		var ks []string
		for k, c := range cards {
			if keep(k, c) {
				ks = append(ks, k)
			}
		}
		sort.Slice(ks, func(i, j int) bool {
			if by != nil && by(ks[i], ks[j]) != by(ks[j], ks[i]) {
				return by(ks[i], ks[j])
			}
			if score[ks[i]] != score[ks[j]] {
				return score[ks[i]] > score[ks[j]]
			}
			return ks[i] < ks[j]
		})
		return ks
	}
	seen := map[string]bool{}
	take := func(ks []string, max int) []MusicCard {
		out := []MusicCard{}
		for _, k := range ks {
			if len(out) >= max {
				break
			}
			if seen[cards[k].Key] || seen[cards[k].MBID] {
				continue
			}
			out = append(out, cards[k])
		}
		return out
	}
	mark := func(l []MusicCard) {
		for _, c := range l {
			seen[c.Key], seen[c.MBID] = true, true
		}
	}
	albumOrEP := func(k string) bool { return kinds[k] == "album" || kinds[k] == "ep" }

	// Hero: the biggest new albums and EPs of the week (of the month when
	// the week is thin), only ones a chart or Library knows.
	big := func(k string, c MusicCard, win string) bool {
		w := musicNewWindow(c.Date, now)
		return albumOrEP(k) && (w == "week" || (win == "recent" && w == "recent")) && (c.Listens > 0 || boost[c.ArtistMBID] > 0 || pop[c.MBID] > 0)
	}
	hero := take(sorted(func(k string, c MusicCard) bool { return big(k, c, "week") }, nil), musicNewHeroMax)
	if len(hero) < 3 {
		hero = take(sorted(func(k string, c MusicCard) bool { return big(k, c, "recent") }, nil), musicNewHeroMax)
	}
	if len(hero) < 3 {
		hero = []MusicCard{}
	}
	mark(hero)

	type rowSpec struct {
		id, title, reason string
		keep              func(k string, c MusicCard) bool
		by                func(a, b string) bool
	}
	soonest := func(a, b string) bool { return cards[a].Date < cards[b].Date }
	specs := []rowSpec{
		{"np_music_library", "New from artists in your Library", "Albums, EPs and singles from artists you have, out in the last 90 days",
			func(k string, c MusicCard) bool {
				w := musicNewWindow(c.Date, now)
				return c.TitleID > 0 && (w == "week" || w == "recent" || w == "library")
			}, nil},
		{"np_music_week", "Out this week", "Albums and EPs out in the last 7 days, biggest first",
			func(k string, c MusicCard) bool { return albumOrEP(k) && musicNewWindow(c.Date, now) == "week" }, nil},
		{"np_music_recent", "Recently out", "Albums and EPs out 8 to 45 days ago",
			func(k string, c MusicCard) bool { return albumOrEP(k) && musicNewWindow(c.Date, now) == "recent" }, nil},
		{"np_music_tracks", "Fresh tracks", "New singles from artists people play",
			func(k string, c MusicCard) bool {
				d, ok := daysFrom(c.Date, now)
				return kinds[k] == "single" && ok && d >= 0 && d <= musicNewSingles
			}, nil},
		{"np_music_coming", "Coming soon", "Albums out in the next 60 days; follow one to hear when it lands",
			func(k string, c MusicCard) bool { return albumOrEP(k) && c.Upcoming }, soonest},
	}
	built := map[string]MusicRow{}
	// Fresh tracks: Deezer's track chart (new albums first) when the
	// metadata server has it; each line asks for the track's album.
	if in.data != nil && len(in.data.server["tracks"]) > 0 {
		var items []MusicCard
		for _, it := range in.data.server["tracks"] {
			id := strings.ToLower(it.MBID)
			if len(items) >= musicNewRowMax || !metadata.ValidMBID(id) || it.Track == "" || seen["track:"+id+":"+it.Track] {
				continue
			}
			c, ok := cards[id]
			if !ok {
				c = card(id, it.Title, it.Artist, strings.ToLower(it.ArtistMBID), it.ReleaseDate, "single", nil)
			}
			c.Key, c.Name, c.Track, c.Note = "track:"+id+":"+it.Track, it.Track, it.Track, "From "+it.Title
			c.Explicit, c.Rank = c.Explicit || it.Explicit, it.Popularity
			seen[c.Key] = true
			items = append(items, c)
		}
		if len(items) >= musicNewRowMin {
			built["np_music_tracks"] = MusicRow{ID: "np_music_tracks", Type: "np_music_tracks", Title: "Fresh tracks", Reason: "The most played songs right now, new ones first", Shape: "square", Items: items}
		}
	}
	for _, s := range specs {
		if _, ok := built[s.id]; ok {
			continue
		}
		items := take(sorted(s.keep, s.by), musicNewRowMax)
		if len(items) < musicNewRowMin {
			continue
		}
		mark(items)
		built[s.id] = MusicRow{ID: s.id, Type: s.id, Title: s.title, Reason: s.reason, Shape: "square", Items: items}
	}
	// Singles & EPs: the short releases the rows above left.
	if items := take(sorted(func(k string, c MusicCard) bool {
		d, ok := daysFrom(c.Date, now)
		return (kinds[k] == "single" || kinds[k] == "ep") && ok && d >= 0 && d <= musicNewRecent
	}, nil), musicNewRowMax); len(items) >= musicNewRowMin {
		mark(items)
		built["np_music_short"] = MusicRow{ID: "np_music_short", Type: "np_music_short", Title: "Singles & EPs", Reason: "New singles and EPs from the last 45 days", Shape: "square", Items: items}
	}
	// New in <genre>: the household's top genres (its artists' releases,
	// else all new releases), one row each.
	var genreRows []string
	for _, g := range musicNewTopGenres(cards, 3) {
		items := take(sorted(func(k string, c MusicCard) bool {
			w := musicNewWindow(c.Date, now)
			if w != "week" && w != "recent" && w != "library" {
				return false
			}
			for _, t := range c.Genres {
				if strings.EqualFold(t, g) {
					return true
				}
			}
			return false
		}, nil), musicNewRowMax)
		if len(items) < musicNewRowMin {
			continue
		}
		mark(items)
		id := "np_music_genre_" + musicSlug(g)
		built[id] = MusicRow{ID: id, Type: "np_music_genre", Title: "New in " + g, Reason: "New releases tagged " + g, Shape: "square", Items: items}
		genreRows = append(genreRows, id)
	}
	// Climbing now: this week's album chart, the recent ones first.
	if in.data != nil {
		var climb []MusicCard
		var later []MusicCard
		for _, it := range in.data.albums.Items {
			id := strings.ToLower(it.MBID)
			if !metadata.ValidMBID(id) || seen[id] || it.Title == "" {
				continue
			}
			c, ok := cards[id]
			if !ok {
				c = card(id, it.Title, it.Artist, strings.ToLower(it.ArtistMBID), it.ReleaseDate, "album", it.Tags)
				c.Badge = ""
			}
			c.Listens, c.Rank = it.Listens, it.Rank
			if w := musicNewWindow(c.Date, now); w == "week" || w == "recent" {
				climb = append(climb, c)
			} else {
				later = append(later, c)
			}
		}
		climb = append(climb, later...)
		if len(climb) > musicNewRowMax {
			climb = climb[:musicNewRowMax]
		}
		if len(climb) >= musicNewRowMin {
			mark(climb)
			built["np_music_climbing"] = MusicRow{ID: "np_music_climbing", Type: "np_music_climbing", Title: "Climbing now", Reason: "The most played albums this week, new ones first", Shape: "square", Items: climb}
		}
	}
	rows := []MusicRow{}
	order := []string{"np_music_tracks", "np_music_library", "np_music_week", "np_music_recent", "np_music_climbing", "np_music_short"}
	order = append(append(order, genreRows...), "np_music_coming")
	for _, id := range order {
		if r, ok := built[id]; ok {
			rows = append(rows, r)
		}
	}
	return hero, rows
}

// musicNewTopGenres are the most common tags of the Library artists'
// new releases, else of all new releases; ties by name, so stable.
func musicNewTopGenres(cards map[string]MusicCard, n int) []string {
	count := func(lib bool) []string {
		hits := map[string]int{}
		name := map[string]string{}
		for _, c := range cards {
			if lib && c.TitleID == 0 {
				continue
			}
			for _, t := range c.Genres {
				k := strings.ToLower(strings.TrimSpace(t))
				if k == "" {
					continue
				}
				hits[k]++
				if name[k] == "" {
					name[k] = strings.ToUpper(k[:1]) + k[1:]
				}
			}
		}
		var ks []string
		for k := range hits {
			ks = append(ks, k)
		}
		sort.Slice(ks, func(i, j int) bool {
			if hits[ks[i]] != hits[ks[j]] {
				return hits[ks[i]] > hits[ks[j]]
			}
			return ks[i] < ks[j]
		})
		if len(ks) > n {
			ks = ks[:n]
		}
		for i := range ks {
			ks[i] = name[ks[i]]
		}
		return ks
	}
	if g := count(true); len(g) > 0 {
		return g
	}
	return count(false)
}

// musicNewTopics are the Explore more chips: the new releases' top genres,
// then decades and moods, each a Search → Music genre link.
func musicNewTopics(rows []MusicRow, hero []MusicCard) []string {
	count := map[string]int{}
	for _, c := range hero {
		for _, g := range c.Genres {
			count[g]++
		}
	}
	for _, r := range rows {
		for _, c := range r.Items {
			for _, g := range c.Genres {
				count[g]++
			}
		}
	}
	// Only short, plain tags: some releases carry whole sentences as tags.
	for g := range count {
		if len(g) > 24 || strings.ContainsAny(g, ":—,;()") {
			delete(count, g)
		}
	}
	return topGenres(count, 8)
}

// exploreNewMusic answers GET /api/v1/explore/new/music: New & Popular's
// Music tab. building says the shared lists are still on their way (ask
// again shortly); kid is true for an account with an age limit.
func (a *API) exploreNewMusic(w http.ResponseWriter, r *http.Request) {
	if p, _ := parts.ForTitleKind("artist"); !a.partIsOn(p) {
		fail(w, http.StatusNotFound, "music is turned off")
		return
	}
	empty := map[string]any{"kid": false, "building": false, "hero": []MusicCard{}, "rows": []MusicRow{}, "genres": []string{}}
	// Music has no ratings, so a Kid limit shows none of it (as on Explore → Music).
	if a.kidMax(r) != "" {
		empty["kid"] = true
		writeJSON(w, http.StatusOK, empty)
		return
	}
	if !a.discover.allow(a.person(r).UserID, time.Now()) {
		w.Header().Set("Retry-After", "60")
		fail(w, http.StatusTooManyRequests, "too many Explore requests; try again in a minute")
		return
	}
	ctx := r.Context()
	now := time.Now().UTC()
	a.landAlbumFollows(ctx, now)
	// The row template engine fills New & Popular → Music when it has its
	// catalog (kill switch music_rows_tpl_off keeps the list rows).
	if a.musicTplOn() {
		if hero, rows, ok := a.buildTplWorld(ctx, r, "new", now); ok {
			var names []nameKey
			for _, row := range append([]MusicRow{{Items: hero}}, rows...) {
				for _, c := range row.Items {
					if c.Kind == "album" {
						names = append(names, nameKey{"album", c.MBID, c.Name, c.Artist, ""})
					}
				}
			}
			a.rememberMusicNames(names)
			a.exploreCaching(w, r, "private, no-cache")
			writeJSON(w, http.StatusOK, map[string]any{"kid": false, "building": false, "hero": hero, "rows": rows, "genres": musicNewTopics(rows, hero)})
			return
		}
	}
	d := a.warmMusicNew()
	if d == nil {
		empty["building"] = true
		a.exploreCaching(w, r, "private, no-cache")
		writeJSON(w, http.StatusOK, empty)
		return
	}
	db := a.s.Store.Read()
	in := musicNewInput{data: d, owned: map[string]int64{}, images: map[string]string{}, follows: map[string]bool{}}
	titles, err := store.Select[domain.Title](ctx, db, "titles", "kind = 'artist' AND state != 'removed' ORDER BY id")
	if err != nil {
		internal(w, err)
		return
	}
	byTitle := map[int64]domain.Title{}
	for _, t := range titles {
		byTitle[t.ID] = t
		if id := strings.ToLower(t.SourceIDs["musicbrainz"]); id != "" {
			in.owned[id] = t.ID
			in.images[id] = artistImage(t)
		}
	}
	ps, err := store.Select[domain.Part](ctx, db, "parts", `kind = 'album' AND dropped_at IS NULL AND release_date >= ? AND release_date <= ?
		AND lower(coalesce(release_type, '')) IN ('album', 'ep', 'single') AND secondary_types = '[]' AND coalesce(release_date_precision, 'day') = 'day'
		AND title_id IN (SELECT id FROM titles WHERE kind = 'artist' AND state != 'removed') ORDER BY release_date DESC, id`,
		now.AddDate(0, 0, -musicNewLibrary).Format("2006-01-02"), now.Format("2006-01-02T15:04:05Z"))
	if err != nil {
		internal(w, err)
		return
	}
	for _, p := range ps {
		t := byTitle[p.TitleID]
		rg := strings.ToLower(p.SourceIDs["musicbrainz"])
		c := MusicCard{Key: "album:" + rg, Kind: "album", MBID: rg, Name: p.Name, Artist: t.Name, ArtistMBID: strings.ToLower(t.SourceIDs["musicbrainz"]),
			TitleID: t.ID, PartID: p.ID, Date: dayOf(p.ReleaseDate), Genres: []string{}, ArtistImage: artistImage(t), Badge: strings.ToLower(derefStr(p.ReleaseType))}
		if rg != "" {
			c.Image = "/api/v1/music-trending/art/album/" + rg
		} else {
			c.Key = "part:" + strings.TrimSpace(p.Name)
		}
		in.library = append(in.library, c)
	}
	if rows, err := db.QueryContext(ctx, "SELECT mbid FROM album_follows WHERE user_id = ?", a.person(r).UserID); err == nil {
		for rows.Next() {
			var id string
			if rows.Scan(&id) == nil {
				in.follows[id] = true
			}
		}
		rows.Close()
	}
	var hero []MusicCard
	var rows []MusicRow
	if a.s.MusicRowsV6 {
		hero, rows = a.musicTrendingRows(ctx, r, in, now)
	} else {
		hero, rows = composeMusicNew(in, now)
	}
	if a.s.MusicListsV1 {
		hero, rows = a.musicNewListRows(ctx, r, in, hero, rows, now)
	}
	var names []nameKey
	for _, c := range hero {
		names = append(names, nameKey{"album", c.MBID, c.Name, c.Artist, ""}, nameKey{"artist", c.ArtistMBID, c.Artist, "", ""})
	}
	for _, row := range rows {
		for _, c := range row.Items {
			names = append(names, nameKey{"album", c.MBID, c.Name, c.Artist, ""})
		}
	}
	a.rememberMusicNames(names)
	a.exploreCaching(w, r, "private, no-cache")
	writeJSON(w, http.StatusOK, map[string]any{"kid": false, "building": false, "hero": hero, "rows": rows, "genres": musicNewTopics(rows, hero)})
}

// landAlbumFollows tells people waiting for an album that it is out (an
// in-app message), then ends those follows. At most every 10 minutes.
func (a *API) landAlbumFollows(ctx context.Context, now time.Time) {
	musicNew.mu.Lock()
	if now.Sub(musicNew.landed) < 10*time.Minute {
		musicNew.mu.Unlock()
		return
	}
	musicNew.landed = now
	musicNew.mu.Unlock()
	today := now.Format("2006-01-02")
	_ = a.s.Store.Write(ctx, func(tx *sql.Tx) error {
		rows, err := tx.Query("SELECT user_id, mbid, name, artist FROM album_follows WHERE release_date <= ?", today)
		if err != nil {
			return err
		}
		type land struct {
			user               int64
			mbid, name, artist string
		}
		var all []land
		for rows.Next() {
			var l land
			if rows.Scan(&l.user, &l.mbid, &l.name, &l.artist) == nil {
				all = append(all, l)
			}
		}
		rows.Close()
		at := store.FormatTime(now)
		for _, l := range all {
			if _, err := tx.Exec("INSERT INTO messages (created_at, user_id, kind, text) VALUES (?, ?, 'ready', ?)", at, l.user, l.name+" by "+l.artist+" is out. Ask for it from New & Popular → Music."); err != nil {
				return err
			}
		}
		_, err = tx.Exec("DELETE FROM album_follows WHERE release_date <= ?", today)
		return err
	})
}

// Album follows: an album not out yet, by MusicBrainz release group.

func (a *API) albumFollowState(w http.ResponseWriter, r *http.Request, mbid string) {
	var n int
	if err := a.s.Store.Read().QueryRowContext(r.Context(), "SELECT count(*) FROM album_follows WHERE user_id = ? AND mbid = ?", a.person(r).UserID, mbid).Scan(&n); err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"mbid": mbid, "following": n > 0})
}

func albumMBID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := strings.ToLower(r.PathValue("mbid"))
	if !metadata.ValidMBID(id) {
		fail(w, http.StatusBadRequest, "mbid must be a MusicBrainz release group id")
		return "", false
	}
	return id, true
}

func (a *API) getAlbumFollow(w http.ResponseWriter, r *http.Request) {
	if id, ok := albumMBID(w, r); ok {
		a.albumFollowState(w, r, id)
	}
}

func (a *API) putAlbumFollow(w http.ResponseWriter, r *http.Request) {
	id, ok := albumMBID(w, r)
	if !ok {
		return
	}
	if a.kidMax(r) != "" {
		fail(w, http.StatusForbidden, "music is not shown on this account")
		return
	}
	var body struct {
		Name        string `json:"name"`
		Artist      string `json:"artist"`
		ReleaseDate string `json:"release_date"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, `send {"name": "...", "artist": "...", "release_date": "YYYY-MM-DD"}`)
		return
	}
	body.Name, body.Artist = strings.TrimSpace(body.Name), strings.TrimSpace(body.Artist)
	days, okDate := daysFrom(body.ReleaseDate, time.Now().UTC())
	if body.Name == "" || body.Artist == "" || len(body.Name) > 300 || len(body.Artist) > 300 || !okDate {
		fail(w, http.StatusBadRequest, "name, artist and release_date (YYYY-MM-DD) are needed")
		return
	}
	if days >= 0 {
		fail(w, http.StatusConflict, "that album is out already")
		return
	}
	if err := a.s.Store.Write(r.Context(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO album_follows (user_id, mbid, name, artist, release_date, created_at) VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT DO NOTHING`, a.person(r).UserID, id, body.Name, body.Artist, body.ReleaseDate, store.FormatTime(time.Now()))
		return err
	}); err != nil {
		internal(w, err)
		return
	}
	a.albumFollowState(w, r, id)
}

func (a *API) deleteAlbumFollow(w http.ResponseWriter, r *http.Request) {
	id, ok := albumMBID(w, r)
	if !ok {
		return
	}
	if err := a.s.Store.Write(r.Context(), func(tx *sql.Tx) error {
		_, err := tx.Exec("DELETE FROM album_follows WHERE user_id = ? AND mbid = ?", a.person(r).UserID, id)
		return err
	}); err != nil {
		internal(w, err)
		return
	}
	a.albumFollowState(w, r, id)
}
