package music

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// More Explore → Music rows: new this week, rising artists, classics,
// popular in a country, anniversaries, live albums and EPs, collaborators,
// debuts, decades, moods and soundtracks for the Library's movies and
// shows. Every row needs only the Library, the metadata server's lists
// (ListenBrainz, Deezer) and MusicBrainz, all keyless.

type genreLinkSource interface {
	ArtistGenresLinks(ctx context.Context, mbid string) ([]string, []metadata.ArtistLink, error)
}

type soundtrackSource interface {
	SoundtrackFor(ctx context.Context, title string, year int) ([]metadata.Album, error)
}

// musicCtx is what the main build hands the extra rows.
type musicCtx struct {
	now          time.Time
	hour         int
	region       string
	artists      []*musicArtist
	byTitle      map[int64]*musicArtist
	owned        map[string]int64
	genres       map[string][]string
	trendAlbums  []MusicCard
	trendArtists []MusicCard
	pool         map[string]MusicCard
	top          []string
	tag          func(MusicCard) MusicCard
	albumCard    func(*musicArtist, mediafiles.Part) MusicCard
	names        *[]nameKey
	cold         bool
}

// musicColdTypes are the rows shown without music in the Library.
var musicColdTypes = map[string]bool{
	"top_albums": true, "trending_artists": true, "popular_genre": true, "popular_country": true, "fresh": true,
	"rising": true, "classic": true, "decade": true, "mood": true, "anniversary": true,
}

// musicOrder places each row type on the page (rows review 2026-10-20:
// New from artists you have, Top albums, Missing albums, Trending artists,
// the seeded artist rows, genre picks, then the tail); rows of one type
// keep their order. Seed rows past the first two go to the tail. Just
// arrived and Your asks are Home rows.
var musicOrder = map[string]int{
	"new_from": 20, "top_albums": 30, "missing": 40, "trending_artists": 50, "because_have": 60, "fans_of": 60,
	"genre": 70, "popular_genre": 70, "fresh": 80, "popular_country": 85, "rising": 90, "debut": 95, "classic": 100,
	"mood": 105, "anniversary": 110, "soundtracks": 115, "live_eps": 120, "collabs": 125, "decade": 130, "tail": 140,
}

func init() {
	for t, n := range map[string]int{"fresh": 4, "rising": 4, "classic": 4, "popular_country": 4, "anniversary": 4,
		"live_eps": 4, "collabs": 4, "debut": 3, "decade": 4, "mood": 4, "soundtracks": 3} {
		musicRowMin[t] = n
	}
}

// musicMinFor lowers every row's minimum to 4 for a small music Library.
func musicMinFor(t string, artists int) int {
	m := musicRowMin[t]
	if artists < 10 && m > 4 {
		return 4
	}
	return m
}

// musicSeedCount scales the Because you have / Fans of rows with the
// Library: a third of its artists, 4 to 10.
func musicSeedCount(artists int) int {
	return max(4, min(10, artists/3))
}

// musicHour is the viewer's hour (0-23) when sent, else the server's.
func musicHour(v string, now time.Time) int {
	if hv, err := strconv.Atoi(v); err == nil && hv >= 0 && hv < 24 {
		return hv
	}
	return time.Now().Hour()
}

// musicMoods map each mood row to the tags and genres that fit it.
var musicMoods = []struct{ id, title, words string }{
	{"focus", "Focus", "ambient,classical,post-rock,instrumental,minimal,piano,modern classical,lo-fi,soundtrack"},
	{"workout", "Workout", "edm,house,techno,drum and bass,hip hop,trap,electro,hard rock,metal,dance"},
	{"chill", "Chill", "chill,downtempo,trip hop,lo-fi,ambient,acoustic,folk,bossa nova,dream pop,indie folk"},
	{"party", "Party", "dance,disco,dance-pop,house,funk,reggaeton,electropop,party,pop"},
	{"late_night", "Late night", "r&b,soul,neo soul,jazz,trip hop,smooth,blues,contemporary r&b,alternative r&b"},
}

// moodLead is the mood that leads at an hour (weekend evenings: party).
func moodLead(hour int, day time.Weekday) string {
	switch {
	case (day == time.Friday || day == time.Saturday) && (hour >= 20 || hour < 2):
		return "party"
	case hour >= 6 && hour < 11:
		return "focus"
	case hour >= 11 && hour < 17:
		return "workout"
	case hour >= 17 && hour < 21:
		return "chill"
	}
	return "late_night"
}

func moodMatch(tags []string, words string) bool {
	for _, t := range tags {
		for _, w := range strings.Split(words, ",") {
			if t == w || strings.Contains(t, w) {
				return true
			}
		}
	}
	return false
}

var deluxeRe = regexp.MustCompile(`(?i)\b(deluxe|expanded|anniversary|special|collector'?s) edition\b|\(deluxe\)|\bdeluxe\b`)

// soundtrackCache keeps soundtrack lookups for Library movies and shows.
type soundtrackCache struct {
	mu      sync.Mutex
	found   map[int64]soundtrackEntry
	queue   []soundtrackJob
	queued  map[int64]bool
	running bool
	lookups []time.Time
}

type soundtrackEntry struct {
	albums []metadata.Album
	at     time.Time
}

type soundtrackJob struct {
	id   int64
	name string
	year int
}

var musicSoundtracks = map[*Handlers]*soundtrackCache{}
var musicSoundtracksMu sync.Mutex

func (h *Handlers) soundtracks() *soundtrackCache {
	musicSoundtracksMu.Lock()
	defer musicSoundtracksMu.Unlock()
	c := musicSoundtracks[h]
	if c == nil {
		c = &soundtrackCache{found: map[int64]soundtrackEntry{}, queued: map[int64]bool{}}
		musicSoundtracks[h] = c
	}
	return c
}

// soundtracksFor answers known soundtracks of each title and queues the
// rest for a paced MusicBrainz search (100 an hour at most).
func (h *Handlers) soundtracksFor(jobs []soundtrackJob) map[int64][]metadata.Album {
	out := map[int64][]metadata.Album{}
	var src soundtrackSource
	if h.Library != nil && h.Library.Artists != nil {
		src, _ = h.Library.Artists.(soundtrackSource)
	}
	c := h.soundtracks()
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, j := range jobs {
		e, ok := c.found[j.id]
		if ok {
			out[j.id] = e.albums
		}
		if (!ok || time.Since(e.at) > musicGenreTTL) && src != nil && !c.queued[j.id] && len(c.queue) < 200 {
			c.queue = append(c.queue, j)
			c.queued[j.id] = true
		}
	}
	if src != nil && len(c.queue) > 0 && !c.running {
		c.running = true
		go func() {
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
				if len(c.queue) == 0 || len(c.lookups) >= 100 {
					c.running = false
					c.mu.Unlock()
					return
				}
				j := c.queue[0]
				c.queue = c.queue[1:]
				c.lookups = append(c.lookups, now)
				c.mu.Unlock()
				ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
				albums, err := src.SoundtrackFor(ctx, j.name, j.year)
				cancel()
				c.mu.Lock()
				delete(c.queued, j.id)
				if err == nil {
					c.found[j.id] = soundtrackEntry{albums: albums, at: time.Now()}
				}
				c.mu.Unlock()
			}
		}()
	}
	return out
}

// linksOf answers the known collaborators of each artist id.
func (h *Handlers) linksOf(ids []string) map[string][]metadata.ArtistLink {
	c := &h.musicx
	c.mu.Lock()
	defer c.mu.Unlock()
	out := map[string][]metadata.ArtistLink{}
	for _, id := range ids {
		if e, ok := c.genres[id]; ok {
			out[id] = e.links
		}
	}
	return out
}

func yearOf(d string) int {
	if len(d) < 4 {
		return 0
	}
	y, _ := strconv.Atoi(d[:4])
	return y
}

func studio(p mediafiles.Part) bool {
	return strings.EqualFold(derefStr(p.ReleaseType), "album") && len(p.SecondaryTypes) == 0
}

// listCards turns one of the metadata server's lists into cards.
func (m *musicCtx) listCards(l metadata.MusicTrend, kind string) []MusicCard {
	var out []MusicCard
	for _, it := range l.Items {
		c := MusicCard{Kind: kind, MBID: strings.ToLower(it.MBID), Rank: it.Rank, Listens: it.Listens, Date: it.ReleaseDate}
		if kind == "artist" {
			c.Name, c.Artist, c.ArtistMBID = it.Name, it.Name, c.MBID
		} else {
			c.Name, c.Artist, c.ArtistMBID = it.Title, it.Artist, strings.ToLower(it.ArtistMBID)
		}
		if c.MBID == "" || c.Name == "" || c.ArtistMBID == "" {
			continue
		}
		c.Image = "/api/v1/music-trending/art/" + kind + "/" + c.MBID
		*m.names = append(*m.names, nameKey{kind, c.MBID, c.Name, c.Artist, ""})
		c = m.tag(c)
		for _, t := range it.Tags {
			if !hasGenre(c.Genres, t) {
				c.Genres = append(c.Genres, t)
			}
		}
		out = append(out, c)
	}
	return out
}

func (h *Handlers) moreMusicRows(ctx context.Context, m *musicCtx, out []MusicRow) []MusicRow {
	db := h.Store.Read()
	add := func(r MusicRow) {
		for i := range r.Items {
			if r.Items[i].Key == "" {
				r.Items[i].Key = r.Items[i].Kind + ":" + r.Items[i].MBID
			}
		}
		out = append(out, r)
	}
	list := func(kind, region string) metadata.MusicTrend {
		if h.MusicTrending == nil {
			return metadata.MusicTrend{}
		}
		l, err := h.cachedMusicTrend(ctx, kind, region)
		if err != nil {
			return metadata.MusicTrend{}
		}
		return l
	}
	fresh := m.listCards(list("fresh", "global"), "album")
	classicAll := m.listCards(list("classic", "global"), "album")

	// Albums already on disk, by release group.
	have := map[string]bool{}
	if rs, err := db.QueryContext(ctx, `SELECT DISTINCT json_extract(p.source_ids, '$.musicbrainz') FROM parts p JOIN items i ON i.part_id = p.id
		JOIN slots s ON s.item_id = i.id WHERE p.kind = 'album' AND s.file_id IS NOT NULL`); err == nil {
		for rs.Next() {
			var id *string
			if rs.Scan(&id) == nil && id != nil {
				have[strings.ToLower(*id)] = true
			}
		}
		rs.Close()
	}
	var classic []MusicCard
	for _, c := range classicAll {
		if !have[c.MBID] {
			classic = append(classic, c)
		}
	}

	add(MusicRow{ID: "fresh", Type: "fresh", Title: "New this week", Reason: "Notable new albums and EPs, most played first", Shape: "square", Items: fresh})
	add(MusicRow{ID: "rising", Type: "rising", Title: "Rising artists", Reason: "New to this week's 100 most played artists", Shape: "round", Items: m.listCards(list("rising", "global"), "artist")})
	add(MusicRow{ID: "classic", Type: "classic", Title: "Classic albums every collection needs", Reason: "The most played albums of all time, not in your Library", Shape: "square", Items: classic})
	if m.region != "global" {
		add(MusicRow{ID: "popular_country:" + m.region, Type: "popular_country", Title: "Popular in " + m.region, Reason: "This week's top albums in " + m.region, Shape: "square", Items: m.listCards(list("album", m.region), "album")})
	}

	// The Library's albums, for anniversaries, live and EPs, debuts, decades.
	var parts []mediafiles.Part
	if len(m.artists) > 0 {
		parts, _ = database.Select[mediafiles.Part](ctx, db, "parts", `kind = 'album' AND dropped_at IS NULL AND release_date IS NOT NULL
			AND title_id IN (SELECT id FROM titles WHERE kind = 'artist' AND state != 'removed') ORDER BY release_date DESC, id`)
	}
	partHas := map[int64]bool{}
	for _, p := range parts {
		if have[strings.ToLower(p.SourceIDs["musicbrainz"])] {
			partHas[p.ID] = true
		}
	}
	// interleave takes turns between artists so one does not fill a row.
	interleave := func(by map[int64][]MusicCard) []MusicCard {
		var order []*musicArtist
		order = append(order, m.artists...)
		sort.SliceStable(order, func(i, j int) bool { return order[i].files > order[j].files })
		var res []MusicCard
		for round := 0; len(res) < musicRowMax && round < musicRowMax; round++ {
			for _, ar := range order {
				if l := by[ar.id]; round < len(l) && len(res) < musicRowMax {
					res = append(res, l[round])
				}
			}
		}
		return res
	}

	// Anniversaries: albums turning 10, 20, 25 or 30 this year.
	{
		marks := map[int]int{}
		for _, n := range []int{10, 20, 25, 30} {
			marks[m.now.Year()-n] = n
		}
		var items []MusicCard
		for _, p := range parts {
			if n, ok := marks[p.ReleaseDate.Year()]; ok && studio(p) {
				if ar := m.byTitle[p.TitleID]; ar != nil {
					c := m.albumCard(ar, p)
					c.Note = fmt.Sprintf("%d years · %s", n, ar.name)
					items = append(items, c)
				}
			}
		}
		for _, c := range classicAll {
			if n, ok := marks[yearOf(c.Date)]; ok {
				c.Note = fmt.Sprintf("%d years · %s", n, c.Artist)
				items = append(items, c)
			}
		}
		add(MusicRow{ID: "anniversary", Type: "anniversary", Title: "Turning 10, 20, 25 or 30 this year", Reason: fmt.Sprintf("Albums from %d, %d, %d and %d", m.now.Year()-10, m.now.Year()-20, m.now.Year()-25, m.now.Year()-30), Shape: "square", Items: items})
	}

	// Live albums, EPs and deluxe editions from artists you have.
	{
		by := map[int64][]MusicCard{}
		for _, p := range parts {
			ar := m.byTitle[p.TitleID]
			if ar == nil || p.ReleaseDate.After(m.now) || len(by[p.TitleID]) >= 3 {
				continue
			}
			kind := ""
			switch {
			case hasGenre(p.SecondaryTypes, "live"):
				kind = "Live"
			case strings.EqualFold(derefStr(p.ReleaseType), "ep") && len(p.SecondaryTypes) == 0:
				kind = "EP"
			case deluxeRe.MatchString(p.Name):
				kind = "Deluxe edition"
			}
			if kind == "" {
				continue
			}
			c := m.albumCard(ar, p)
			c.Note = kind + " · " + ar.name
			by[p.TitleID] = append(by[p.TitleID], c)
		}
		add(MusicRow{ID: "live_eps", Type: "live_eps", Title: "Live albums, EPs and deluxe editions", Reason: "More from artists you have", Shape: "square", Items: interleave(by)})
	}

	// Debut albums missing from the Library.
	{
		first := map[int64]mediafiles.Part{}
		for _, p := range parts {
			if studio(p) && !p.ReleaseDate.After(m.now) {
				if f, ok := first[p.TitleID]; !ok || p.ReleaseDate.Before(*f.ReleaseDate) {
					first[p.TitleID] = p
				}
			}
		}
		by := map[int64][]MusicCard{}
		for id, p := range first {
			if ar := m.byTitle[id]; ar != nil && !partHas[p.ID] {
				c := m.albumCard(ar, p)
				c.Note = fmt.Sprintf("%s's debut · %d", ar.name, p.ReleaseDate.Year())
				by[id] = []MusicCard{c}
			}
		}
		add(MusicRow{ID: "debut", Type: "debut", Title: "Debut albums missing from your Library", Reason: "Where the artists you have started", Shape: "square", Items: interleave(by)})
	}

	// Collaborators: members, bands and collaborators of artists you have.
	{
		var ids []string
		for _, ar := range m.artists {
			ids = append(ids, ar.mbid)
		}
		links := h.linksOf(ids)
		var items []MusicCard
		seen := map[string]bool{}
		for _, ar := range m.artists {
			for _, l := range links[ar.mbid] {
				if _, mine := m.owned[l.MBID]; mine || seen[l.MBID] {
					continue
				}
				seen[l.MBID] = true
				c := m.tag(MusicCard{Kind: "artist", MBID: l.MBID, Name: l.Name, Artist: l.Name, ArtistMBID: l.MBID, Image: "/api/v1/music-trending/art/artist/" + l.MBID, Note: "With " + ar.name})
				*m.names = append(*m.names, nameKey{"artist", l.MBID, l.Name, "", ""})
				items = append(items, c)
			}
		}
		add(MusicRow{ID: "collabs", Type: "collabs", Title: "Artists your artists worked with", Reason: "Members, bands and collaborators of artists you have", Shape: "round", Items: items})
	}

	// Decades: the Library's favourite decades; cold start: the classics'.
	{
		count := map[int]int{}
		for _, p := range parts {
			if studio(p) && (len(partHas) == 0 || partHas[p.ID]) {
				count[p.ReleaseDate.Year()/10*10]++
			}
		}
		var decades []int
		if m.cold || len(count) == 0 {
			decades = []int{1980, 1990, 2000, 2010}
		} else {
			for d := range count {
				if d >= 1950 {
					decades = append(decades, d)
				}
			}
			sort.Slice(decades, func(i, j int) bool {
				if count[decades[i]] != count[decades[j]] {
					return count[decades[i]] > count[decades[j]]
				}
				return decades[i] > decades[j]
			})
			decades = decades[:min(3, len(decades))]
		}
		for _, d := range decades {
			var items []MusicCard
			for _, c := range classic {
				if y := yearOf(c.Date); y >= d && y < d+10 {
					items = append(items, c)
				}
			}
			title, reason := fmt.Sprintf("Classic %ds albums", d), fmt.Sprintf("All-time favourites from the %ds", d)
			if !m.cold {
				for _, p := range parts {
					if y := p.ReleaseDate.Year(); y >= d && y < d+10 && studio(p) && !partHas[p.ID] {
						if ar := m.byTitle[p.TitleID]; ar != nil {
							items = append(items, m.albumCard(ar, p))
						}
					}
				}
				title, reason = fmt.Sprintf("%ds albums for you", d), fmt.Sprintf("Your Library leans to the %ds", d)
			}
			add(MusicRow{ID: fmt.Sprintf("decade:%d", d), Type: "decade", Title: title, Reason: reason, Shape: "square", Items: items})
		}
	}

	// Moods, from tags and genres; the hour picks which one leads.
	{
		var albums []MusicCard
		albums = append(albums, fresh...)
		albums = append(albums, classic...)
		albums = append(albums, m.trendAlbums...)
		artists := sortedPool(m.pool)
		lead := moodLead(m.hour, m.now.Weekday())
		var rows []MusicRow
		for _, md := range musicMoods {
			var items []MusicCard
			for _, c := range albums {
				if moodMatch(c.Genres, md.words) {
					items = append(items, c)
				}
			}
			for _, c := range artists {
				if moodMatch(c.Genres, md.words) {
					items = append(items, c)
				}
			}
			words := strings.Split(md.words, ",")
			r := MusicRow{ID: "mood:" + md.id, Type: "mood", Title: md.title, Reason: "Tagged " + strings.Join(words[:3], ", ") + " and more", Shape: "square", Items: items}
			if md.id == lead {
				rows = append([]MusicRow{r}, rows...)
			} else {
				rows = append(rows, r)
			}
		}
		for _, r := range rows {
			add(r)
		}
	}

	// Soundtracks for movies and shows in the Library.
	{
		ts, _ := database.Select[mediafiles.Title](ctx, db, "titles", "kind IN ('movie', 'show') AND state != 'removed' ORDER BY id DESC LIMIT 60")
		var jobs []soundtrackJob
		for _, t := range ts {
			y := 0
			if t.Year != nil {
				y = *t.Year
			}
			jobs = append(jobs, soundtrackJob{t.ID, t.Name, y})
		}
		found := h.soundtracksFor(jobs)
		var items []MusicCard
		for _, t := range ts {
			for _, al := range found[t.ID][:min(1, len(found[t.ID]))] {
				c := MusicCard{Kind: "album", MBID: al.MBID, Name: al.Name, Date: al.ReleaseDate, Image: "/api/v1/music-trending/art/album/" + al.MBID, Note: "From " + t.Name, Genres: []string{"soundtrack"}}
				*m.names = append(*m.names, nameKey{"album", al.MBID, al.Name, "", ""})
				items = append(items, c)
			}
		}
		add(MusicRow{ID: "soundtracks", Type: "soundtracks", Title: "Soundtracks for titles in your Library", Reason: "Matched to movies and shows you have", Shape: "square", Items: items})
	}

	// Seed rows past the first two make the tail.
	rank := func(r MusicRow) int {
		n := musicOrder[r.Type]
		if n == 0 {
			n = musicOrder["tail"]
		}
		return n
	}
	seen := 0
	ranks := make([]int, len(out))
	for i, r := range out {
		ranks[i] = rank(r)
		if r.Type == "because_have" || r.Type == "fans_of" {
			seen++
			if seen > 2 {
				ranks[i] = musicOrder["tail"]
			}
		}
	}
	idx := make([]int, len(out))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(i, j int) bool { return ranks[idx[i]] < ranks[idx[j]] })
	sorted := make([]MusicRow, len(out))
	for i, k := range idx {
		sorted[i] = out[k]
	}
	return sorted
}
