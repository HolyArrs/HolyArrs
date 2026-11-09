package music

import (
	"sort"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

func (h *Handlers) rememberNames(list []nameKey) {
	c := &h.music
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.names == nil {
		c.lists, c.names = map[string]musicTrendEntry{}, map[string]metadata.MusicTrendItem{}
	}
	if len(c.names) > musicNamesMax {
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
func (h *Handlers) rememberMusicCards(rows []MusicRow, extra ...MusicCard) {
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
	h.rememberMusicNames(names)
	h.warmMusicNames(names)
}

// musicWarmTTL is how long a queued card picture is not queued again.
const musicWarmTTL = 10 * time.Minute

// warmMusicNames fetches the pictures of these names into the artwork cache
// in the background (albums at 250 and 500, artists at 250), two at a time.
func (h *Handlers) warmMusicNames(list []nameKey) {
	if !h.PartIsOn(parts.Music) || h.lib() == nil || h.lib().Artwork == nil {
		return
	}
	c := &h.music
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
		h.prefetchMusicArt("album", albums)
		h.prefetchMusicArt("artist", artists)
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

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
